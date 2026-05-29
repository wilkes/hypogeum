// Package watch observes a directory tree for markdown-relevant filesystem
// changes and surfaces them as coarse, debounced events on a channel.
//
// fsnotify watches individual directories, not trees, so the watcher walks
// the tree once at startup and re-Adds directories that appear later. It
// does not recurse into hidden directories (.git, etc.) — same rule as
// internal/tree, so the two views stay consistent.
package watch

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/wilkes/hypogeum/internal/tree"
)

// EventKind tells callers what kind of refresh the change requires.
//
// We deliberately keep it coarse: TUI consumers care about "rebuild the tree"
// vs. "reread the open file", not the exact fsnotify op.
type EventKind int

const (
	// StructureChanged means a markdown file or directory was created,
	// removed, or renamed. The tree should be re-walked.
	StructureChanged EventKind = iota
	// FileModified means an existing markdown file's contents were written.
	// Only the currently displayed file (if it matches) needs a refresh.
	FileModified
)

// Event is a debounced summary of one or more raw fsnotify events.
type Event struct {
	Kind  EventKind
	Paths []string // affected absolute paths (deduped)
}

// Watcher observes a directory tree. Construct with New, read from Events(),
// and call Close when done.
type Watcher struct {
	fsw    *fsnotify.Watcher
	out    chan Event
	done   chan struct{}
	closed sync.Once

	// debounceWindow controls how long raw events are coalesced before
	// being emitted. Editors often write via rename-over-temp, producing
	// 2–4 events for a single save.
	debounceWindow time.Duration
}

// New starts watching the tree(s) rooted at roots. Multiple roots are watched
// as one overlaid set, mirroring tree.Merge. The returned Watcher emits
// debounced events on Events() until Close is called.
func New(roots ...string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		fsw:            fsw,
		out:            make(chan Event, 8),
		done:           make(chan struct{}),
		debounceWindow: 100 * time.Millisecond,
	}
	for _, root := range roots {
		if err := addDirsRecursive(fsw, root); err != nil {
			fsw.Close()
			return nil, err
		}
	}
	go w.run()
	return w, nil
}

// Events returns the channel of debounced events. Closed when Close is called.
func (w *Watcher) Events() <-chan Event { return w.out }

// Close stops watching and releases resources. Safe to call multiple times.
func (w *Watcher) Close() error {
	w.closed.Do(func() {
		close(w.done)
		w.fsw.Close()
	})
	return nil
}

// addDirsRecursive Adds dir and every non-hidden subdirectory to fsw.
// Errors on individual entries are skipped (a transient ENOENT during
// startup shouldn't kill the watcher).
func addDirsRecursive(fsw *fsnotify.Watcher, dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != dir && tree.IsHidden(d.Name()) {
			return filepath.SkipDir
		}
		_ = fsw.Add(path)
		return nil
	})
}

func (w *Watcher) run() {
	var (
		p      pending
		timer  *time.Timer
		timerC <-chan time.Time
	)

	for {
		select {
		case <-w.done:
			close(w.out)
			return

		case ev, ok := <-w.fsw.Events:
			if !ok {
				close(w.out)
				return
			}
			w.stage(ev, &p)
			timerC = w.resetTimer(&timer)

		case _, ok := <-w.fsw.Errors:
			if !ok {
				close(w.out)
				return
			}
			// fsnotify errors are typically transient and not actionable
			// from here; drop them rather than crashing the goroutine.

		case <-timerC:
			w.flush(&p)
			timerC = nil
		}
	}
}

// AddPath adds dir to the underlying fsnotify watcher so writes inside
// dir surface as Events. Idempotent and nil-safe — callers can invoke
// it freely from per-render code paths. Used by the TUI to extend the
// watch set to source files referenced by ![[...]] embeds living
// outside the markdown root.
func (w *Watcher) AddPath(dir string) error {
	if w == nil || w.fsw == nil {
		return nil
	}
	return w.fsw.Add(dir)
}

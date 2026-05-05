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
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
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

// markdown extensions recognized for FileModified events. Kept in sync with
// internal/tree.markdownExts — duplicated rather than imported to keep this
// package leaf-level.
var markdownExts = map[string]struct{}{
	".md":       {},
	".markdown": {},
	".mdown":    {},
	".mkd":      {},
}

// New starts watching the tree rooted at root. The returned Watcher emits
// debounced events on Events() until Close is called.
func New(root string) (*Watcher, error) {
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
	if err := addDirsRecursive(fsw, root); err != nil {
		fsw.Close()
		return nil, err
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
		if path != dir && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		_ = fsw.Add(path)
		return nil
	})
}

func (w *Watcher) run() {
	var (
		pendingStruct = make(map[string]struct{})
		pendingWrite  = make(map[string]struct{})
		timer         *time.Timer
		timerC        <-chan time.Time
	)

	resetTimer := func() {
		if timer == nil {
			timer = time.NewTimer(w.debounceWindow)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.debounceWindow)
		}
		timerC = timer.C
	}

	flush := func() {
		if len(pendingStruct) > 0 {
			paths := drainSet(pendingStruct)
			select {
			case w.out <- Event{Kind: StructureChanged, Paths: paths}:
			case <-w.done:
				return
			}
		}
		if len(pendingWrite) > 0 {
			paths := drainSet(pendingWrite)
			select {
			case w.out <- Event{Kind: FileModified, Paths: paths}:
			case <-w.done:
				return
			}
		}
		timerC = nil
	}

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
			w.classify(ev, pendingStruct, pendingWrite)
			resetTimer()

		case _, ok := <-w.fsw.Errors:
			if !ok {
				close(w.out)
				return
			}
			// fsnotify errors are typically transient and not actionable
			// from here; drop them rather than crashing the goroutine.

		case <-timerC:
			flush()
		}
	}
}

// classify routes one raw fsnotify event into the pending-event maps.
// Hidden paths are ignored. Newly created directories are added to the
// underlying watcher so their contents are observed too.
func (w *Watcher) classify(ev fsnotify.Event, pendingStruct, pendingWrite map[string]struct{}) {
	if isHiddenPath(ev.Name) {
		return
	}

	switch {
	case ev.Op&fsnotify.Create != 0:
		// If a new directory appeared, watch it (and its descendants) so
		// files dropped into it later raise events too.
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			_ = addDirsRecursive(w.fsw, ev.Name)
			pendingStruct[ev.Name] = struct{}{}
			return
		}
		if isMarkdown(ev.Name) {
			pendingStruct[ev.Name] = struct{}{}
		}

	case ev.Op&(fsnotify.Remove|fsnotify.Rename) != 0:
		// We don't know whether the removed entry was a directory or a
		// markdown file (it's gone), so be conservative: any remove or
		// rename inside a watched dir triggers a structure refresh.
		pendingStruct[ev.Name] = struct{}{}

	case ev.Op&fsnotify.Write != 0:
		if isMarkdown(ev.Name) {
			pendingWrite[ev.Name] = struct{}{}
		}
	}
}

func drainSet(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
		delete(m, k)
	}
	return out
}

func isMarkdown(name string) bool {
	_, ok := markdownExts[strings.ToLower(filepath.Ext(name))]
	return ok
}

func isHiddenPath(p string) bool {
	for _, part := range strings.Split(filepath.ToSlash(p), "/") {
		if part == "" || part == "." || part == ".." {
			continue
		}
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

package watch

import (
	"os"

	"github.com/fsnotify/fsnotify"

	"github.com/wilkes/hypogeum/internal/tree"
)

// classifyResult tells the watcher how to stage one fsnotify event.
//
// classify is pure: it inspects only the op flags and path. Distinguishing
// a newly created directory from a newly created file requires os.Stat, so
// classify reports MaybeNewDir on Create and the wrapper performs the stat.
// Likewise, a Create whose target is neither a directory nor markdown is
// dropped by the wrapper after the stat — the free function returns
// Kind=StructureChanged and the wrapper decides whether to keep it.
type classifyResult struct {
	Kind        EventKind // StructureChanged or FileModified (only meaningful when Ignore is false)
	Path        string
	Ignore      bool // true when the event should be dropped (hidden, non-markdown write)
	MaybeNewDir bool // Create event: wrapper must stat to decide dir vs file
}

// classify maps one fsnotify event to a classifyResult without touching
// the filesystem. Hidden paths are filtered here so the wrapper can early-
// out without a stat.
func classify(ev fsnotify.Event) classifyResult {
	if tree.IsHiddenPath(ev.Name) {
		return classifyResult{Path: ev.Name, Ignore: true}
	}
	switch {
	case ev.Op&fsnotify.Create != 0:
		return classifyResult{Kind: StructureChanged, Path: ev.Name, MaybeNewDir: true}
	case ev.Op&(fsnotify.Remove|fsnotify.Rename) != 0:
		// We don't know whether the removed entry was a directory or a
		// markdown file (it's gone), so be conservative: any remove or
		// rename inside a watched dir triggers a structure refresh.
		return classifyResult{Kind: StructureChanged, Path: ev.Name}
	case ev.Op&fsnotify.Write != 0:
		// Emit FileModified for any write. The TUI's handleFSEvent
		// filters by "is this the currently open file?" so writes to
		// non-md files we don't have open are discarded one layer up
		// at zero cost. Relaxing here is what makes live-reload work
		// when the open file is a .go/.rb/.py/etc.
		return classifyResult{Kind: FileModified, Path: ev.Name}
	}
	return classifyResult{Path: ev.Name, Ignore: true}
}

// stage applies the impure side effects classify deferred: stat'ing Create
// targets to decide between a new directory (which must be added to fsw),
// a new markdown file (StructureChanged), or a non-markdown file (drop).
// All other classifications are recorded into pending directly.
func (w *Watcher) stage(ev fsnotify.Event, p *pending) {
	r := classify(ev)
	if r.Ignore {
		return
	}
	if r.MaybeNewDir {
		if info, err := os.Stat(r.Path); err == nil && info.IsDir() {
			_ = addDirsRecursive(w.fsw, r.Path)
			p.addStruct(r.Path)
			return
		}
		if !tree.IsMarkdown(r.Path) {
			return
		}
		p.addStruct(r.Path)
		return
	}
	switch r.Kind {
	case StructureChanged:
		p.addStruct(r.Path)
	case FileModified:
		p.addWrite(r.Path)
	}
}

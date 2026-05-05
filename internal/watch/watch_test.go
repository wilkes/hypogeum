package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// receive waits up to timeout for one event from w.Events(). Returns
// (event, true) on success, (zero, false) on timeout.
func receive(t *testing.T, w *Watcher, timeout time.Duration) (Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-w.Events():
		if !ok {
			t.Fatal("events channel closed unexpectedly")
		}
		return ev, true
	case <-time.After(timeout):
		return Event{}, false
	}
}

func TestWatcher_FileModified(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("# original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if err := os.WriteFile(path, []byte("# updated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ev, ok := receive(t, w, 2*time.Second)
	if !ok {
		t.Fatal("timed out waiting for FileModified event")
	}
	if ev.Kind != FileModified {
		t.Errorf("kind = %v, want FileModified", ev.Kind)
	}
	if len(ev.Paths) != 1 || ev.Paths[0] != path {
		t.Errorf("paths = %v, want [%s]", ev.Paths, path)
	}
}

func TestWatcher_NewMarkdownFileTriggersStructure(t *testing.T) {
	root := t.TempDir()
	w, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	path := filepath.Join(root, "fresh.md")
	if err := os.WriteFile(path, []byte("# hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Saw at least one StructureChanged before the deadline. A Write may
	// also be coalesced into the same flush; either way the first event
	// of the burst should be a structure change.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-w.Events():
			if ev.Kind == StructureChanged {
				found := false
				for _, p := range ev.Paths {
					if p == path {
						found = true
					}
				}
				if !found {
					t.Errorf("StructureChanged paths = %v, want to contain %s", ev.Paths, path)
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for StructureChanged")
		}
	}
}

func TestWatcher_IgnoresHiddenAndNonMarkdown(t *testing.T) {
	root := t.TempDir()
	w, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// non-markdown
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	// hidden directory contents are filtered by isHiddenPath even though
	// we never Add'd the dir itself; the create event for the dir comes
	// through the parent and is hidden-prefixed.
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref"), 0o644); err != nil {
		t.Fatal(err)
	}

	if ev, ok := receive(t, w, 400*time.Millisecond); ok {
		t.Errorf("expected no event, got %+v", ev)
	}
}

func TestWatcher_RemovedFileEmitsStructure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "gone.md")
	if err := os.WriteFile(path, []byte("# bye\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	ev, ok := receive(t, w, 2*time.Second)
	if !ok {
		t.Fatal("timed out waiting for StructureChanged after remove")
	}
	if ev.Kind != StructureChanged {
		t.Errorf("kind = %v, want StructureChanged", ev.Kind)
	}
}

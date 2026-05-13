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

// TestWatcher_IgnoresHidden verifies the hidden-path filter in classify.
// Non-markdown writes are NOT filtered here — that's a deliberate
// relaxation so live-reload works for the open .go/.py/etc. file; the
// TUI's handleFSEvent does the "is this the open file?" filtering one
// layer up. (See classify.go and CLAUDE.md's "write classifier accepts
// any path" gotcha.) So this test only asserts the hidden filter.
func TestWatcher_IgnoresHidden(t *testing.T) {
	root := t.TempDir()
	w, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Hidden directory contents are filtered by IsHiddenPath even though
	// we never Add'd the dir itself; the create event for the dir comes
	// through the parent and is hidden-prefixed.
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref"), 0o644); err != nil {
		t.Fatal(err)
	}

	if ev, ok := receive(t, w, 400*time.Millisecond); ok {
		t.Errorf("expected no event for hidden path, got %+v", ev)
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

func TestWatcher_AddPathIdempotent(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	extra := t.TempDir()
	if err := w.AddPath(extra); err != nil {
		t.Fatalf("AddPath: %v", err)
	}
	if err := w.AddPath(extra); err != nil { // second call is a no-op
		t.Fatalf("AddPath (second call): %v", err)
	}
}

func TestWatcher_AddPathNilSafe(t *testing.T) {
	var w *Watcher
	if err := w.AddPath("/tmp"); err != nil {
		t.Fatalf("nil receiver AddPath: %v", err)
	}
}

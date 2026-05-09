package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wilkes/hypogeum/internal/watch"
)

func TestModel_FSEventRebuildsTreeOnStructureChange(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	before := len(m.tree.flat)

	// Add a new top-level markdown file and synthesize the watcher event
	// directly, bypassing the real fsnotify path so the test is deterministic.
	newPath := filepath.Join(root, "added.md")
	if err := os.WriteFile(newPath, []byte("# added\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.handleFSEvent(watch.Event{Kind: watch.StructureChanged, Paths: []string{newPath}})

	if len(m.tree.flat) != before+1 {
		t.Fatalf("flatTree length = %d, want %d", len(m.tree.flat), before+1)
	}
	found := false
	for _, row := range m.tree.flat {
		if row.node.Path == newPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("new file %s not in flatTree", newPath)
	}
}

func TestModel_FSEventPreservesCursorOnStructureChange(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	firstPath := filepath.Join(root, "notes", "first.md")
	for i, row := range m.tree.flat {
		if row.node.Path == firstPath {
			m.tree.cursor = i
		}
	}
	if m.tree.flat[m.tree.cursor].node.Path != firstPath {
		t.Fatalf("setup: cursor not on %s", firstPath)
	}

	if err := os.WriteFile(filepath.Join(root, "zzz.md"), []byte("# z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.handleFSEvent(watch.Event{Kind: watch.StructureChanged})

	if got := m.tree.flat[m.tree.cursor].node.Path; got != firstPath {
		t.Errorf("cursor moved to %s, want %s", got, firstPath)
	}
}

func TestModel_FSEventRefreshesOpenFileOnWrite(t *testing.T) {
	root := writeFixture(t)
	indexPath := filepath.Join(root, "index.md")
	m := sized(t, root, indexPath)

	if err := os.WriteFile(indexPath, []byte("# Replaced\n\nFresh body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.handleFSEvent(watch.Event{Kind: watch.FileModified, Paths: []string{indexPath}})

	// Glamour wraps each word in its own SGR span, so "Fresh body" is
	// split by ANSI escapes in the rendered output. Check tokens separately.
	view := m.viewport.View()
	if !strings.Contains(view, "Fresh") || !strings.Contains(view, "body") {
		t.Errorf("viewport did not pick up new contents:\n%s", view)
	}
}

func TestModel_FSEventIgnoresWriteToOtherFile(t *testing.T) {
	root := writeFixture(t)
	indexPath := filepath.Join(root, "index.md")
	m := sized(t, root, indexPath)

	otherPath := filepath.Join(root, "notes", "first.md")
	original := m.viewport.View()

	// Modify a different file. Even with a FileModified event for it,
	// the open file's view should be untouched.
	if err := os.WriteFile(otherPath, []byte("# Different\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.handleFSEvent(watch.Event{Kind: watch.FileModified, Paths: []string{otherPath}})

	if m.viewport.View() != original {
		t.Errorf("viewport changed for write to other file")
	}
}

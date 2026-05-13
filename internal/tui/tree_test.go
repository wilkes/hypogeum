package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModel_TreeNavigationAndOpen(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	want := filepath.Join(root, "notes", "first.md")
	target := m.rowIndexByPath(want)
	if target < 0 {
		t.Fatalf("first.md not found in flattened tree: %+v", m.tree.flat)
	}
	m = driveCursorTo(t, m, target)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if got := m.history.Current(); got != want {
		t.Errorf("after Enter, history.Current = %q, want %q", got, want)
	}
	if m.modals.kind != modalNone {
		t.Errorf("Enter on a file row should close the tree modal, got kind=%v", m.modals.kind)
	}
}

// TestModel_ToggleTreeOpensAndClosesModal checks that ^b opens the tree
// modal and a second ^b closes it. The tree renders only inside the
// modal — there is no side pane.
func TestModel_ToggleTreeOpensAndClosesModal(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	if m.modals.kind != modalNone {
		t.Fatalf("modal should start closed, got %v", m.modals.kind)
	}
	if strings.Contains(m.View(), "▾ notes/") || strings.Contains(m.View(), "▸ notes/") {
		t.Fatalf("tree rows must not render outside the modal")
	}

	// First ^b opens the tree modal.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(Model)
	if m.modals.kind != modalTree {
		t.Fatalf("first ^b should open tree modal, got kind=%v", m.modals.kind)
	}
	if !strings.Contains(m.View(), "notes") {
		t.Errorf("expected tree modal to mention 'notes' after open")
	}

	// Second ^b closes it.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(Model)
	if m.modals.kind != modalNone {
		t.Errorf("second ^b should close tree modal, got kind=%v", m.modals.kind)
	}
}

// TestModel_TreeModalLandsOnCurrentFile checks that opening the tree
// modal positions the cursor on the currently open file, not on the
// vault root.
func TestModel_TreeModalLandsOnCurrentFile(t *testing.T) {
	root := writeFixture(t)
	firstPath := filepath.Join(root, "notes", "first.md")
	m := sized(t, root, firstPath)

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.modals.kind != modalTree {
		t.Fatalf("^b should open tree modal, got kind=%v", m.modals.kind)
	}
	if got := m.tree.flat[m.tree.cursor].node.Path; got != firstPath {
		t.Errorf("tree cursor on open = %q, want %q", got, firstPath)
	}
}

// TestModel_TreeModalEscClosesWithoutOpening checks that Esc closes the
// tree modal without opening anything in the content pane.
func TestModel_TreeModalEscClosesWithoutOpening(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	before := m.history.Current()

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.modals.kind != modalTree {
		t.Fatalf("^b should open tree modal, got kind=%v", m.modals.kind)
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.modals.kind != modalNone {
		t.Errorf("Esc should close tree modal, got kind=%v", m.modals.kind)
	}
	if m.history.Current() != before {
		t.Errorf("Esc should not change history; was %q, now %q", before, m.history.Current())
	}
}

// TestModel_CollapseFolderHidesChildren checks that pressing space on a
// directory row inside the tree modal removes its descendants from flatTree.
func TestModel_CollapseFolderHidesChildren(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	notesDir := filepath.Join(root, "notes")
	target := m.rowIndexByPath(notesDir)
	if target < 0 || !m.tree.flat[target].node.IsDir {
		t.Fatalf("notes/ directory row not found in flatTree")
	}
	m = driveCursorTo(t, m, target)

	before := len(m.tree.flat)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace})

	if !m.isCollapsed(notesDir) {
		t.Fatalf("notes/ should be collapsed after space")
	}
	if len(m.tree.flat) >= before {
		t.Errorf("flatTree should shrink on collapse: before=%d after=%d", before, len(m.tree.flat))
	}
	for _, row := range m.tree.flat {
		if strings.HasPrefix(row.node.Path, notesDir+string(filepath.Separator)) {
			t.Errorf("collapsed descendant still in flatTree: %s", row.node.Path)
		}
	}

	// Toggling again restores the descendants.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if m.isCollapsed(notesDir) {
		t.Fatalf("notes/ should be expanded again")
	}
	if len(m.tree.flat) != before {
		t.Errorf("flatTree should restore on re-expand: before=%d after=%d", before, len(m.tree.flat))
	}
}

// TestModel_SelectInTreeExpandsAncestors checks that navigating to a file
// inside a collapsed folder auto-expands it so the cursor lands on a
// visible row.
func TestModel_SelectInTreeExpandsAncestors(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	deep := filepath.Join(root, "notes", "sub", "deep.md")
	notesDir := filepath.Join(root, "notes")
	subDir := filepath.Join(root, "notes", "sub")

	// Force both ancestors collapsed.
	m.tree.expanded[notesDir] = false
	m.tree.expanded[subDir] = false
	m.tree.flat = m.flattenVisible()

	for _, row := range m.tree.flat {
		if row.node.Path == deep {
			t.Fatalf("precondition: deep.md should be hidden under collapsed parents")
		}
	}

	m.selectInTree(deep)

	if m.isCollapsed(notesDir) {
		t.Errorf("notes/ should be expanded after selectInTree(deep)")
	}
	if m.isCollapsed(subDir) {
		t.Errorf("notes/sub should be expanded after selectInTree(deep)")
	}
	if m.tree.cursor >= len(m.tree.flat) || m.tree.flat[m.tree.cursor].node.Path != deep {
		t.Errorf("cursor should land on deep.md, got row %d in tree of %d", m.tree.cursor, len(m.tree.flat))
	}
}

// TestModel_TreeScrollsToCursorOnTallTree checks that when the tree has
// more rows than the modal is tall, moving the cursor down past the
// visible window scrolls the modal's viewport so the cursor row stays visible.
func TestModel_TreeScrollsToCursorOnTallTree(t *testing.T) {
	root := writeTallFixture(t, 60)
	m := sized(t, root, "")
	// Open the modal so the tree viewport is sized.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.modals.kind != modalTree {
		t.Fatalf("^b should open tree modal")
	}

	if got := len(m.tree.flat); got <= m.tree.vp.Height {
		t.Fatalf("precondition: flatTree (%d rows) should exceed modal height (%d)", got, m.tree.vp.Height)
	}

	target := m.tree.vp.Height + 10
	if target >= len(m.tree.flat) {
		t.Fatalf("test setup: target row %d out of range (flatTree=%d)", target, len(m.tree.flat))
	}
	m = driveCursorTo(t, m, target)

	if m.tree.cursor != target {
		t.Fatalf("cursor at %d, expected %d", m.tree.cursor, target)
	}
	visibleTop := m.tree.vp.YOffset
	visibleBot := visibleTop + m.tree.vp.Height - 1
	if m.tree.cursor < visibleTop || m.tree.cursor > visibleBot {
		t.Errorf("cursor row %d outside visible window [%d, %d]", m.tree.cursor, visibleTop, visibleBot)
	}
}

// TestModel_TreeScrollsBackUp checks that scrolling the cursor back up
// past the top of the visible window scrolls the viewport back.
func TestModel_TreeScrollsBackUp(t *testing.T) {
	root := writeTallFixture(t, 60)
	m := sized(t, root, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.modals.kind != modalTree {
		t.Fatalf("^b should open tree modal")
	}

	m = driveCursorTo(t, m, m.tree.vp.Height+10)
	if m.tree.vp.YOffset == 0 {
		t.Fatalf("precondition: viewport should have scrolled down before this test")
	}
	m = driveCursorTo(t, m, 0)

	if m.tree.vp.YOffset != 0 {
		t.Errorf("YOffset should be 0 after scrolling back to row 0, got %d", m.tree.vp.YOffset)
	}
}

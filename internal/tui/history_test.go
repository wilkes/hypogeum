package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// openViaTree opens the tree modal, walks the cursor to path, and
// presses Enter to open it. Used to drive history without going
// through the link-following path.
func openViaTree(t *testing.T, m Model, path string) Model {
	t.Helper()
	if m.modals.kind != modalTree {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
		m = updated.(Model)
	}
	if m.modals.kind != modalTree {
		t.Fatalf("openViaTree: ^b should open tree modal, got kind=%v", m.modals.kind)
	}
	target := -1
	for i, row := range m.tree.flat {
		if row.node.Path == path {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("path %q not in flat tree", path)
	}
	for m.tree.cursor != target {
		var key tea.KeyMsg
		if m.tree.cursor < target {
			key = tea.KeyMsg{Type: tea.KeyDown}
		} else {
			key = tea.KeyMsg{Type: tea.KeyUp}
		}
		updated, _ := m.Update(key)
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(Model)
}

func TestModel_BackKeyReturnsToPreviousFile(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	indexPath := m.history.Current()
	firstPath := filepath.Join(root, "notes", "first.md")

	m = openViaTree(t, m, firstPath)
	if got := m.history.Current(); got != firstPath {
		t.Fatalf("after open, history.Current = %q, want %q", got, firstPath)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(Model)

	if got := m.history.Current(); got != indexPath {
		t.Errorf("after 'h', history.Current = %q, want %q", got, indexPath)
	}
	// Tree cursor should follow the back-navigation.
	if m.tree.flat[m.tree.cursor].node.Path != indexPath {
		t.Errorf("tree cursor did not follow back navigation: %q", m.tree.flat[m.tree.cursor].node.Path)
	}
}

func TestModel_ForwardKeyAfterBack(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	firstPath := filepath.Join(root, "notes", "first.md")

	m = openViaTree(t, m, firstPath)

	// Back, then forward.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(Model)

	if got := m.history.Current(); got != firstPath {
		t.Errorf("after h/l, history.Current = %q, want %q", got, firstPath)
	}
	if m.tree.flat[m.tree.cursor].node.Path != firstPath {
		t.Errorf("tree cursor did not follow forward navigation: %q", m.tree.flat[m.tree.cursor].node.Path)
	}
}

func TestModel_BackAtStartIsNoop(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	indexPath := m.history.Current()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(Model)

	if got := m.history.Current(); got != indexPath {
		t.Errorf("'h' at history start should be no-op; was %q, now %q", indexPath, got)
	}
}

func TestModel_ForwardAtEndIsNoop(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	indexPath := m.history.Current()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(Model)

	if got := m.history.Current(); got != indexPath {
		t.Errorf("'l' at history end should be no-op; was %q, now %q", indexPath, got)
	}
}

// TestModel_TabIsNoopWithoutBacklinks confirms that with no backlinks
// pane open, Tab has nowhere to go and is a clean no-op. The tree is
// not a Tab destination — it lives in a modal opened with ^b — so the
// cycle is only content ↔ backlinks when that pane is visible.
func TestModel_TabIsNoopWithoutBacklinks(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	if m.focus != focusContent {
		t.Fatalf("default focus = %v, want focusContent", m.focus)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusContent {
		t.Errorf("Tab without backlinks open: focus = %v, want focusContent (no-op)", m.focus)
	}
}

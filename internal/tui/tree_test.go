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
		t.Fatalf("first.md not found in flattened tree: %+v", m.flatTree)
	}
	m = driveCursorTo(t, m, target)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if got := m.history.Current(); got != want {
		t.Errorf("after Enter, history.Current = %q, want %q", got, want)
	}
}

// TestModel_ToggleTreeHidesPane checks that ^b hides the tree pane: the
// rendered View() drops tree row names and treeWidth() falls to 0 so the
// content pane gets the full width.
func TestModel_ToggleTreeHidesPane(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	if !m.treeVisible {
		t.Fatalf("tree should default to visible")
	}
	if w := m.treeWidth(); w == 0 {
		t.Fatalf("visible tree should have nonzero width, got 0")
	}
	beforeView := m.View()
	if !strings.Contains(beforeView, "notes") {
		t.Fatalf("expected tree pane to mention 'notes' before hide, got: %q", beforeView)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(Model)

	if m.treeVisible {
		t.Errorf("treeVisible should be false after ^b")
	}
	if w := m.treeWidth(); w != 0 {
		t.Errorf("hidden tree should have zero width, got %d", w)
	}
	if m.focus == focusTree {
		t.Errorf("focus should leave focusTree when tree is hidden")
	}
}

// TestModel_CollapseFolderHidesChildren checks that pressing space on a
// directory row removes its descendants from flatTree.
func TestModel_CollapseFolderHidesChildren(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	notesDir := filepath.Join(root, "notes")
	target := m.rowIndexByPath(notesDir)
	if target < 0 || !m.flatTree[target].node.IsDir {
		t.Fatalf("notes/ directory row not found in flatTree")
	}
	m = driveCursorTo(t, m, target)

	before := len(m.flatTree)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace})

	if !m.isCollapsed(notesDir) {
		t.Fatalf("notes/ should be collapsed after space")
	}
	if len(m.flatTree) >= before {
		t.Errorf("flatTree should shrink on collapse: before=%d after=%d", before, len(m.flatTree))
	}
	for _, row := range m.flatTree {
		if strings.HasPrefix(row.node.Path, notesDir+string(filepath.Separator)) {
			t.Errorf("collapsed descendant still in flatTree: %s", row.node.Path)
		}
	}

	// Toggling again restores the descendants.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if m.isCollapsed(notesDir) {
		t.Fatalf("notes/ should be expanded again")
	}
	if len(m.flatTree) != before {
		t.Errorf("flatTree should restore on re-expand: before=%d after=%d", before, len(m.flatTree))
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
	m.expanded[notesDir] = false
	m.expanded[subDir] = false
	m.flatTree = m.flattenVisible()

	for _, row := range m.flatTree {
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
	if m.treeCursor >= len(m.flatTree) || m.flatTree[m.treeCursor].node.Path != deep {
		t.Errorf("cursor should land on deep.md, got row %d in tree of %d", m.treeCursor, len(m.flatTree))
	}
}

// TestModel_TreeForceHiddenAt60Cols checks that below twoPaneMinWidth
// the tree pane is rendered as 0 cells wide, its row text doesn't
// appear in the View output, and focus snaps off the (now invisible)
// tree onto the content pane so keystrokes route somewhere visible.
func TestModel_TreeForceHiddenAt60Cols(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	if m.focus != focusTree {
		t.Fatalf("precondition: model defaults to focusTree, got %v", m.focus)
	}

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m = updated.(Model)

	if m.shouldShowTree() {
		t.Errorf("shouldShowTree() should be false at 60 cols")
	}
	if w := m.treeWidth(); w != 0 {
		t.Errorf("treeWidth() = %d at 60 cols, want 0", w)
	}
	if m.focus == focusTree {
		t.Errorf("focus should snap off focusTree when the tree is force-hidden")
	}
	// The tree pane renders directory rows with a chevron prefix; the
	// rendered content of index.md may contain "notes/" inside link
	// text, so we match the chevron+name shape that only appears in the
	// tree pane.
	if strings.Contains(m.View(), "▾ notes/") || strings.Contains(m.View(), "▸ notes/") {
		t.Errorf("View() should not contain tree row 'notes/' at 60 cols")
	}
}

// TestModel_TreeShownAtNarrowWidths checks that shouldShowTree() returns false
// when the terminal is narrower than twoPaneMinWidth even if the user
// has the tree visible — the threshold gates effective state.
func TestModel_TreeShownAtNarrowWidths(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	if !m.treeVisible {
		t.Fatalf("tree should default to visible")
	}

	cases := []struct {
		width int
		want  bool
	}{
		{60, false},
		{79, false},
		{80, true},
		{120, true},
	}
	for _, tc := range cases {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: tc.width, Height: 30})
		mm := updated.(Model)
		if got := mm.shouldShowTree(); got != tc.want {
			t.Errorf("width=%d: shouldShowTree() = %v, want %v", tc.width, got, tc.want)
		}
	}
}

// TestModel_ToggleTreeNarrowFlipsIntentOnly checks that ^b at a narrow
// terminal width flips treeVisible (so the user's preference survives
// resize) but doesn't change effective state — shouldShowTree stays false
// because the width gate fails.
func TestModel_ToggleTreeNarrowFlipsIntentOnly(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m = updated.(Model)
	if !m.treeVisible {
		t.Fatalf("precondition: treeVisible should still be true after a narrow resize")
	}
	if m.shouldShowTree() {
		t.Fatalf("precondition: shouldShowTree should be false at 60 cols")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(Model)
	if m.treeVisible {
		t.Errorf("treeVisible should be false after ^b")
	}
	if m.shouldShowTree() {
		t.Errorf("shouldShowTree should still be false at 60 cols")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(Model)
	if !m.treeVisible {
		t.Errorf("treeVisible should flip back to true on second ^b")
	}
	if m.shouldShowTree() {
		t.Errorf("shouldShowTree should still be false at 60 cols")
	}
}

// TestModel_TreeReturnsOnGrow checks that after a narrow resize hides
// the tree, growing the terminal back above the threshold restores it
// without any user interaction — m.treeVisible is preserved. Focus
// stays on content (where it was snapped during the narrow window);
// restoring it to the tree on grow would yank focus away from whatever
// the user was reading.
func TestModel_TreeReturnsOnGrow(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m = updated.(Model)
	if m.shouldShowTree() {
		t.Fatalf("precondition: shouldShowTree should be false at 60 cols")
	}
	if m.focus == focusTree {
		t.Fatalf("precondition: narrow resize should have snapped focus off the tree")
	}

	updated, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	if !m.shouldShowTree() {
		t.Errorf("shouldShowTree should be true after growing to 100 cols")
	}
	if w := m.treeWidth(); w == 0 {
		t.Errorf("treeWidth should be nonzero after growing to 100 cols")
	}
	if m.focus == focusTree {
		t.Errorf("focus should not be restored to tree on grow; stays where the user left it")
	}
}

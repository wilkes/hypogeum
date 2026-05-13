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
}

// TestModel_ToggleTreeShowsAndHidesPane checks that ^b reveals the tree
// (hidden by default) and a second ^b hides it again: treeWidth toggles
// between zero and nonzero, and the rendered View() gains/loses tree rows.
func TestModel_ToggleTreeShowsAndHidesPane(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	if m.tree.visible {
		t.Fatalf("tree should default to hidden")
	}
	if w := m.treeWidth(); w != 0 {
		t.Fatalf("hidden tree should have zero width, got %d", w)
	}

	// First ^b reveals the tree.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(Model)
	if !m.tree.visible {
		t.Fatalf("first ^b should reveal tree")
	}
	if w := m.treeWidth(); w == 0 {
		t.Errorf("visible tree should have nonzero width, got 0")
	}
	if !strings.Contains(m.View(), "notes") {
		t.Errorf("expected tree pane to mention 'notes' after reveal")
	}

	// Second ^b hides it again.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(Model)
	if m.tree.visible {
		t.Errorf("treeVisible should be false after second ^b")
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

// TestModel_TreeForceHiddenAt60Cols checks that below twoPaneMinWidth
// the tree pane is rendered as 0 cells wide, its row text doesn't
// appear in the View output, and focus snaps off the (now invisible)
// tree onto the content pane so keystrokes route somewhere visible.
func TestModel_TreeForceHiddenAt60Cols(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	// Reveal+focus the tree at full width so the narrow resize has
	// something to force-hide and snap focus away from.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusTree {
		t.Fatalf("precondition: tree should be focused after ^b+Tab, got %v", m.focus)
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

// TestModel_TreeScrollsToCursorOnTallTree checks that when the tree has
// more rows than the pane is tall, moving the cursor down past the
// visible window scrolls the tree pane so the cursor row stays visible.
// Without scrolling, lipgloss truncates from the top and the cursor
// disappears below the fold.
func TestModel_TreeScrollsToCursorOnTallTree(t *testing.T) {
	root := writeTallFixture(t, 60)
	m := sized(t, root, "")

	if got := len(m.tree.flat); got <= m.tree.vp.Height {
		t.Fatalf("precondition: flatTree (%d rows) should exceed pane height (%d)", got, m.tree.vp.Height)
	}

	// Drive the cursor down to a row well past the initial visible window.
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
// past the top of the visible window scrolls the viewport back. Without
// this, only the down direction scrolls and the cursor would go invisible
// above the viewport.
func TestModel_TreeScrollsBackUp(t *testing.T) {
	root := writeTallFixture(t, 60)
	m := sized(t, root, "")

	// Drive far down then back to row 0.
	m = driveCursorTo(t, m, m.tree.vp.Height+10)
	if m.tree.vp.YOffset == 0 {
		t.Fatalf("precondition: viewport should have scrolled down before this test")
	}
	m = driveCursorTo(t, m, 0)

	if m.tree.vp.YOffset != 0 {
		t.Errorf("YOffset should be 0 after scrolling back to row 0, got %d", m.tree.vp.YOffset)
	}
}

// TestModel_TreeShownAtNarrowWidths checks that shouldShowTree() returns false
// when the terminal is narrower than twoPaneMinWidth even if the user
// has the tree visible — the threshold gates effective state.
func TestModel_TreeShownAtNarrowWidths(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	// Reveal the tree (hidden by default) so the width-gate is what
	// suppresses display, not the user's intent.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if !m.tree.visible {
		t.Fatalf("^b should reveal tree")
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
	// Reveal the tree at full width so the precondition (intent=true) holds
	// after the narrow resize.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if !m.tree.visible {
		t.Fatalf("^b should reveal tree")
	}

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m = updated.(Model)
	if !m.tree.visible {
		t.Fatalf("precondition: treeVisible should still be true after a narrow resize")
	}
	if m.shouldShowTree() {
		t.Fatalf("precondition: shouldShowTree should be false at 60 cols")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(Model)
	if m.tree.visible {
		t.Errorf("treeVisible should be false after ^b")
	}
	if m.shouldShowTree() {
		t.Errorf("shouldShowTree should still be false at 60 cols")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(Model)
	if !m.tree.visible {
		t.Errorf("treeVisible should flip back to true on second ^b")
	}
	if m.shouldShowTree() {
		t.Errorf("shouldShowTree should still be false at 60 cols")
	}
}

// TestModel_TreeReturnsOnGrow checks that after a narrow resize hides
// the tree, growing the terminal back above the threshold restores it
// without any user interaction — m.tree.visible is preserved. Focus
// stays on content (where it was snapped during the narrow window);
// restoring it to the tree on grow would yank focus away from whatever
// the user was reading.
func TestModel_TreeReturnsOnGrow(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	// Reveal the tree at full width so the grow restores something
	// the user actually asked for.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if !m.tree.visible {
		t.Fatalf("^b should reveal tree")
	}

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

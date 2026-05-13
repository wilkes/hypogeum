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
	// Open the modal first (which calls expandAncestorsOf and clears the
	// map), then expand notes/ so first.md becomes reachable.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	m.tree.expanded[filepath.Join(root, "notes")] = true
	m.tree.flat = m.flattenVisible()
	m.refreshTreeVP()
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

// TestModel_ToggleFolderShowsAndHidesChildren checks that Space on a
// collapsed folder reveals its children, and a second Space hides them
// again. Default-collapsed means a fresh tree has all non-ancestor
// folders closed, so the first toggle is the expand, the second is the
// collapse.
func TestModel_ToggleFolderShowsAndHidesChildren(t *testing.T) {
	root := writeFixture(t)
	// Open on a top-level file so notes/ stays off the ancestor chain
	// (i.e. starts collapsed).
	m := sized(t, root, filepath.Join(root, "index.md"))

	notesDir := filepath.Join(root, "notes")
	target := m.rowIndexByPath(notesDir)
	if target < 0 || !m.tree.flat[target].node.IsDir {
		t.Fatalf("notes/ directory row not found in flatTree")
	}
	for _, row := range m.tree.flat {
		if strings.HasPrefix(row.node.Path, notesDir+string(filepath.Separator)) {
			t.Fatalf("notes/ children should be hidden by default, got %s", row.node.Path)
		}
	}
	before := len(m.tree.flat)

	// Drive cursor + Space to expand.
	m = driveCursorTo(t, m, target)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace})

	if !m.tree.expanded[notesDir] {
		t.Fatalf("notes/ should be expanded after first Space")
	}
	if len(m.tree.flat) <= before {
		t.Errorf("flatTree should grow on expand: before=%d after=%d", before, len(m.tree.flat))
	}

	// Second Space collapses.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if m.tree.expanded[notesDir] {
		t.Fatalf("notes/ should be collapsed after second Space")
	}
	if len(m.tree.flat) != before {
		t.Errorf("flatTree should return to original size after collapse: before=%d after=%d", before, len(m.tree.flat))
	}
	for _, row := range m.tree.flat {
		if strings.HasPrefix(row.node.Path, notesDir+string(filepath.Separator)) {
			t.Errorf("collapsed descendant still in flatTree: %s", row.node.Path)
		}
	}
}

// TestModel_ArrowKeysCollapseAndExpand checks that ← collapses an
// expanded directory and → expands a collapsed one, with both being
// no-ops when there is nothing to change or when the cursor is on a
// file row.
func TestModel_ArrowKeysCollapseAndExpand(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))

	notesDir := filepath.Join(root, "notes")
	target := m.rowIndexByPath(notesDir)
	if target < 0 {
		t.Fatalf("notes/ directory row not found in flatTree")
	}
	m = driveCursorTo(t, m, target)

	// → on collapsed dir expands it.
	if m.tree.expanded[notesDir] {
		t.Fatalf("precondition: notes/ should start collapsed")
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if !m.tree.expanded[notesDir] {
		t.Errorf("→ on collapsed notes/ should expand it")
	}

	// → on already-expanded dir is a no-op.
	flatLen := len(m.tree.flat)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if !m.tree.expanded[notesDir] {
		t.Errorf("→ on already-expanded notes/ should leave it expanded")
	}
	if len(m.tree.flat) != flatLen {
		t.Errorf("→ on already-expanded dir should not change flat size: was %d now %d", flatLen, len(m.tree.flat))
	}

	// ← on expanded dir collapses it.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.tree.expanded[notesDir] {
		t.Errorf("← on expanded notes/ should collapse it")
	}

	// ← on already-collapsed dir is a no-op.
	flatLen = len(m.tree.flat)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.tree.expanded[notesDir] {
		t.Errorf("← on already-collapsed notes/ should leave it collapsed")
	}
	if len(m.tree.flat) != flatLen {
		t.Errorf("← on already-collapsed dir should not change flat size: was %d now %d", flatLen, len(m.tree.flat))
	}
}

// TestModel_ArrowKeysAreNoopOnFileRow confirms that ← and → do nothing
// when the cursor is on a file row (not a directory).
func TestModel_ArrowKeysAreNoopOnFileRow(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))

	// Cursor lands on index.md after opening, which is a file row.
	indexPath := filepath.Join(root, "index.md")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.modals.kind != modalTree {
		t.Fatalf("setup: ^b should open tree modal")
	}
	target := m.rowIndexByPath(indexPath)
	if target < 0 {
		t.Fatalf("index.md not found in flat tree")
	}
	m.tree.cursor = target

	before := len(m.tree.flat)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if len(m.tree.flat) != before {
		t.Errorf("← on file row should not change flat size: was %d now %d", before, len(m.tree.flat))
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if len(m.tree.flat) != before {
		t.Errorf("→ on file row should not change flat size: was %d now %d", before, len(m.tree.flat))
	}
	// History must not change either — ← would otherwise be the global
	// Back binding, which would step out of the auto-opened index.md.
	if got := m.history.Current(); got != indexPath {
		t.Errorf("← in tree modal must not trigger history Back; current = %q, want %q", got, indexPath)
	}
}

// TestModel_ArrowKeysShadowHistoryWhileTreeModalOpen confirms that ←
// inside the tree modal does NOT trigger history Back, even when the
// cursor is on a file row (where ← is a no-op for collapse purposes).
// The modal's key dispatch runs before the global Back/Forward switch.
func TestModel_ArrowKeysShadowHistoryWhileTreeModalOpen(t *testing.T) {
	root := writeFixture(t)
	firstPath := filepath.Join(root, "notes", "first.md")
	m := sized(t, root, firstPath)

	// Open a second file to put something in history.
	indexPath := filepath.Join(root, "index.md")
	m.navigateTo(indexPath)
	if m.history.Current() != indexPath {
		t.Fatalf("setup: expected history at index.md")
	}

	// Open tree modal; ← should not navigate back to firstPath.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.history.Current() != indexPath {
		t.Errorf("← in tree modal triggered history Back; current = %q, want %q",
			m.history.Current(), indexPath)
	}
}

// TestModel_SelectInTreeExpandsAncestors checks that selectInTree
// expands every directory on the target file's ancestor chain so the
// cursor lands on a visible row, and collapses everything else.
func TestModel_SelectInTreeExpandsAncestors(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	deep := filepath.Join(root, "notes", "sub", "deep.md")
	notesDir := filepath.Join(root, "notes")
	subDir := filepath.Join(root, "notes", "sub")
	otherDir := filepath.Join(root, "other") // expanded by user before navigation

	// User manually expands an unrelated branch; selectInTree should
	// re-derive expansion state from the new file's chain only, so this
	// expansion must not survive the call.
	m.tree.expanded[otherDir] = true
	m.tree.flat = m.flattenVisible()

	m.selectInTree(deep)

	if !m.tree.expanded[notesDir] {
		t.Errorf("notes/ should be expanded after selectInTree(deep)")
	}
	if !m.tree.expanded[subDir] {
		t.Errorf("notes/sub should be expanded after selectInTree(deep)")
	}
	if m.tree.expanded[otherDir] {
		t.Errorf("other/ (off the ancestor chain) should be collapsed after selectInTree(deep)")
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

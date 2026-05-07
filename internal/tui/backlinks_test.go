package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wilkes/hypogeum/internal/vault"
)

func writeTUITestFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func TestBacklinksPaneShowsLinkers(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[b]] for more.")
	writeTUITestFile(t, dir, "b.md", "i am b.")

	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)
	bAbs := filepath.Join(dir, "b.md")
	m.openFile(bAbs)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = mm.(Model)

	rendered := m.renderBacklinks()
	if !strings.Contains(rendered, "a.md") {
		t.Fatalf("expected a.md in backlinks pane, got %q", rendered)
	}
}

func TestBacklinksPaneAutoCollapsesBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	m.backlinksOpen = true
	m.height = 15 // below threshold
	if m.shouldShowBacklinks() {
		t.Fatalf("expected backlinks suppressed at height %d", m.height)
	}
	m.height = 25
	if !m.shouldShowBacklinks() {
		t.Fatalf("expected backlinks visible at height %d", m.height)
	}
}

func TestBacklinksModalToggleAndEsc(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[b]].")
	writeTUITestFile(t, dir, "b.md", "i am b.")

	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)
	m.openFile(filepath.Join(dir, "b.md"))

	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	if out.(Model).modalOpen != modalBacklinks {
		t.Fatalf("after B: expected modalBacklinks, got %v", out.(Model).modalOpen)
	}

	out2, _ := out.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if out2.(Model).modalOpen != modalNone {
		t.Fatalf("after Esc: expected modalNone, got %v", out2.(Model).modalOpen)
	}
}

func TestBacklinksPane_CursorMovement(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "b.md", "also [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	m.openFile(filepath.Join(dir, "c.md"))

	// Open backlinks pane (b). Subsequent task wires focus; for now we
	// only need backlinks populated and the input router to dispatch
	// j/k to the pane handler when focus is focusBacklinks.
	m = pressRune(t, m, 'b')
	if m.focus != focusBacklinks {
		t.Fatalf("expected focusBacklinks after b, got %v", m.focus)
	}
	if len(m.backlinks) != 2 {
		t.Fatalf("expected 2 backlinks, got %d", len(m.backlinks))
	}
	if m.backlinkCursor != 0 {
		t.Fatalf("expected cursor to start at 0, got %d", m.backlinkCursor)
	}

	m = pressRune(t, m, 'j')
	if m.backlinkCursor != 1 {
		t.Fatalf("expected cursor=1 after j, got %d", m.backlinkCursor)
	}

	// j past the end clamps.
	m = pressRune(t, m, 'j')
	if m.backlinkCursor != 1 {
		t.Fatalf("expected cursor=1 (clamped) after j at end, got %d", m.backlinkCursor)
	}

	// k moves up.
	m = pressRune(t, m, 'k')
	if m.backlinkCursor != 0 {
		t.Fatalf("expected cursor=0 after k, got %d", m.backlinkCursor)
	}

	// k past the start clamps.
	m = pressRune(t, m, 'k')
	if m.backlinkCursor != 0 {
		t.Fatalf("expected cursor=0 (clamped) after k at start, got %d", m.backlinkCursor)
	}
}

func TestFormatBacklinks_HighlightsSelectedRow(t *testing.T) {
	links := []vault.Backlink{
		{SourceFile: "/r/a.md", DisplayText: "x", Snippet: "hello", Line: 1},
		{SourceFile: "/r/b.md", DisplayText: "x", Snippet: "world", Line: 2},
	}
	rendered := formatBacklinks(links, "/r", 80, 1)
	if !strings.Contains(rendered, "▌") {
		t.Fatalf("expected cursor marker '▌' in output, got %q", rendered)
	}
	lines := strings.Split(rendered, "\n")
	var sawMarkerOnA, sawMarkerOnB bool
	for _, line := range lines {
		if strings.Contains(line, "a.md") && strings.Contains(line, "▌") {
			sawMarkerOnA = true
		}
		if strings.Contains(line, "b.md") && strings.Contains(line, "▌") {
			sawMarkerOnB = true
		}
	}
	if sawMarkerOnA {
		t.Fatalf("marker should NOT be on a.md row")
	}
	if !sawMarkerOnB {
		t.Fatalf("marker SHOULD be on b.md row")
	}
}

func TestBacklinksPane_EnterFollows(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "blah blah\n\nsee [[c]] in here.\n")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	aAbs := filepath.Join(dir, "a.md")
	m.openFile(cAbs)

	m = pressRune(t, m, 'b')
	if len(m.backlinks) != 1 {
		t.Fatalf("expected 1 backlink, got %d", len(m.backlinks))
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	// Should have navigated to a.md.
	if m.history.Current() != aAbs {
		t.Fatalf("expected current=%s, got %s", aAbs, m.history.Current())
	}
	// Focus should be on content (we left the backlinks surface).
	if m.focus != focusContent {
		t.Fatalf("expected focusContent after Enter, got %v", m.focus)
	}
	// returnCursor should be set with sourceFile=cAbs.
	if m.returnCursor == nil {
		t.Fatalf("expected returnCursor set, got nil")
	}
	if m.returnCursor.sourceFile != cAbs {
		t.Fatalf("expected returnCursor.sourceFile=%s, got %s", cAbs, m.returnCursor.sourceFile)
	}
	if m.returnCursor.cursor != 0 {
		t.Fatalf("expected returnCursor.cursor=0, got %d", m.returnCursor.cursor)
	}
	if m.returnCursor.surface != surfacePane {
		t.Fatalf("expected returnCursor.surface=surfacePane, got %v", m.returnCursor.surface)
	}
}

func TestBacklinksModal_CursorAndEnter(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "b.md", "also [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	m.openFile(cAbs)

	// Open backlinks modal.
	m = pressRune(t, m, 'B')
	if m.modalOpen != modalBacklinks {
		t.Fatalf("expected modalBacklinks, got %v", m.modalOpen)
	}
	if len(m.backlinks) != 2 {
		t.Fatalf("expected 2 backlinks, got %d", len(m.backlinks))
	}
	if m.backlinkCursor != 0 {
		t.Fatalf("expected cursor=0, got %d", m.backlinkCursor)
	}

	// j moves cursor in modal.
	m = pressRune(t, m, 'j')
	if m.backlinkCursor != 1 {
		t.Fatalf("expected cursor=1 after j in modal, got %d", m.backlinkCursor)
	}

	// Enter follows AND closes the modal.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.modalOpen != modalNone {
		t.Fatalf("expected modal closed after Enter, got %v", m.modalOpen)
	}
	if m.focus != focusContent {
		t.Fatalf("expected focusContent after Enter, got %v", m.focus)
	}
	if m.returnCursor == nil || m.returnCursor.surface != surfaceModal {
		t.Fatalf("expected returnCursor.surface=surfaceModal, got %+v", m.returnCursor)
	}
}

func TestScrollToLine_PositionsLineNearTop(t *testing.T) {
	dir := t.TempDir()
	// Build a 100-paragraph file so the viewport has somewhere to scroll.
	// Paragraphs (blank-line separated) are needed because Glamour folds
	// consecutive non-blank lines into a single wrapped paragraph.
	var sb strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&sb, "line %d\n\n", i)
	}
	writeTUITestFile(t, dir, "long.md", sb.String())

	m := sized(t, dir, filepath.Join(dir, "long.md"))
	// Initially YOffset = 0.
	if m.viewport.YOffset != 0 {
		t.Fatalf("expected YOffset=0 initially, got %d", m.viewport.YOffset)
	}

	// Scroll to line 60. Expect YOffset to leave ~25% padding above:
	//   target = 60 - viewportHeight*0.25
	// With viewportHeight ≈ 32 (height 40 - 4 for borders/footer - 4 misc),
	// target ≈ 60 - 8 = 52. Be lenient: assert YOffset is in [40, 56].
	m.scrollToLine(60)
	if m.viewport.YOffset < 40 || m.viewport.YOffset > 56 {
		t.Fatalf("expected YOffset in [40, 56] after scrollToLine(60), got %d", m.viewport.YOffset)
	}

	// scrollToLine(huge) clamps to last line.
	m.scrollToLine(99999)
	maxYOffset := m.viewport.TotalLineCount() - m.viewport.Height
	if maxYOffset < 0 {
		maxYOffset = 0
	}
	if m.viewport.YOffset > maxYOffset {
		t.Fatalf("expected YOffset clamped to max %d, got %d", maxYOffset, m.viewport.YOffset)
	}
}

func TestBacklinksPane_BackRestoresCursor(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "b.md", "also [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	m.openFile(cAbs)
	m = pressRune(t, m, 'b')          // open pane
	m = pressRune(t, m, 'j')          // cursor → 1
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // follow

	// Now we're on a.md or b.md. Press h.
	m = pressRune(t, m, 'h')

	if m.history.Current() != cAbs {
		t.Fatalf("expected back at c.md, got %s", m.history.Current())
	}
	if m.backlinkCursor != 1 {
		t.Fatalf("expected backlinkCursor restored to 1, got %d", m.backlinkCursor)
	}
	if m.focus != focusBacklinks {
		t.Fatalf("expected focusBacklinks restored, got %v", m.focus)
	}
	if m.returnCursor != nil {
		t.Fatalf("expected returnCursor cleared, got %+v", m.returnCursor)
	}
}

func TestReturnCursor_DiscardedOnUnrelatedNav(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")
	writeTUITestFile(t, dir, "d.md", "i am d, unrelated.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	dAbs := filepath.Join(dir, "d.md")
	m.openFile(cAbs)
	m = pressRune(t, m, 'b')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // follow → a.md

	// Now jump to an unrelated file via openFile (simulates tree click).
	m.openFile(dAbs)

	// Press h. We should land back on a.md, NOT on c.md.
	m = pressRune(t, m, 'h')
	if m.history.Current() == cAbs {
		t.Fatalf("expected to be on a.md (one back from d.md), got c.md")
	}

	// Step beyond: explicit unrelated nav DOES NOT pre-empt the slot.
	// The slot is consumed only on path-match Back. This test asserts
	// the more interesting case: openFile to d.md did NOT consume the
	// slot, so navigating Back twice eventually still restores.
	if m.returnCursor == nil {
		t.Fatalf("returnCursor unexpectedly cleared by unrelated nav (only matching Back should clear it)")
	}
}

func TestBacklinksModal_BackReopensModal(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "b.md", "also [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	m.openFile(cAbs)
	m = pressRune(t, m, 'B')          // open modal
	m = pressRune(t, m, 'j')          // cursor → 1
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // follow + close modal
	if m.modalOpen != modalNone {
		t.Fatalf("expected modal closed during follow, got %v", m.modalOpen)
	}

	m = pressRune(t, m, 'h')

	if m.modalOpen != modalBacklinks {
		t.Fatalf("expected modalBacklinks reopened on Back, got %v", m.modalOpen)
	}
	if m.backlinkCursor != 1 {
		t.Fatalf("expected cursor=1 restored, got %d", m.backlinkCursor)
	}
	if m.returnCursor != nil {
		t.Fatalf("expected returnCursor cleared, got %+v", m.returnCursor)
	}
}

func TestReturnCursor_ClampsToShrunkList(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "b.md", "also [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	bAbs := filepath.Join(dir, "b.md")
	m.openFile(cAbs)
	m = pressRune(t, m, 'b')
	m = pressRune(t, m, 'j')          // cursor → 1
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // follow

	// Simulate b.md being deleted (and the vault refreshing) while we're
	// away on the source file. Easiest path: rewrite b.md to drop its
	// link, then call vault.RefreshFile.
	if err := os.WriteFile(bAbs, []byte("no link anymore"), 0o644); err != nil {
		t.Fatalf("rewrite b.md: %v", err)
	}
	if err := m.vault.RefreshFile(bAbs); err != nil {
		t.Fatalf("vault.RefreshFile: %v", err)
	}

	// Now Back. The vault will report only 1 backlink for c.md; cursor
	// must clamp from 1 down to 0.
	m = pressRune(t, m, 'h')

	if m.backlinkCursor != 0 {
		t.Fatalf("expected cursor clamped to 0 after list shrank, got %d", m.backlinkCursor)
	}
	if len(m.backlinks) != 1 {
		t.Fatalf("expected 1 backlink after refresh, got %d", len(m.backlinks))
	}
}

func TestEsc_RestoresFocusFromBacklinksWithoutClosingPane(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	m.openFile(filepath.Join(dir, "c.md"))
	m = pressRune(t, m, 'b')
	if m.focus != focusBacklinks || !m.backlinksOpen {
		t.Fatalf("setup: expected focusBacklinks and pane open, got focus=%v open=%v", m.focus, m.backlinksOpen)
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.focus == focusBacklinks {
		t.Fatalf("Esc should restore prevFocus, but focus is still focusBacklinks")
	}
	if !m.backlinksOpen {
		t.Fatalf("Esc should NOT close the pane")
	}
}

func TestTab_ThreeWayCycleWhenPaneVisible(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	m.openFile(filepath.Join(dir, "c.md"))
	m = pressRune(t, m, 'b')          // pane open, focus on backlinks
	if m.focus != focusBacklinks {
		t.Fatalf("setup: expected focusBacklinks, got %v", m.focus)
	}

	// Tab: backlinks → tree.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusTree {
		t.Fatalf("Tab from backlinks: expected focusTree, got %v", m.focus)
	}

	// Tab: tree → content.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusContent {
		t.Fatalf("Tab from tree: expected focusContent, got %v", m.focus)
	}

	// Tab: content → backlinks (pane is visible).
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusBacklinks {
		t.Fatalf("Tab from content: expected focusBacklinks, got %v", m.focus)
	}
}

func TestTab_TwoWayWhenPaneClosed(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "hi.")
	m := sized(t, dir, "")
	m.openFile(filepath.Join(dir, "a.md"))

	// Pane closed (default). Cycle: tree ↔ content.
	if m.focus != focusTree {
		t.Fatalf("default focus should be tree, got %v", m.focus)
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusContent {
		t.Fatalf("expected focusContent, got %v", m.focus)
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != focusTree {
		t.Fatalf("expected focusTree (skipping invisible backlinks), got %v", m.focus)
	}
}

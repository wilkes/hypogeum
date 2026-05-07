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

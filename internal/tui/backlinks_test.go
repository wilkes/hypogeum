package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wilkes/hypogeum/internal/markdown"
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

func TestBacklinksModalToggleAndEsc(t *testing.T) {
	isolatedHome(t)
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[b]].")
	writeTUITestFile(t, dir, "b.md", "i am b.")

	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)
	m.openFile(filepath.Join(dir, "b.md"))

	// `b` opens the modal.
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if out.(Model).modals.kind != modalBacklinks {
		t.Fatalf("after b: expected modalBacklinks, got %v", out.(Model).modals.kind)
	}

	// Esc closes it.
	out2, _ := out.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if out2.(Model).modals.kind != modalNone {
		t.Fatalf("after Esc: expected modalNone, got %v", out2.(Model).modals.kind)
	}

	// Second `b` re-toggles closed (same as Esc).
	out3, _ := out.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if out3.(Model).modals.kind != modalNone {
		t.Fatalf("after second b: expected modalNone, got %v", out3.(Model).modals.kind)
	}
}

func TestBacklinksModalShowsLinkers(t *testing.T) {
	isolatedHome(t)
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

	m = pressRune(t, m, 'b')
	if m.modals.kind != modalBacklinks {
		t.Fatalf("expected modalBacklinks open, got %v", m.modals.kind)
	}
	rendered := m.modals.vp.View()
	if !strings.Contains(rendered, "a.md") {
		t.Fatalf("expected a.md in backlinks modal, got %q", rendered)
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

func TestBacklinksModal_CursorAndEnter(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[c]].")
	writeTUITestFile(t, dir, "b.md", "also [[c]].")
	writeTUITestFile(t, dir, "c.md", "i am c.")

	m := sized(t, dir, "")
	cAbs := filepath.Join(dir, "c.md")
	m.openFile(cAbs)

	m = pressRune(t, m, 'b')
	if m.modals.kind != modalBacklinks {
		t.Fatalf("expected modalBacklinks, got %v", m.modals.kind)
	}
	if len(m.backlinks.items) != 2 {
		t.Fatalf("expected 2 backlinks, got %d", len(m.backlinks.items))
	}
	if m.backlinks.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", m.backlinks.cursor)
	}

	// j moves cursor in modal.
	m = pressRune(t, m, 'j')
	if m.backlinks.cursor != 1 {
		t.Fatalf("expected cursor=1 after j in modal, got %d", m.backlinks.cursor)
	}

	// Enter follows AND closes the modal.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.modals.kind != modalNone {
		t.Fatalf("expected modal closed after Enter, got %v", m.modals.kind)
	}
	if m.focus != focusContent {
		t.Fatalf("expected focusContent after Enter, got %v", m.focus)
	}
	if m.backlinks.returnCursor == nil {
		t.Fatalf("expected returnCursor set, got nil")
	}
	if m.backlinks.returnCursor.cursor != 1 {
		t.Fatalf("expected returnCursor.cursor=1, got %d", m.backlinks.returnCursor.cursor)
	}
	if m.backlinks.returnCursor.sourceFile != cAbs {
		t.Fatalf("expected returnCursor.sourceFile=%s, got %s", cAbs, m.backlinks.returnCursor.sourceFile)
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
	if m.content.viewport.YOffset != 0 {
		t.Fatalf("expected YOffset=0 initially, got %d", m.content.viewport.YOffset)
	}

	m.scrollToLine(60)
	if m.content.viewport.YOffset < 40 || m.content.viewport.YOffset > 56 {
		t.Fatalf("expected YOffset in [40, 56] after scrollToLine(60), got %d", m.content.viewport.YOffset)
	}

	m.scrollToLine(99999)
	maxYOffset := m.content.viewport.TotalLineCount() - m.content.viewport.Height
	if maxYOffset < 0 {
		maxYOffset = 0
	}
	if m.content.viewport.YOffset > maxYOffset {
		t.Fatalf("expected YOffset clamped to max %d, got %d", maxYOffset, m.content.viewport.YOffset)
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

	// Now jump to an unrelated file (simulates picker selection).
	m.openFile(dAbs)

	// Press h. We should land back on a.md, NOT on c.md.
	m = pressRune(t, m, 'h')
	if m.history.Current() == cAbs {
		t.Fatalf("expected to be on a.md (one back from d.md), got c.md")
	}

	// The slot is consumed only on path-match Back. openFile to d.md
	// did NOT consume it, so it's still pending.
	if m.backlinks.returnCursor == nil {
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
	m = pressRune(t, m, 'b')                           // open modal
	m = pressRune(t, m, 'j')                           // cursor → 1
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // follow + close
	if m.modals.kind != modalNone {
		t.Fatalf("expected modal closed during follow, got %v", m.modals.kind)
	}

	m = pressRune(t, m, 'h')

	if m.modals.kind != modalBacklinks {
		t.Fatalf("expected modalBacklinks reopened on Back, got %v", m.modals.kind)
	}
	if m.backlinks.cursor != 1 {
		t.Fatalf("expected cursor=1 restored, got %d", m.backlinks.cursor)
	}
	if m.backlinks.returnCursor != nil {
		t.Fatalf("expected returnCursor cleared, got %+v", m.backlinks.returnCursor)
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
	m = pressRune(t, m, 'j')                           // cursor → 1
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // follow

	// Simulate b.md being rewritten to drop its link while we're away
	// on the source file. Vault refresh reflects the new state.
	if err := os.WriteFile(bAbs, []byte("no link anymore"), 0o644); err != nil {
		t.Fatalf("rewrite b.md: %v", err)
	}
	if err := m.vault.RefreshFile(bAbs); err != nil {
		t.Fatalf("vault.RefreshFile: %v", err)
	}

	// Now Back. The vault will report only 1 backlink for c.md; cursor
	// must clamp from 1 down to 0.
	m = pressRune(t, m, 'h')

	if m.backlinks.cursor != 0 {
		t.Fatalf("expected cursor clamped to 0 after list shrank, got %d", m.backlinks.cursor)
	}
	if len(m.backlinks.items) != 1 {
		t.Fatalf("expected 1 backlink after refresh, got %d", len(m.backlinks.items))
	}
}

func TestFollowBacklink_CapturesPendingPreselectRange(t *testing.T) {
	// Verify followBacklink mirrors Back/Forward's capture of
	// m.content.rangeHighlight into m.pending.preselectRange so the
	// destination's refreshContent can disambiguate between multiple
	// links to the same target (one with a #L range, one without).
	//
	// The user-facing scenario (a code file with rangeHighlight that
	// also has backlinks) is unreachable through vault.Build today —
	// vault only indexes markdown files. We construct an all-markdown
	// fixture that exercises the same disambiguation path: source.md
	// has two links to dest.md, one with #L10-L20 and one plain;
	// pre-setting rangeHighlight on dest.md before followBacklink must
	// route the disambiguation to the ranged link.
	dir := t.TempDir()
	writeTUITestFile(t, dir, "source.md", "ranged [a](dest.md#L10-L20) and plain [b](dest.md).")
	writeTUITestFile(t, dir, "dest.md", "i am dest.")

	m := sized(t, dir, "")
	destAbs := filepath.Join(dir, "dest.md")
	srcAbs := filepath.Join(dir, "source.md")
	m.openFile(destAbs)

	// Simulate the unreachable-via-UI state: an active range highlight
	// on the current file while a backlink modal is open. The capture
	// in followBacklink should mirror this into pending.preselectRange.
	m.content.rangeHighlight = &markdown.LineRange{Start: 10, End: 20}
	m = pressRune(t, m, 'b')
	if len(m.backlinks.items) < 1 {
		t.Fatalf("expected at least 1 backlink (source.md → dest.md), got %d", len(m.backlinks.items))
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.history.Current() != srcAbs {
		t.Fatalf("expected on source.md after follow, got %q", m.history.Current())
	}
	if m.content.linkCursor < 0 {
		t.Fatalf("expected linkCursor preselected after follow, got %d", m.content.linkCursor)
	}
	// The pre-selected link must be the *ranged* one, proving that
	// pending.preselectRange was captured and used to disambiguate.
	got := m.content.links[m.content.linkCursor].Resolved.Range
	if got == nil || got.Start != 10 || got.End != 20 {
		t.Fatalf("expected ranged link pre-selected (Range=10-20); got Range=%+v", got)
	}
}

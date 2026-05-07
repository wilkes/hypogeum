package tui

import (
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

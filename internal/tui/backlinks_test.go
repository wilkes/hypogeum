package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// writeTallContentFixture creates a single markdown file long enough to
// require scrolling at a 40-line terminal, so HalfViewDown/HalfViewUp /
// GotoTop / GotoBottom actually move the viewport. Returns the root and
// the absolute path to the tall file (pass as initialFile to sized()).
func writeTallContentFixture(t *testing.T) (root, initial string) {
	t.Helper()
	root = t.TempDir()
	var b strings.Builder
	b.WriteString("# Tall\n\n")
	for i := 0; i < 200; i++ {
		b.WriteString("paragraph ")
		b.WriteString(strings.Repeat("x", 3))
		b.WriteString("\n\n")
	}
	rel := "tall.md"
	full := filepath.Join(root, rel)
	if err := os.WriteFile(full, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, full
}

// TestModel_GotoTop_Pager asserts `g` scrolls the content viewport to top.
func TestModel_GotoTop_Pager(t *testing.T) {
	root, initial := writeTallContentFixture(t)
	m := sized(t, root, initial)
	m.content.viewport.SetYOffset(10)
	m = pressRune(t, m, 'g')
	if got := m.content.viewport.YOffset; got != 0 {
		t.Errorf("YOffset after g = %d, want 0", got)
	}
}

// TestModel_GotoBottom_Pager asserts `G` scrolls the content viewport to bottom.
func TestModel_GotoBottom_Pager(t *testing.T) {
	root, initial := writeTallContentFixture(t)
	m := sized(t, root, initial)
	m = pressRune(t, m, 'G')
	if got := m.content.viewport.YOffset; got != m.content.viewport.TotalLineCount()-m.content.viewport.Height && !m.content.viewport.AtBottom() {
		t.Errorf("not at bottom: YOffset=%d, total=%d, height=%d",
			got, m.content.viewport.TotalLineCount(), m.content.viewport.Height)
	}
}

// TestModel_HalfPageDown_Pager asserts ^d advances half a viewport.
func TestModel_HalfPageDown_Pager(t *testing.T) {
	root, initial := writeTallContentFixture(t)
	m := sized(t, root, initial)
	startOffset := m.content.viewport.YOffset
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	delta := m.content.viewport.YOffset - startOffset
	half := m.content.viewport.Height / 2
	if delta < half-1 || delta > half+1 {
		t.Errorf("^d advanced by %d lines, want ~%d (height/2)", delta, half)
	}
}

// TestModel_HalfPageUp_Pager asserts ^u retreats half a viewport.
func TestModel_HalfPageUp_Pager(t *testing.T) {
	root, initial := writeTallContentFixture(t)
	m := sized(t, root, initial)
	m.content.viewport.SetYOffset(20)
	startOffset := m.content.viewport.YOffset
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	delta := startOffset - m.content.viewport.YOffset
	half := m.content.viewport.Height / 2
	if delta < half-1 || delta > half+1 {
		t.Errorf("^u retreated by %d lines, want ~%d (height/2)", delta, half)
	}
}

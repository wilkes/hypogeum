package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// writeFixture lays down a small markdown directory and returns its root.
func writeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"index.md":          "# Index\n\nSee [first](notes/first.md) and [external](https://x.test).\n",
		"notes/first.md":    "# First\n\nHello.\n",
		"notes/sub/deep.md": "# Deep\n\nNested.\n",
	}
	for rel, body := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// sized returns a model that has received an initial size message, so that
// View() produces real output rather than the empty pre-resize string.
func sized(t *testing.T, root, initialFile string) Model {
	t.Helper()
	m, err := New(root, initialFile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model)
}

// switchToContent presses Tab to move focus to the content pane.
// Used as a setup step for link-cursor tests.
func switchToContent(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusContent {
		t.Fatalf("expected focusContent after Tab, got %v", m.focus)
	}
	return m
}

// leftClick builds a tea.MouseMsg representing a left-button press at (x, y).
func leftClick(x, y int) tea.MouseMsg {
	return tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}
}

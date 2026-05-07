package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
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
// It also calls renderAndScan to populate BubbleZone bounds so tests that
// synthesize mouse clicks find their zones.
func sized(t *testing.T, root, initialFile string) Model {
	t.Helper()
	m, err := New(root, initialFile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	renderAndScan(t, m, zoneContentPane)
	return m
}

// renderAndScan calls View() and waits for BubbleZone's worker goroutine
// to record zone bounds. Use waitID as a sentinel zone — the function
// returns once that zone has non-zero bounds (or the deadline expires).
//
// BubbleZone's Scan is async: it pushes to a channel and the worker
// goroutine writes the zone map. Tests that synthesize a click directly
// after View() can race the worker and see empty bounds. Polling for a
// known zone is the cheapest reliable sync without exporting internals.
func renderAndScan(t *testing.T, m Model, waitID string) {
	t.Helper()
	_ = m.View() // triggers zone.Scan
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !zone.Get(waitID).IsZero() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("zone %q never registered after View()", waitID)
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

// pressKey sends a single character key (or a special key for non-rune
// keys) through the model's Update loop and returns the new model.
// For a rune like 'b' or 'j': pass `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}`.
// For special keys: use Type: tea.KeyEnter, tea.KeyEsc, tea.KeyTab, etc.
func pressKey(t *testing.T, m Model, msg tea.KeyMsg) Model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

// pressRune is shorthand for pressKey with a single rune.
func pressRune(t *testing.T, m Model, r rune) Model {
	t.Helper()
	return pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
}

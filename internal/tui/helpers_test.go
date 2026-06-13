package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/muesli/termenv"
)

// TestMain initialises package-level state needed by tests:
//   - Forces lipgloss to emit ANSI colour codes even outside a TTY so that
//     tests which assert on ANSI output (e.g. picker highlight) work correctly.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	os.Exit(m.Run())
}

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

// writeTallFixture lays down n flat markdown files (file000.md, file001.md, …)
// at root, used to exercise tree-pane scrolling when the row count exceeds
// terminal height.
func writeTallFixture(t *testing.T, n int) string {
	t.Helper()
	root := t.TempDir()
	for i := 0; i < n; i++ {
		full := filepath.Join(root, fmt.Sprintf("file%03d.md", i))
		if err := os.WriteFile(full, []byte("# "+filepath.Base(full)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// isolatedHome redirects $HOME and $XDG_CONFIG_HOME to a tempdir so that
// recent.DefaultStateFile() resolves to a scratch location and tests don't
// pollute the developer's real ~/Library/Application Support/hypogeum/visits.json
// (or ~/.config/hypogeum/visits.json on Linux). t.Setenv rolls back at test end.
func isolatedHome(t *testing.T) {
	t.Helper()
	d := t.TempDir()
	t.Setenv("HOME", d)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(d, ".config"))
}

// sized returns a model that has received an initial size message, so that
// View() produces real output rather than the empty pre-resize string.
// It also calls renderAndScan to populate BubbleZone bounds so tests that
// synthesize mouse clicks find their zones.
func sized(t *testing.T, root, initialFile string) Model {
	t.Helper()
	isolatedHome(t)
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

// switchToContent ensures focus is on the content pane. With the
// backlinks pane removed, focus is now a single-value enum and this is
// always a no-op — kept for callsite stability across the many tests
// that invoke it as a setup step.
func switchToContent(t *testing.T, m Model) Model {
	t.Helper()
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

// driveCursorTo presses up/down until m.tree.cursor reaches target, failing
// the test if the cursor ever fails to advance (a stuck cursor would
// otherwise loop forever).
//
// Opens the tree modal first — the modal is the only surface that
// routes KeyDown/KeyUp to the tree cursor.
func driveCursorTo(t *testing.T, m Model, target int) Model {
	t.Helper()
	if m.modals.kind != modalTree {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
		m = updated.(Model)
	}
	if m.modals.kind != modalTree {
		t.Fatalf("driveCursorTo: t should open tree modal, got kind=%v", m.modals.kind)
	}
	for m.tree.cursor != target {
		key := tea.KeyMsg{Type: tea.KeyDown}
		if m.tree.cursor > target {
			key = tea.KeyMsg{Type: tea.KeyUp}
		}
		prev := m.tree.cursor
		m = pressKey(t, m, key)
		if m.tree.cursor == prev {
			t.Fatalf("cursor stuck at %d trying to reach %d", prev, target)
		}
	}
	return m
}

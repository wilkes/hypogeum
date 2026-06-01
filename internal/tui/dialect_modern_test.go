package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// modernModel constructs a sized model wired with the modern keymap.
func modernModel(t *testing.T, root string) Model {
	t.Helper()
	return sizedWithOptions(t, root, "", Options{Dialect: "modern"})
}

// modernModelTall constructs a sized model wired with the modern keymap,
// using a content fixture tall enough to exercise scroll-based bindings
// (Ctrl+Home, PageDown).
func modernModelTall(t *testing.T) Model {
	t.Helper()
	root, initial := writeTallContentFixture(t)
	return sizedWithOptions(t, root, initial, Options{Dialect: "modern"})
}

func TestModel_BackForward_Modern(t *testing.T) {
	root := writeFixture(t)
	m := modernModel(t, root)

	// Navigate to a second file so Back has somewhere to go.
	m.navigateTo(root + "/notes/first.md")
	prevTop := m.history.Current()

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
	if got := m.history.Current(); got == prevTop {
		t.Errorf("Alt+← did not navigate back: current still %q", got)
	}
	prevTop = m.history.Current()
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	if got := m.history.Current(); got == prevTop {
		t.Errorf("Alt+→ did not navigate forward: current still %q", got)
	}
}

func TestModel_OpenSearch_Modern(t *testing.T) {
	root := writeFixture(t)
	m := modernModel(t, root)

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlF})
	if m.modals.kind != modalSearch {
		t.Errorf("Ctrl+F did not open search modal; kind=%v", m.modals.kind)
	}
}

func TestModel_OpenBacklinks_Modern(t *testing.T) {
	root := writeFixture(t)
	m := modernModel(t, root)

	// Alt+b is encoded as a rune-bearing KeyMsg with Alt=true.
	m = pressKey(t, m, tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'b'},
		Alt:   true,
	})
	if m.modals.kind != modalBacklinks {
		t.Errorf("Alt+b did not open backlinks modal; kind=%v", m.modals.kind)
	}
}

func TestModel_OpenLogs_Modern(t *testing.T) {
	root := writeFixture(t)
	m := modernModel(t, root)

	m = pressKey(t, m, tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'l'},
		Alt:   true,
	})
	if m.modals.kind != modalLogs {
		t.Errorf("Alt+l did not open logs modal; kind=%v", m.modals.kind)
	}
}

func TestModel_LinkCycle_Modern(t *testing.T) {
	root := writeFixture(t)
	m := modernModel(t, root)

	// cycleLink (links.go) wraps and never re-enters the sentinel -1 state
	// once cycling has started, so a Tab→Shift+Tab round trip from cursor=-1
	// lands on the *last* link, not back at -1. Instead, advance to a known
	// post-Tab position, take a second Tab, then verify Shift+Tab undoes the
	// last step — that's the inverse-step property the plan is pinning.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	afterFirstTab := m.content.linkCursor
	if afterFirstTab < 0 {
		t.Fatalf("Tab did not advance link cursor (still %d)", afterFirstTab)
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.content.linkCursor == afterFirstTab {
		t.Errorf("second Tab did not advance link cursor (still %d)", afterFirstTab)
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.content.linkCursor != afterFirstTab {
		t.Errorf("Shift+Tab did not undo the last Tab; got %d, want %d",
			m.content.linkCursor, afterFirstTab)
	}
}

func TestModel_QuitBothBindings_Modern(t *testing.T) {
	root := writeFixture(t)

	// Each q variant — bare 'q' and Ctrl+Q — must yield a tea.Quit cmd.
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyCtrlQ},
	} {
		m := modernModel(t, root)
		_, cmd := m.Update(key)
		if cmd == nil {
			t.Errorf("modern quit (%v) did not return a command", key)
			continue
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("modern quit (%v) did not produce QuitMsg; got %T", key, msg)
		}
	}
}

func TestModel_GotoTop_Modern(t *testing.T) {
	m := modernModelTall(t)
	m.content.viewport.SetYOffset(10)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlHome})
	if got := m.content.viewport.YOffset; got != 0 {
		t.Errorf("Ctrl+Home did not goto top: YOffset=%d", got)
	}
}

func TestModel_PageDown_Modern(t *testing.T) {
	m := modernModelTall(t)
	startOffset := m.content.viewport.YOffset
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.content.viewport.YOffset == startOffset {
		t.Errorf("PageDown did not advance; YOffset still %d", startOffset)
	}
}

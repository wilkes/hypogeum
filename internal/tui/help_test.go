package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// pressQuestion synthesizes the `?` keypress.
func pressQuestion() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}
}

// TestHelpModalOpensOnQuestionMark verifies `?` opens the help modal and
// the body lists representative bindings, including the moved `^l logs`
// label so a regression that drops it is caught.
func TestHelpModalOpensOnQuestionMark(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	out, _ := m.Update(pressQuestion())
	got := out.(Model)
	if got.modalOpen != modalHelp {
		t.Fatalf("after ?: expected modalHelp, got %v", got.modalOpen)
	}

	// Assert against the full formatted help text, not modalVP.View() —
	// the viewport only shows the rows currently scrolled into view, and
	// at typical terminal sizes the bottom of the cheat sheet is below
	// the fold. The source of truth is formatHelp(m.keys).
	body := formatHelp(got.keys)
	wantSubstrings := []string{
		"Navigation", "Tree", "Links", "Modals", // section headers
		"^l", "logs", // moved logs binding
		"?", "help", // help itself
		"^p", "open file", // picker
		"h/←", "back",
		"esc", "clear link",
	}
	for _, s := range wantSubstrings {
		if !strings.Contains(body, s) {
			t.Errorf("help body missing %q\nbody:\n%s", s, body)
		}
	}
}

// TestHelpModalEscCloses verifies Esc dismisses the help modal, just
// like every other modal.
func TestHelpModalEscCloses(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	out, _ := m.Update(pressQuestion())
	m = out.(Model)
	if m.modalOpen != modalHelp {
		t.Fatalf("precondition: help modal should be open")
	}

	out, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := out.(Model).modalOpen; got != modalNone {
		t.Errorf("Esc should close help modal; modalOpen = %v", got)
	}
}

// TestHelpModalDoesNotSwap verifies that pressing `?` while another
// modal is open is a no-op — help is anchored, unlike B and ^l which
// swap content under the single-modal-swap rule. This prevents `?` from
// stealing focus when the user is mid-task in another modal.
func TestHelpModalDoesNotSwap(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	// Open backlinks modal first.
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	m = out.(Model)
	if m.modalOpen != modalBacklinks {
		t.Fatalf("precondition: backlinks modal should be open")
	}

	// Press `?`. Expect: backlinks stays, help does NOT open.
	out, _ = m.Update(pressQuestion())
	if got := out.(Model).modalOpen; got != modalBacklinks {
		t.Errorf("? should be a no-op while backlinks modal is open; got %v", got)
	}
}

// TestHelpModalTogglesClosed verifies pressing `?` while the help modal
// is already open closes it — same toggle behavior as every other
// modal-toggle key.
func TestHelpModalTogglesClosed(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	out, _ := m.Update(pressQuestion())
	m = out.(Model)
	if m.modalOpen != modalHelp {
		t.Fatalf("precondition: help modal should be open")
	}

	out, _ = m.Update(pressQuestion())
	if got := out.(Model).modalOpen; got != modalNone {
		t.Errorf("? while help is open should close it; got %v", got)
	}
}

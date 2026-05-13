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
	isolatedHome(t)
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	out, _ := m.Update(pressQuestion())
	got := out.(Model)
	if got.modals.kind != modalHelp {
		t.Fatalf("after ?: expected modalHelp, got %v", got.modals.kind)
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
	isolatedHome(t)
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	out, _ := m.Update(pressQuestion())
	m = out.(Model)
	if m.modals.kind != modalHelp {
		t.Fatalf("precondition: help modal should be open")
	}

	out, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := out.(Model).modals.kind; got != modalNone {
		t.Errorf("Esc should close help modal; modalOpen = %v", got)
	}
}

// modalCases enumerates the three non-help modals and the keypress that
// opens each. Shared by TestHelpModalDoesNotSwap and
// TestHelpModalSwapsToOtherModals — both tests need to exercise help
// against every other modal.
var modalCases = []struct {
	name string
	key  tea.KeyMsg
	want modalKind
}{
	{"backlinks", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}}, modalBacklinks},
	{"logs", ctrlL(), modalLogs},
	{"picker", tea.KeyMsg{Type: tea.KeyCtrlP}, modalPicker},
}

// TestHelpModalDoesNotSwap verifies that pressing `?` while another
// modal is open is a no-op — help is anchored, unlike B and ^l which
// swap content under the single-modal-swap rule. This prevents `?` from
// stealing focus when the user is mid-task in another modal.
//
// The three sub-cases each pin a distinct way the guard could regress:
// a refactor narrowing the condition to a single == comparison would
// only catch one and silently break the others.
func TestHelpModalDoesNotSwap(t *testing.T) {
	for _, tc := range modalCases {
		t.Run(tc.name, func(t *testing.T) {
			m := sized(t, t.TempDir(), "")

			out, _ := m.Update(tc.key)
			m = out.(Model)
			if m.modals.kind != tc.want {
				t.Fatalf("precondition: expected %v open, got %v", tc.want, m.modals.kind)
			}

			out, _ = m.Update(pressQuestion())
			if got := out.(Model).modals.kind; got != tc.want {
				t.Errorf("? should be a no-op while %s modal is open; got %v", tc.name, got)
			}
		})
	}
}

// TestHelpModalSwapsToOtherModals verifies the *other* direction of the
// anchor asymmetry: opening help and then pressing B/^l/^p should swap
// to the new modal (because those handlers don't gate on modalHelp).
// Without this test, someone copying the anchor guard to the swap
// handlers ("be consistent with help") would silently break the
// "user can read the cheat sheet then jump straight into a task"
// flow — they'd be stuck having to Esc out of help first.
func TestHelpModalSwapsToOtherModals(t *testing.T) {
	for _, tc := range modalCases {
		t.Run(tc.name, func(t *testing.T) {
			m := sized(t, t.TempDir(), "")

			out, _ := m.Update(pressQuestion())
			m = out.(Model)
			if m.modals.kind != modalHelp {
				t.Fatalf("precondition: help modal should be open")
			}

			out, _ = m.Update(tc.key)
			if got := out.(Model).modals.kind; got != tc.want {
				t.Errorf("expected %v after swap from help, got %v", tc.want, got)
			}
		})
	}
}

// TestFooterAdvertisesHelp pins the trimmed footer's two essential hints
// — the entry point to the cheat sheet (`?`) and the way out (`q`).
// Without this, a future "let's drop `?: help` because power users know
// it" change would silently destroy discoverability of every other
// binding, since the cheat sheet is now the only place they're listed.
func TestFooterAdvertisesHelp(t *testing.T) {
	m := sized(t, t.TempDir(), "")

	rendered := m.renderFooter()
	for _, want := range []string{"?: help", "q: quit"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("footer missing %q\nfooter:\n%s", want, rendered)
		}
	}
}

// TestHelpModalTogglesClosed verifies pressing `?` while the help modal
// is already open closes it — same toggle behavior as every other
// modal-toggle key.
func TestHelpModalTogglesClosed(t *testing.T) {
	isolatedHome(t)
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	out, _ := m.Update(pressQuestion())
	m = out.(Model)
	if m.modals.kind != modalHelp {
		t.Fatalf("precondition: help modal should be open")
	}

	out, _ = m.Update(pressQuestion())
	if got := out.(Model).modals.kind; got != modalNone {
		t.Errorf("? while help is open should close it; got %v", got)
	}
}

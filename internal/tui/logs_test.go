package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ctrlL synthesizes a Ctrl+L keypress. Bubble Tea models Ctrl+letter as
// tea.KeyCtrlL (a distinct Type), not as a rune.
func ctrlL() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlL} }

func TestLogsModalShowsRingBuffer(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	m.diag.Warn("a problem occurred")

	out, _ := m.Update(ctrlL())
	mm2 := out.(Model)
	if mm2.modalOpen != modalLogs {
		t.Fatalf("after ^l: expected modalLogs, got %v", mm2.modalOpen)
	}
	rendered := mm2.modalVP.View()
	if !strings.Contains(rendered, "a problem occurred") {
		t.Fatalf("expected log entry in modal: %q", rendered)
	}
}

// TestLogsModalReplacesBacklinksModal verifies the single-modal-swap
// invariant still holds for B and ^l (the two "swap" modals). Help (?)
// is anchored and does not swap — see TestHelpModalDoesNotSwap.
func TestLogsModalReplacesBacklinksModal(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	out1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	if out1.(Model).modalOpen != modalBacklinks {
		t.Fatalf("expected modalBacklinks")
	}
	out2, _ := out1.(Model).Update(ctrlL())
	if out2.(Model).modalOpen != modalLogs {
		t.Fatalf("expected modalLogs after ^l, got %v", out2.(Model).modalOpen)
	}
}

package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLogsModalShowsRingBuffer(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	m.diag.Warn("a problem occurred")

	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	mm2 := out.(Model)
	if mm2.modalOpen != modalLogs {
		t.Fatalf("after ?: expected modalLogs, got %v", mm2.modalOpen)
	}
	rendered := mm2.modalVP.View()
	if !strings.Contains(rendered, "a problem occurred") {
		t.Fatalf("expected log entry in modal: %q", rendered)
	}
}

func TestLogsModalReplacesBacklinksModal(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	out1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	if out1.(Model).modalOpen != modalBacklinks {
		t.Fatalf("expected modalBacklinks")
	}
	out2, _ := out1.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if out2.(Model).modalOpen != modalLogs {
		t.Fatalf("expected modalLogs after ?, got %v", out2.(Model).modalOpen)
	}
}

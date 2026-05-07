package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// drainCmd executes cmd and feeds its message back into m.Update,
// returning the resulting model and any follow-up cmd. Filepicker uses
// async commands for directory reads, so tests need to drain them.
func drainCmd(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return m, nil
	}
	msg := cmd()
	if msg == nil {
		return m, nil
	}
	updated, next := m.Update(msg)
	return updated.(Model), next
}

// drainAllCmds repeatedly drains until cmd returns nil. Bounds the loop
// so a misbehaving picker can't hang the test.
func drainAllCmds(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for i := 0; i < 16 && cmd != nil; i++ {
		m, cmd = drainCmd(t, m, cmd)
	}
	return m
}

func TestModel_OpenPickerModal(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	if m.modalOpen != modalNone {
		t.Fatalf("precondition: no modal should be open")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(Model)

	if m.modalOpen != modalPicker {
		t.Errorf("modalOpen = %v, want modalPicker", m.modalOpen)
	}
	if m.picker.CurrentDirectory != root {
		t.Errorf("picker.CurrentDirectory = %q, want %q", m.picker.CurrentDirectory, root)
	}
}

func TestModel_PickerEscAtRootCloses(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(Model)
	if m.modalOpen != modalPicker {
		t.Fatalf("precondition: picker should be open")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.modalOpen != modalNone {
		t.Errorf("Esc at root should close picker; modalOpen = %v", m.modalOpen)
	}
}

// TestModel_PickerSelectOpensFile drives the picker through Ctrl+P → down
// (past the notes/ directory) → enter, and asserts the selected file
// becomes the new history entry.
func TestModel_PickerSelectOpensFile(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	// Open the picker. Init returns a Cmd that lazily reads the directory;
	// drain it so m.picker.files is populated before we synthesize Enter.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(Model)
	m = drainAllCmds(t, m, cmd)

	// Fixture root sorts as: notes/ (dir, first), then index.md.
	// One Down moves the cursor onto index.md.
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	m = drainAllCmds(t, m, cmd)

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	m = drainAllCmds(t, m, cmd)

	want := filepath.Join(root, "index.md")
	if got := m.history.Current(); got != want {
		t.Errorf("history.Current = %q, want %q", got, want)
	}
	if m.modalOpen != modalNone {
		t.Errorf("picker should close after selection; modalOpen = %v", m.modalOpen)
	}
}

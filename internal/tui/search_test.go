package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// minimal smoke test that ^s opens the modal. Fuller behavior covered in later tasks.
func TestSearch_CtrlSOpensModal(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A\n")
	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.modals.kind != modalSearch {
		t.Errorf("modals.kind = %v, want modalSearch", m.modals.kind)
	}
}

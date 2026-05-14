package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wilkes/hypogeum/internal/search"
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

func TestSearch_TypingShortQueryDoesNotFire(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A\nfoobar\n")
	m := sized(t, dir, "")
	// Open the search modal.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.modals.kind != modalSearch {
		t.Fatalf("expected modalSearch, got %v", m.modals.kind)
	}
	// Type one character — below the minimum query length.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	if got := m.modals.search.input.Value(); got != "a" {
		t.Errorf("input.Value() = %q, want %q", got, "a")
	}
	if cmd != nil {
		t.Errorf("expected nil cmd for 1-char query (below minimum), got non-nil")
	}
	if m.modals.search.inFlight {
		t.Errorf("inFlight should be false for short query")
	}
}

func TestSearch_TypingTwoCharsSchedulesTick(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A\nfoobar\n")
	m := sized(t, dir, "")
	// Open the search modal.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.modals.kind != modalSearch {
		t.Fatalf("expected modalSearch, got %v", m.modals.kind)
	}
	// Type first character — still below minimum.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	// Type second character — now at minimum length; a tick should be scheduled.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = updated.(Model)
	if got := m.modals.search.input.Value(); got != "fo" {
		t.Errorf("input.Value() = %q, want %q", got, "fo")
	}
	if cmd == nil {
		t.Errorf("expected non-nil cmd (tick scheduled) for 2-char query, got nil")
	}
}

func TestSearch_HitsRenderAsPathPlusSnippet(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "note.md")
	if err := os.WriteFile(p, []byte("line one\nline with foo here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	m.modals.kind = modalSearch
	m.modals.search.input.SetValue("foo")
	m.modals.search.paths = []string{p}
	m.modals.search.hits = []search.Hit{
		{Path: p, Line: 2, Snippet: "line with \x11foo\x12 here"},
	}
	m.modals.search.cursor = 0
	m.resizeSearch()

	out := m.renderSearchRows()
	if !strings.Contains(out, "note.md:2") {
		t.Errorf("expected path:line in output, got: %q", out)
	}
	if !strings.Contains(out, "foo") {
		t.Errorf("expected snippet text in output, got: %q", out)
	}
}

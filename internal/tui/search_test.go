package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestSearch_CursorDownAndUp(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.modals.kind = modalSearch
	m.modals.search.hits = []search.Hit{
		{Path: "/x/a.md", Line: 1, Snippet: "a"},
		{Path: "/x/b.md", Line: 1, Snippet: "b"},
		{Path: "/x/c.md", Line: 1, Snippet: "c"},
	}

	// ^j moves down
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlJ})
	mm := updated.(Model)
	if mm.modals.search.cursor != 1 {
		t.Errorf("cursor = %d after ^j, want 1", mm.modals.search.cursor)
	}
	// ^k moves up
	updated, _ = mm.handleKey(tea.KeyMsg{Type: tea.KeyCtrlK})
	mm = updated.(Model)
	if mm.modals.search.cursor != 0 {
		t.Errorf("cursor = %d after ^k, want 0", mm.modals.search.cursor)
	}
	// Don't overshoot at boundaries
	updated, _ = mm.handleKey(tea.KeyMsg{Type: tea.KeyCtrlK})
	mm = updated.(Model)
	if mm.modals.search.cursor != 0 {
		t.Errorf("cursor = %d after ^k at top, want 0", mm.modals.search.cursor)
	}
}

func TestSearch_EscClearsQueryThenCloses(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.modals.kind = modalSearch
	m.modals.search.input.SetValue("foo")

	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(Model)
	if mm.modals.kind != modalSearch {
		t.Errorf("first Esc should not close, kind = %v", mm.modals.kind)
	}
	if mm.modals.search.input.Value() != "" {
		t.Errorf("first Esc should clear query, got %q", mm.modals.search.input.Value())
	}

	updated, _ = mm.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	mm = updated.(Model)
	if mm.modals.kind != modalNone {
		t.Errorf("second Esc should close modal, kind = %v", mm.modals.kind)
	}
}

func TestSearch_EnterNavigatesAndScrolls(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.md")
	var sb strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&sb, "line %d\n\n", i)
	}
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)

	m.modals.kind = modalSearch
	m.modals.search.hits = []search.Hit{
		{Path: p, Line: 50, Snippet: "line 50"},
	}
	m.modals.search.cursor = 0

	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.modals.kind != modalNone {
		t.Errorf("Enter should close modal, kind = %v", mm.modals.kind)
	}
	if mm.history.Current() != p {
		t.Errorf("Current = %q, want %q", mm.history.Current(), p)
	}
	if mm.content.viewport.YOffset == 0 {
		t.Errorf("Expected viewport scrolled, YOffset = 0")
	}
}

func TestSearch_RecencyRerank(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	if err := os.WriteFile(a, []byte("alpha needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("bravo needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Visit b first so it scores higher in recency than a, then a so a
	// is the most-recent — final order should put a first.
	m.openFile(b)
	time.Sleep(2 * time.Millisecond)
	m.openFile(a)

	// Synthesize search results in alphabetical input order: a, b.
	hits := []search.Hit{
		{Path: a, Line: 1, Snippet: "alpha \x11needle\x12"},
		{Path: b, Line: 1, Snippet: "bravo \x11needle\x12"},
	}
	reranked := rerankByRecency(m.recent, hits)
	if len(reranked) != 2 {
		t.Fatalf("got %d hits, want 2", len(reranked))
	}
	if reranked[0].Path != a {
		t.Errorf("reranked[0].Path = %q, want %q (most recent)", reranked[0].Path, a)
	}
}

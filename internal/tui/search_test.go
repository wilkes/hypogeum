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

// minimal smoke test that / opens the modal. Fuller behavior covered in later tasks.
func TestSearch_SlashOpensModal(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A\n")
	m := sized(t, dir, "")
	m = pressRune(t, m, '/')
	if m.modals.kind != modalSearch {
		t.Errorf("modals.kind = %v, want modalSearch", m.modals.kind)
	}
}

func TestSearch_TypingShortQueryDoesNotFire(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A\nfoobar\n")
	m := sized(t, dir, "")
	// Open the search modal.
	m = pressRune(t, m, '/')
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
	m = pressRune(t, m, '/')
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

func TestSearch_MTimeRerank(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	if err := os.WriteFile(a, []byte("alpha needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("bravo needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Search re-rank is now edit-recency (mtime), not visit history: make a
	// the most recently edited file so it ranks first regardless of visits.
	base := time.Now()
	if err := os.Chtimes(b, base.Add(-2*time.Hour), base.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(a, base.Add(-1*time.Hour), base.Add(-1*time.Hour)); err != nil {
		t.Fatal(err)
	}

	// Synthesize search results in input order b, a to prove re-rank sorts.
	hits := []search.Hit{
		{Path: b, Line: 1, Snippet: "bravo \x11needle\x12"},
		{Path: a, Line: 1, Snippet: "alpha \x11needle\x12"},
	}
	reranked := rerankByMTime(hits)
	if len(reranked) != 2 {
		t.Fatalf("got %d hits, want 2", len(reranked))
	}
	if reranked[0].Path != a {
		t.Errorf("reranked[0].Path = %q, want %q (most recently edited)", reranked[0].Path, a)
	}
}

// Regression: opening the modal with paths populated should not show
// "(no markdown files in vault)" — that branch is for a truly empty
// vault. The pre-fix bug came from resizeSearch caching the empty-
// vault placeholder before the modal was opened.
func TestSearch_InitialOpenShowsEmptyQueryHint(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A\n")
	m := sized(t, dir, "")
	m = pressRune(t, m, '/')
	body := m.searchView()
	if strings.Contains(body, "no markdown files in vault") {
		t.Errorf("first open showed (no markdown files in vault) despite paths being populated:\n%s", body)
	}
	if !strings.Contains(body, "type 2+ chars to search") {
		t.Errorf("first open did not show the type-more hint:\n%s", body)
	}
}

// Regression: Backspace must edit the textinput query inside the
// search modal. Prior to the fix, only KeyRunes were routed to the
// textinput so Backspace fell through to the global handler and the
// query couldn't be edited.
func TestSearch_BackspaceEditsQuery(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A\nfoobar\n")
	m := sized(t, dir, "")
	m = pressRune(t, m, '/')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fool")})
	if got := m.modals.search.input.Value(); got != "fool" {
		t.Fatalf("setup: input=%q want %q", got, "fool")
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	if got := m.modals.search.input.Value(); got != "foo" {
		t.Errorf("after backspace: input=%q want %q", got, "foo")
	}
}

// Regression: the prompt line ("> " + textinput View()) must fit
// inside the modal interior width. The textinput's cursor block
// renders one column past the visible value, so reserving only the
// "> " prefix (pw-2) leaves the line 1 char wider than the modal —
// which can wrap onto the next row under some render conditions and
// produce a stack of duplicate prompts in the modal.
func TestSearch_PromptFitsModalInterior(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A\n")
	cases := []struct{ w, h int }{
		{100, 30},
		{120, 40},
		{162, 40},
	}
	for _, c := range cases {
		m := newTestModelAtSize(t, dir, "", c.w, c.h)
		m = pressRune(t, m, '/')
		m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("xxxxxxxx")})
		_, _, mw, _ := modalGeometry(c.w, c.h)
		interior := mw - 2
		body := m.searchView()
		first := strings.SplitN(body, "\n", 2)[0]
		visible := visibleWidth(first)
		if visible > interior {
			t.Errorf("term %dx%d: prompt visible width %d > modal interior %d",
				c.w, c.h, visible, interior)
		}
	}
}

// Regression: the prompt must be exactly one row tall, regardless of
// the query length. A wrapped prompt under rapid-typing render cycles
// is what produced the stacked-prompts bug. searchView wraps the
// prompt in lipgloss MaxHeight(1) to enforce this; the test exercises
// pathological inputs (100+ char queries) to ensure the clamp holds.
func TestSearch_PromptStaysSingleRow(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A\n")
	queries := []string{"x", "footer af",
		"a very long query string that almost certainly exceeds the modal interior",
		strings.Repeat("y", 200)}
	for _, q := range queries {
		m := newTestModelAtSize(t, dir, "", 100, 30)
		m = pressRune(t, m, '/')
		m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(q)})
		body := m.searchView()
		lines := strings.Split(body, "\n")
		if len(lines) < 2 {
			t.Fatalf("q=%q: searchView returned %d lines, want >= 2", q, len(lines))
		}
		// Line 1 must be the separator ─. If the prompt wrapped, line 1
		// would still be prompt content.
		if !strings.HasPrefix(lines[1], "─") {
			t.Errorf("q=%q: line[1] is not the separator (prompt wrapped):\n  line[0]=%q\n  line[1]=%q",
				q, lines[0], lines[1])
		}
	}
}

func newTestModelAtSize(t *testing.T, dir, initial string, w, h int) Model {
	t.Helper()
	m, err := New(dir, initial)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(Model)
}

func visibleWidth(s string) int {
	count := 0
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		count++
	}
	return count
}

// Regression: when the query changes, prior hits must disappear from
// the viewport immediately — not linger until the next scan returns.
// Prior to the fix, typing more characters after results landed kept
// the old hits visible alongside whatever the next render produced.
func TestSearch_QueryChangeClearsStaleHits(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	writePickerFile(t, p, "# A\nfoobar\n")
	m := sized(t, dir, "")
	m = pressRune(t, m, '/')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fo")})
	// Inject a result for "fo".
	updated, _ := m.Update(searchResultsMsg{
		query: "fo",
		hits:  []search.Hit{{Path: p, Line: 2, Snippet: "foobar"}},
	})
	m = updated.(Model)
	if len(m.modals.search.hits) != 1 {
		t.Fatalf("setup: want 1 hit, got %d", len(m.modals.search.hits))
	}
	// Now type more characters — hits must clear immediately.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ol")})
	if got := len(m.modals.search.hits); got != 0 {
		t.Errorf("after query change: hits=%d want 0 (stale hits not cleared)", got)
	}
}

package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wilkes/hypogeum/internal/search"
)

// TestRenderBar pins the proportional-bar math: it must always emit exactly
// `width` visible runes (no overflow / off-by-one), zero out an empty result,
// floor any nonzero count to at least one eighth, and fill solid at the max.
func TestRenderBar(t *testing.T) {
	const w = 8
	cases := []struct {
		count, max int
		want       string
	}{
		{100, 100, "████████"},  // count == max → solid
		{1, 4, "████    "},      // sqrt(1/4)=0.5 → 4 full cells
		{0, 5, "        "},      // zero count → empty
		{5, 0, "        "},      // zero max → empty
		{1, 100000, "▏       "}, // sqrt rounds to 0 → floored to one eighth (▏)
	}
	for _, c := range cases {
		got := renderBar(c.count, c.max, w)
		if n := utf8.RuneCountInString(got); n != w {
			t.Errorf("renderBar(%d,%d,%d) = %q has %d runes, want %d", c.count, c.max, w, got, n, w)
		}
		if got != c.want {
			t.Errorf("renderBar(%d,%d,%d) = %q, want %q", c.count, c.max, w, got, c.want)
		}
	}
	// Property: across the whole count range the bar is always exactly w runes
	// — never overflows or under-fills, even when count > max.
	for n := 0; n <= 250; n++ {
		got := renderBar(n, 200, w)
		if rc := utf8.RuneCountInString(got); rc != w {
			t.Fatalf("renderBar(%d,200,%d) = %q has %d runes, want %d", n, w, got, rc, w)
		}
	}
}

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

// TestSearch_ModalToggleKeyTypedIntoSearchFilters is the search-modal twin of
// the picker's printable-keys-grab regression guard: pressing a key that is a
// global modal toggle (`r` opens the recent modal, `b` the backlinks modal)
// while the search modal is open must type into the query, NOT swap modals.
func TestSearch_ModalToggleKeyTypedIntoSearchFilters(t *testing.T) {
	for _, r := range []rune{'r', 'b'} {
		t.Run(string(r), func(t *testing.T) {
			dir := t.TempDir()
			writePickerFile(t, filepath.Join(dir, "a.md"), "# A\n")
			m := sized(t, dir, "")
			m = pressRune(t, m, '/')
			if m.modals.kind != modalSearch {
				t.Fatalf("/ should open search, got %v", m.modals.kind)
			}

			m = pressRune(t, m, r)

			if m.modals.kind != modalSearch {
				t.Fatalf("typing %q in search swapped to modal kind %v; it must stay in search", r, m.modals.kind)
			}
			if got := m.modals.search.input.Value(); got != string(r) {
				t.Errorf("search query after typing %q: got %q, want %q", r, got, string(r))
			}
		})
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

func TestSearch_FilesRenderWithCountThenSnippetOnExpand(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "note.md")
	if err := os.WriteFile(p, []byte("line one\nline with foo here\nanother foo line\n"), 0o644); err != nil {
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
	m.modals.search.files = []search.FileMatches{
		{Path: p, Count: 2, Lines: []search.Line{
			{Num: 2, Text: "line with foo here", At: 10, Len: 3},
			{Num: 3, Text: "another foo line", At: 8, Len: 3},
		}},
	}
	m.modals.search.expanded = map[string]bool{}
	m.modals.search.flatten()
	m.modals.search.cursor = 0
	m.resizeSearch()

	// Collapsed: one row showing the file path and its match count, but no
	// snippet text yet (snippets are lazy).
	out := m.renderSearchRows()
	if !strings.Contains(out, "note.md") {
		t.Errorf("expected file path in collapsed output, got: %q", out)
	}
	if strings.Contains(out, "here") {
		t.Errorf("collapsed file should not render a snippet, got: %q", out)
	}

	// Expand → the matches' snippets appear.
	m.modals.search.expanded[p] = true
	m.modals.search.flatten()
	out = m.renderSearchRows()
	if !strings.Contains(out, "here") {
		t.Errorf("expanded file should render snippet text, got: %q", out)
	}
}

func TestSearch_CursorDownAndUp(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	m.modals.kind = modalSearch
	m.modals.search.files = []search.FileMatches{
		{Path: "/x/a.md", Count: 1, Lines: []search.Line{{Num: 1}}},
		{Path: "/x/b.md", Count: 1, Lines: []search.Line{{Num: 1}}},
		{Path: "/x/c.md", Count: 1, Lines: []search.Line{{Num: 1}}},
	}
	m.modals.search.expanded = map[string]bool{}
	m.modals.search.flatten()

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

// helper: a search modal seeded with one file of n matches, all collapsed.
func searchModelWithFile(t *testing.T, dir, path string, n int) Model {
	t.Helper()
	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	m.modals.kind = modalSearch
	lines := make([]search.Line, n)
	for i := range lines {
		lines[i] = search.Line{Num: i + 1, Text: fmt.Sprintf("match %d", i+1), At: 0, Len: 5}
	}
	m.modals.search.files = []search.FileMatches{{Path: path, Count: n, Lines: lines}}
	m.modals.search.expanded = map[string]bool{}
	m.modals.search.flatten()
	return m
}

func TestSearch_TabExpandsAndCollapses(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "note.md")
	m := searchModelWithFile(t, dir, p, 3)

	if len(m.modals.search.rows) != 1 {
		t.Fatalf("collapsed: want 1 row, got %d", len(m.modals.search.rows))
	}
	// Tab expands → 1 file row + 3 match rows.
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if got := len(m.modals.search.rows); got != 4 {
		t.Fatalf("expanded: want 4 rows, got %d", got)
	}
	if !m.modals.search.expanded[p] {
		t.Errorf("expanded[%q] = false, want true", p)
	}
	// Cursor stays on the file header (row 0).
	if m.modals.search.cursor != 0 {
		t.Errorf("cursor = %d after expand, want 0 (stays on file)", m.modals.search.cursor)
	}
	// Tab again collapses.
	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if len(m.modals.search.rows) != 1 {
		t.Errorf("re-collapsed: want 1 row, got %d", len(m.modals.search.rows))
	}
}

func TestSearch_EnterOnMatchRowNavigatesToThatLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "note.md")
	var sb strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&sb, "line %d\n\n", i) // blank-separated so each is its own paragraph
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
	// Two matches: a shallow one (line 2) and a deep one (line 50). Selecting
	// the deep match row must scroll the destination there, proving Enter
	// follows the specific match — not just the file's first hit.
	m.modals.search.files = []search.FileMatches{
		{Path: p, Count: 2, Lines: []search.Line{
			{Num: 2, Text: "line 2", At: 0, Len: 4},
			{Num: 50, Text: "line 50", At: 0, Len: 4},
		}},
	}
	m.modals.search.expanded = map[string]bool{}
	m.modals.search.flatten()

	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyTab}) // expand
	m = updated.(Model)
	m.modals.search.moveCursor(1) // -> first match (line 2)
	m.modals.search.moveCursor(1) // -> second match (line 50)
	r := m.modals.search.rows[m.modals.search.cursor]
	if r.kind != rowMatch || m.modals.search.files[0].Lines[r.line].Num != 50 {
		t.Fatalf("cursor not on the line-50 match: %+v", r)
	}

	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.modals.kind != modalNone {
		t.Errorf("Enter should close modal, kind = %v", m.modals.kind)
	}
	if m.history.Current() != p {
		t.Errorf("Current = %q, want %q", m.history.Current(), p)
	}
	if m.content.viewport.YOffset == 0 {
		t.Errorf("expected viewport scrolled to the deep match, YOffset = 0")
	}
}

func TestSearch_OverflowRowNotSelectable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "note.md")
	n := searchExpandedMatchCap + 5 // forces an overflow rowMore
	m := searchModelWithFile(t, dir, p, n)

	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab}) // expand
	m = updated.(Model)
	// rows: 1 file + searchExpandedMatchCap matches + 1 overflow.
	wantRows := 1 + searchExpandedMatchCap + 1
	if got := len(m.modals.search.rows); got != wantRows {
		t.Fatalf("rows = %d, want %d", got, wantRows)
	}
	last := m.modals.search.rows[len(m.modals.search.rows)-1]
	if last.kind != rowMore || last.more != 5 {
		t.Fatalf("last row = %+v, want rowMore with more=5", last)
	}
	// Drive the cursor to the bottom; it must never land on the rowMore.
	for i := 0; i < wantRows+2; i++ {
		m.modals.search.moveCursor(1)
		if m.modals.search.rows[m.modals.search.cursor].kind == rowMore {
			t.Fatalf("cursor landed on a non-navigable overflow row at index %d", m.modals.search.cursor)
		}
	}
	// It should rest on the last match row (index wantRows-2).
	if m.modals.search.cursor != wantRows-2 {
		t.Errorf("cursor = %d, want %d (last match row)", m.modals.search.cursor, wantRows-2)
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
	m.modals.search.files = []search.FileMatches{
		{Path: p, Count: 1, Lines: []search.Line{{Num: 50, Text: "line 50", At: 0, Len: 4}}},
	}
	m.modals.search.expanded = map[string]bool{}
	m.modals.search.flatten()
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

func TestSearch_SortByCountThenMTime(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	c := filepath.Join(dir, "c.md")
	for _, p := range []string{a, b, c} {
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// a and c tie on count; the tie-break is edit recency, so make a more
	// recently edited than c. b has the most matches, so it must lead.
	base := time.Now()
	if err := os.Chtimes(b, base.Add(-3*time.Hour), base.Add(-3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(c, base.Add(-2*time.Hour), base.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(a, base.Add(-1*time.Hour), base.Add(-1*time.Hour)); err != nil {
		t.Fatal(err)
	}

	// Input order deliberately not the wanted order, to prove the sort.
	files := []search.FileMatches{
		{Path: c, Count: 2, Lines: make([]search.Line, 2)},
		{Path: a, Count: 2, Lines: make([]search.Line, 2)},
		{Path: b, Count: 5, Lines: make([]search.Line, 5)},
	}
	got := sortSearchFiles(files)
	want := []string{b, a, c} // count desc (b), then recency tie-break (a before c)
	for i, p := range want {
		if got[i].Path != p {
			t.Errorf("sorted[%d].Path = %q, want %q (full order want %v)", i, got[i].Path, p, want)
		}
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
		files: []search.FileMatches{{Path: p, Count: 1, Lines: []search.Line{{Num: 2, Text: "foobar", At: 0, Len: 2}}}},
	})
	m = updated.(Model)
	if len(m.modals.search.files) != 1 {
		t.Fatalf("setup: want 1 file, got %d", len(m.modals.search.files))
	}
	// Now type more characters — results must clear immediately.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ol")})
	if got := len(m.modals.search.files); got != 0 {
		t.Errorf("after query change: files=%d want 0 (stale results not cleared)", got)
	}
}

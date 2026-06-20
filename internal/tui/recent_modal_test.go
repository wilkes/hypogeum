package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestRecentModalOpensOnR verifies the `r` key opens the recently-opened
// modal as a swappable modalKind.
func TestRecentModalOpensOnR(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")

	m := sized(t, dir, "")
	m = pressRune(t, m, 'r')

	if m.modals.kind != modalRecent {
		t.Fatalf("r should open recent modal: got kind %d want %d", m.modals.kind, modalRecent)
	}
}

// TestRecentModalListsVisitedInVisitOrder verifies the modal lists only
// visited files, most-recently-opened first, and excludes never-opened ones.
func TestRecentModalListsVisitedInVisitOrder(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	unvisited := filepath.Join(dir, "c.md")
	for _, p := range []string{a, b, unvisited} {
		writePickerFile(t, p, "# x")
	}

	m := sized(t, dir, "")
	// Open a then b → b is the most-recently-visited.
	m.openFile(a)
	m.openFile(b)

	m = pressRune(t, m, 'r')
	if got := len(m.recentList.items); got != 2 {
		t.Fatalf("recent items: got %d, want 2 (unvisited excluded)", got)
	}
	if m.recentList.items[0].Path != b {
		t.Errorf("first item %q, want %q (most recent visit)", m.recentList.items[0].Path, b)
	}
	if m.recentList.items[1].Path != a {
		t.Errorf("second item %q, want %q", m.recentList.items[1].Path, a)
	}
	for _, it := range m.recentList.items {
		if it.Path == unvisited {
			t.Errorf("unvisited file %q should not appear", unvisited)
		}
	}
}

// TestRecentModalCursorMoveDoesNotRerank locks in the caching invariant: the
// visit-ordered list is captured when the modal opens, so moving the cursor
// must not re-query the store. We record a fresh visit while the modal is open
// and confirm a cursor move doesn't fold it into the list (an old version
// re-ranked the whole vault on every keystroke).
func TestRecentModalCursorMoveDoesNotRerank(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	c := filepath.Join(dir, "c.md")
	for _, p := range []string{a, b, c} {
		writePickerFile(t, p, "# x")
	}

	m := sized(t, dir, "")
	m.openFile(a)
	m.openFile(b)

	m = pressRune(t, m, 'r')
	if got := len(m.recentList.items); got != 2 {
		t.Fatalf("recent items at open: got %d, want 2", got)
	}

	// Record a visit to c *after* the modal is open, then move the cursor.
	if err := m.recent.Record(c); err != nil {
		t.Fatalf("Record: %v", err)
	}
	m = pressRune(t, m, 'j')

	if got := len(m.recentList.items); got != 2 {
		t.Fatalf("after cursor move: got %d items, want 2 — cursor move re-ranked the store", got)
	}
	for _, it := range m.recentList.items {
		if it.Path == c {
			t.Errorf("file %q visited after open should not appear until reopen", c)
		}
	}
}

// TestRecentModalEmptyState verifies a vault with no visits shows an empty
// list (and renders without panicking).
func TestRecentModalEmptyState(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")

	m := sized(t, dir, "")
	// New auto-opens the first top-level file, which records a visit. Use a
	// fixture where nothing is auto-opened by clearing history is awkward;
	// instead assert the body renders the empty-state when items is empty.
	body := formatRecent(nil, dir, 40, 0)
	if body == "" {
		t.Fatal("formatRecent(nil) returned empty string, want placeholder")
	}
	// Open the modal too, just to exercise the open path without panic.
	m = pressRune(t, m, 'r')
	if m.modals.kind != modalRecent {
		t.Fatalf("modal kind: got %d want %d", m.modals.kind, modalRecent)
	}
}

// TestRecentModalEnterNavigates verifies Enter on a row closes the modal and
// navigates to the selected file (history-aware).
func TestRecentModalEnterNavigates(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	writePickerFile(t, a, "# A")
	writePickerFile(t, b, "# B")

	m := sized(t, dir, "")
	m.openFile(a)
	m.openFile(b)

	m = pressRune(t, m, 'r')
	// Top item is b (most recent). Enter should navigate to b.
	want := m.recentList.items[0].Path
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.modals.kind != modalNone {
		t.Errorf("Enter should close the modal, got kind %d", m.modals.kind)
	}
	if got := m.history.Current(); got != want {
		t.Errorf("history.Current after Enter: got %q want %q", got, want)
	}
}

// TestRecentModalCursorMoves verifies j/k move the cursor within the list.
func TestRecentModalCursorMoves(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	writePickerFile(t, a, "# A")
	writePickerFile(t, b, "# B")

	m := sized(t, dir, "")
	m.openFile(a)
	m.openFile(b)

	m = pressRune(t, m, 'r')
	if m.recentList.cursor != 0 {
		t.Fatalf("initial cursor: %d, want 0", m.recentList.cursor)
	}
	m = pressRune(t, m, 'j')
	if m.recentList.cursor != 1 {
		t.Errorf("after j: cursor=%d, want 1", m.recentList.cursor)
	}
	m = pressRune(t, m, 'k')
	if m.recentList.cursor != 0 {
		t.Errorf("after k: cursor=%d, want 0", m.recentList.cursor)
	}
}

// TestRecentModalEscCloses verifies Esc closes the modal without navigating.
func TestRecentModalEscCloses(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")

	m := sized(t, dir, "")
	before := m.history.Current()

	m = pressRune(t, m, 'r')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.modals.kind != modalNone {
		t.Errorf("Esc should close recent modal, got kind %d", m.modals.kind)
	}
	if m.history.Current() != before {
		t.Errorf("Esc should not navigate; was %q now %q", before, m.history.Current())
	}
}

// TestRecentModalSwapsWithBacklinks verifies the recent modal participates in
// the single-modal-swap invariant: opening backlinks while recent is open
// swaps to backlinks rather than stacking.
func TestRecentModalSwapsWithBacklinks(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")

	m := sized(t, dir, "")
	m = pressRune(t, m, 'r')
	if m.modals.kind != modalRecent {
		t.Fatalf("r should open recent modal, got %d", m.modals.kind)
	}
	m = pressRune(t, m, 'b')
	if m.modals.kind != modalBacklinks {
		t.Errorf("b while recent open should swap to backlinks, got %d", m.modals.kind)
	}
}

// TestRecentKeyTypedIntoPickerDoesNotOpenRecent is the regression guard for
// the picker-grabs-printable-keys gotcha: pressing `r` while the finder is
// open must type into the fuzzy filter, NOT open the recent modal.
func TestRecentKeyTypedIntoPickerDoesNotOpenRecent(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "report.md"), "# R")
	writePickerFile(t, filepath.Join(dir, "notes.md"), "# N")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.modals.kind != modalPicker {
		t.Fatalf("^p should open picker, got %d", m.modals.kind)
	}

	m = pressRune(t, m, 'r')

	// The picker must still be open (not swapped to the recent modal)...
	if m.modals.kind != modalPicker {
		t.Fatalf("typing r in the picker opened the recent modal (kind=%d); it must stay in the picker", m.modals.kind)
	}
	// ...and the `r` must have landed in the query.
	if got := m.modals.picker.input.Value(); got != "r" {
		t.Errorf("picker query after typing r: got %q, want %q", got, "r")
	}
}

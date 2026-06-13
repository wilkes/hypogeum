package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestCursorMoveAndRefresh_AdvancesAndCallsRefresh(t *testing.T) {
	cursor := 2
	calls := 0
	cursorMoveAndRefresh(&cursor, 5, +1, func() { calls++ })
	if cursor != 3 {
		t.Errorf("cursor: got %d want 3", cursor)
	}
	if calls != 1 {
		t.Errorf("refresh calls: got %d want 1", calls)
	}
}

func TestCursorMoveAndRefresh_ClampsAtZero(t *testing.T) {
	cursor := 0
	calls := 0
	cursorMoveAndRefresh(&cursor, 5, -1, func() { calls++ })
	if cursor != 0 {
		t.Errorf("cursor should not move below 0, got %d", cursor)
	}
	if calls != 0 {
		t.Errorf("refresh should not be called when cursor doesn't move; got %d calls", calls)
	}
}

func TestCursorMoveAndRefresh_ClampsAtMax(t *testing.T) {
	cursor := 4
	calls := 0
	cursorMoveAndRefresh(&cursor, 5, +1, func() { calls++ })
	if cursor != 4 {
		t.Errorf("cursor should clamp at max-1 (4), got %d", cursor)
	}
	if calls != 0 {
		t.Errorf("refresh should not be called at clamp; got %d calls", calls)
	}
}

func TestCursorMoveAndRefresh_SingleElement(t *testing.T) {
	cursor := 0
	calls := 0
	cursorMoveAndRefresh(&cursor, 1, +1, func() { calls++ })
	if cursor != 0 || calls != 0 {
		t.Errorf("single-element collection: cursor=%d calls=%d, want 0/0", cursor, calls)
	}
	cursorMoveAndRefresh(&cursor, 1, -1, func() { calls++ })
	if cursor != 0 || calls != 0 {
		t.Errorf("single-element collection: cursor=%d calls=%d, want 0/0", cursor, calls)
	}
}

func TestCursorMoveAndRefresh_EmptyCollection(t *testing.T) {
	cursor := 0
	calls := 0
	cursorMoveAndRefresh(&cursor, 0, +1, func() { calls++ })
	if cursor != 0 || calls != 0 {
		t.Errorf("empty collection: cursor=%d calls=%d, want 0/0", cursor, calls)
	}
	cursorMoveAndRefresh(&cursor, 0, -1, func() { calls++ })
	if cursor != 0 || calls != 0 {
		t.Errorf("empty collection: cursor=%d calls=%d, want 0/0", cursor, calls)
	}
}

func TestViewportClamp_CursorAlreadyVisible(t *testing.T) {
	vp := viewport.New(20, 10)
	vp.SetYOffset(0)
	viewportClamp(&vp, 5, 1)
	if vp.YOffset != 0 {
		t.Errorf("visible cursor should not scroll; YOffset=%d want 0", vp.YOffset)
	}
}

func TestViewportClamp_CursorAboveViewport(t *testing.T) {
	vp := viewport.New(20, 10)
	tall := ""
	for i := 0; i < 100; i++ {
		tall += "x\n"
	}
	vp.SetContent(tall)
	vp.SetYOffset(20)
	viewportClamp(&vp, 5, 1)
	if vp.YOffset != 5 {
		t.Errorf("cursor above viewport should scroll up; YOffset=%d want 5", vp.YOffset)
	}
}

func TestViewportClamp_CursorBelowViewport(t *testing.T) {
	vp := viewport.New(20, 10)
	// Build content tall enough so SetYOffset can land where we want.
	tall := ""
	for i := 0; i < 100; i++ {
		tall += "x\n"
	}
	vp.SetContent(tall)
	vp.SetYOffset(0)
	// Cursor at row 30 with rowsPerEntry=1 must scroll so target=30 is visible.
	viewportClamp(&vp, 30, 1)
	// New YOffset should make 30 visible: YOffset = 30 - height + 1 = 30 - 10 + 1 = 21.
	if vp.YOffset != 21 {
		t.Errorf("cursor below viewport should scroll down; YOffset=%d want 21", vp.YOffset)
	}
}

func TestViewportClamp_RowsPerEntryGreaterThanOne(t *testing.T) {
	vp := viewport.New(20, 10)
	tall := ""
	for i := 0; i < 100; i++ {
		tall += "x\n"
	}
	vp.SetContent(tall)
	vp.SetYOffset(0)
	// Two rows per entry: cursor=4 → target row 8, both rows of the
	// entry (rows 8 and 9) just fit. YOffset stays 0.
	viewportClamp(&vp, 4, 2)
	if vp.YOffset != 0 {
		t.Errorf("rowsPerEntry=2 cursor=4 fits; YOffset=%d want 0", vp.YOffset)
	}
	// Cursor=5 → target row 10, falls past the bottom of the visible
	// window (rows 0..9). YOffset must scroll so the entry's bottom row
	// (11) sits inside the window: YOffset = 10 - 10 + 2 = 2.
	viewportClamp(&vp, 5, 2)
	if vp.YOffset != 2 {
		t.Errorf("rowsPerEntry=2 cursor=5 scrolls; YOffset=%d want 2", vp.YOffset)
	}
}

func TestOpenFileRecordsVisit(t *testing.T) {
	isolatedHome(t)
	dir := t.TempDir()
	// Create an initial file so New won't fail on an empty tree.
	initFile := filepath.Join(dir, "init.md")
	if err := os.WriteFile(initFile, []byte("# Init"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(dir, initFile)
	if err != nil {
		t.Fatal(err)
	}

	// Create a new file that wasn't opened by New.
	p := filepath.Join(dir, "n.md")
	if err := os.WriteFile(p, []byte("# N"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Sanity: Recent rank before openFile shouldn't include p as visited.
	pre := m.recent.Rank([]string{p})
	if len(pre) != 1 {
		t.Fatalf("pre Rank: got %d, want 1", len(pre))
	}
	if !pre[0].Visit.IsZero() {
		t.Errorf("pre Rank: visit should be zero, got %v", pre[0].Visit)
	}

	m.openFile(p)

	post := m.recent.Rank([]string{p})
	if len(post) != 1 {
		t.Fatalf("post Rank: got %d, want 1", len(post))
	}
	if post[0].Visit.IsZero() {
		t.Error("post Rank: visit should be non-zero after openFile")
	}
}

// TestSearch_EndToEnd opens the search modal via ^s, types a query that
// matches a single file, simulates the debounce tick + scan result, presses
// Enter, and verifies the destination renders scrolled to the matched line.
// This is the wire-it-all-up sanity check — fine-grained behavior covered
// in search_test.go.
func TestSearch_EndToEnd(t *testing.T) {
	isolatedHome(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "target.md")
	var sb strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&sb, "line %d\n\n", i)
	}
	sb.WriteString("the magic phrase appears here\n")
	for i := 1; i <= 10; i++ {
		sb.WriteString("trailing line\n")
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

	// / opens the modal
	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	mm := updated.(Model)
	if mm.modals.kind != modalSearch {
		t.Fatalf("modal not opened, kind = %v", mm.modals.kind)
	}

	// Type "magic"
	for _, r := range "magic" {
		updated, _ = mm.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		mm = updated.(Model)
	}

	// Simulate the debounce tick by feeding the message directly. In a real
	// tea.Program, tea.Tick would deliver this; here we synthesize it so the
	// test doesn't depend on wall-clock timing.
	updated, cmd := mm.Update(searchTickMsg{query: "magic"})
	mm = updated.(Model)
	if cmd != nil {
		// Run the cmd to get the searchResultsMsg.
		msg := cmd()
		updated, _ = mm.Update(msg)
		mm = updated.(Model)
	}

	if len(mm.modals.search.hits) == 0 {
		t.Fatalf("expected hits for 'magic', got 0")
	}

	// Enter on the hit
	updated, _ = mm.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	mm = updated.(Model)

	if mm.modals.kind != modalNone {
		t.Errorf("Enter should close modal, kind = %v", mm.modals.kind)
	}
	if mm.history.Current() != p {
		t.Errorf("Current = %q, want %q", mm.history.Current(), p)
	}
	if mm.content.viewport.YOffset == 0 {
		t.Errorf("expected viewport scrolled after Enter, YOffset = 0")
	}
}

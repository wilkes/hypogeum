package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
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

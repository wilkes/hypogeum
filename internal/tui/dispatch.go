package tui

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// cursorMoveAndRefresh updates *cursor by delta, clamped to [0, max-1],
// and only invokes refresh if the cursor actually moved. max is the
// length of whatever collection the cursor indexes into.
func cursorMoveAndRefresh(cursor *int, max, delta int, refresh func()) {
	next := *cursor + delta
	if next < 0 || next >= max || next == *cursor {
		return
	}
	*cursor = next
	refresh()
}

// viewportClamp scrolls vp so the row at cursor*rowsPerEntry is visible.
// rowsPerEntry is 1 for line-based viewports (tree, picker), and the
// number of rows each backlinks entry occupies in the backlinks modal.
func viewportClamp(vp *viewport.Model, cursor, rowsPerEntry int) {
	target := cursor * rowsPerEntry
	if target < vp.YOffset {
		vp.SetYOffset(target)
		return
	}
	visibleEnd := vp.YOffset + vp.Height - rowsPerEntry
	if target > visibleEnd {
		vp.SetYOffset(target - vp.Height + rowsPerEntry)
	}
}

// openModalWith is a thin wrapper around toggleModal that runs prepare()
// before swapping the modal. Used by the four modal-toggle key handlers.
func (m *Model) openModalWith(kind modalKind, prepare func()) tea.Cmd {
	return m.toggleModal(kind, func() tea.Cmd {
		prepare()
		return nil
	})
}

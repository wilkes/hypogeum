package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// modalKind enumerates which modal (if any) is currently visible.
// The single-modal invariant means at most one is open at a time:
// pressing the toggle key for one while another is open swaps content.
type modalKind int

const (
	modalNone modalKind = iota
	modalBacklinks
	modalLogs
)

// modalGeometry returns the (x, y, w, h) of the modal frame given the
// current terminal dimensions. The modal is 60% × 60% clamped to a
// minimum of 40×12 and a maximum of 120×40.
func modalGeometry(termW, termH int) (x, y, w, h int) {
	w = termW * 60 / 100
	h = termH * 60 / 100
	if w < 40 {
		w = 40
	}
	if h < 12 {
		h = 12
	}
	if w > 120 {
		w = 120
	}
	if h > 40 {
		h = 40
	}
	if w > termW {
		w = termW
	}
	if h > termH {
		h = termH
	}
	x = (termW - w) / 2
	y = (termH - h) / 2
	return
}

// renderModal styles `body` with a border at the modal geometry.
// The caller is responsible for clipping `body` to modal interior size.
func (m Model) renderModal(body string) string {
	_, _, w, h := modalGeometry(m.width, m.height)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Width(w - 2).
		Height(h - 2).
		Render(body)
}

// resizeModalVP resizes the shared modal viewport to fit the modal interior.
func (m *Model) resizeModalVP() {
	_, _, w, h := modalGeometry(m.width, m.height)
	m.modalVP.Width = w - 2
	m.modalVP.Height = h - 2
}

// newModalViewport returns an empty viewport sized 0,0 — resized later.
func newModalViewport() viewport.Model {
	return viewport.New(0, 0)
}

// overlayModal places `modal` in the center of `base`. Both are full
// width/height strings; this implementation renders `modal` over the
// corresponding rows of `base`.
//
// spliceLine is naive about ANSI escapes inside `base` — Phase 1 modal
// is opaque enough that this works in practice.
func overlayModal(base, modal string, termW, termH int) string {
	x, y, _, _ := modalGeometry(termW, termH)

	baseLines := strings.Split(base, "\n")
	modalLines := strings.Split(modal, "\n")

	for i, ml := range modalLines {
		row := y + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		baseLines[row] = spliceLine(baseLines[row], ml, x)
	}
	return strings.Join(baseLines, "\n")
}

// spliceLine overlays `over` onto `base` starting at column x.
// Naive ASCII-aware version (does not handle ANSI escapes inside
// `base` — modal is opaque so this is acceptable for Phase 1).
func spliceLine(base, over string, x int) string {
	if x >= len(base) {
		return base + strings.Repeat(" ", x-len(base)) + over
	}
	end := x + len(over)
	if end > len(base) {
		return base[:x] + over
	}
	return base[:x] + over + base[end:]
}

package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// modalKind enumerates which modal (if any) is currently visible.
// The single-modal invariant means at most one is open at a time:
// pressing the toggle key for one while another is open swaps content.
type modalKind int

const (
	modalNone modalKind = iota
	modalBacklinks
	modalLogs
	modalPicker
)

// toggleModal closes the modal of `kind` if it's currently open;
// otherwise it saves the current focus (unless the persistent backlinks
// pane has it — that pane stays sticky across modal opens), sets the
// modal open, and runs onOpen for per-modal init. The returned Cmd is
// whatever onOpen produced, threaded back to Bubble Tea.
func (m *Model) toggleModal(kind modalKind, onOpen func() tea.Cmd) tea.Cmd {
	if m.modalOpen == kind {
		m.modalOpen = modalNone
		m.focus = m.prevFocus
		return nil
	}
	if m.modalOpen == modalNone && m.focus != focusBacklinks {
		m.prevFocus = m.focus
	}
	m.modalOpen = kind
	return onOpen()
}

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

// ansiReset clears any SGR state leaking from the base line so the modal
// segment renders with its own styling, and prevents the trailing tail of
// base from inheriting the modal's styling.
const ansiReset = "\x1b[0m"

// spliceLine overlays `over` onto `base` starting at visible column x.
// ANSI-aware: uses cell widths (not byte offsets) so SGR escapes in `base`
// aren't sliced mid-sequence, which would otherwise leak control bytes
// into the rendered output and corrupt subsequent styling.
func spliceLine(base, over string, x int) string {
	overWidth := ansi.StringWidth(over)
	baseWidth := ansi.StringWidth(base)

	if x >= baseWidth {
		// Modal starts past the end of this base line; pad with spaces.
		return base + strings.Repeat(" ", x-baseWidth) + ansiReset + over + ansiReset
	}

	left := ansi.Truncate(base, x, "")
	end := x + overWidth
	if end >= baseWidth {
		return left + ansiReset + over + ansiReset
	}
	right := ansi.TruncateLeft(base, end, "")
	return left + ansiReset + over + ansiReset + right
}

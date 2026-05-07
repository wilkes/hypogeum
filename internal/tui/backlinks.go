package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/wilkes/hypogeum/internal/vault"
)

// backlinksHeight is the row count of the persistent bottom-split pane,
// including its border. Two visible rows per backlink × ~3 backlinks
// visible at a time + border (2) = 8.
const backlinksHeight = 8

// backlinksMinTotalHeight is the minimum terminal height at which the
// persistent backlinks pane is shown. Below this, the pane state is
// preserved but the pane is suppressed in View().
const backlinksMinTotalHeight = 20

// snippetHighlightOpenChar / CloseChar mirror the markers vault embeds
// in Backlink.Snippet. Defined here so the TUI doesn't import vault's
// internal constants — the markers are part of the Snippet string's
// data contract.
const (
	snippetHighlightOpenChar  = "\x11"
	snippetHighlightCloseChar = "\x12"
)

// backlinksSurface identifies which backlinks UI surface (persistent
// pane vs modal) the user was navigating when they followed a backlink.
// Used by returnCursor so Back can restore them to the same surface.
type backlinksSurface int

const (
	surfacePane backlinksSurface = iota
	surfaceModal
)

// returnCursor remembers where the user was in the backlinks list
// before following a backlink. Single-slot: we only restore on the
// next Back navigation, and only if it lands on the file we recorded.
type returnCursor struct {
	sourceFile string
	cursor     int
	surface    backlinksSurface
}

// clamp returns v constrained to [lo, hi]. If hi < lo (e.g. when the
// list is empty so hi = -1), returns lo. Used on cursor restoration
// to defend against the list shrinking between follow and return.
func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// shouldShowBacklinks returns true when the persistent pane is open
// AND the terminal is tall enough for it.
func (m Model) shouldShowBacklinks() bool {
	return m.backlinksOpen && m.height >= backlinksMinTotalHeight
}

// refreshBacklinks repopulates m.backlinksVP from the vault for the
// currently-open file. Called on file change and on toggle.
func (m *Model) refreshBacklinks(currentPath string) {
	if m.vault == nil || currentPath == "" {
		m.backlinks = nil
		m.backlinksVP.SetContent("")
		return
	}
	links := m.vault.Backlinks(currentPath)
	m.backlinks = links
	m.backlinksVP.SetContent(formatBacklinks(links, m.root, m.viewport.Width, m.backlinkCursor))
}

// renderBacklinks returns the rendered string of the persistent pane,
// styled to match the rest of the UI. Empty string when the pane is
// suppressed.
func (m Model) renderBacklinks() string {
	if !m.shouldShowBacklinks() {
		return ""
	}
	return paneStyle(false).
		Width(m.viewport.Width).
		Height(backlinksHeight - 2). // -2 for top/bottom border
		Render(m.backlinksVP.View())
}

// cursorMarkerStyle is the left-edge highlight for the selected entry.
// Distinct from the snippet's yellow highlight (which marks the matched
// display text) — this one signals structural position in the list.
var cursorMarkerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)

// formatBacklinks renders a slice of vault.Backlink as the two-row-per-
// entry text used in both the persistent pane and the modal. If
// cursor is in [0, len(links)), the row at that index gets a left-edge
// marker; pass -1 for no selection.
func formatBacklinks(links []vault.Backlink, root string, width, cursor int) string {
	if len(links) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("(no backlinks)")
	}
	var b strings.Builder
	for i, l := range links {
		rel, err := filepath.Rel(root, l.SourceFile)
		if err != nil {
			rel = l.SourceFile
		}
		marker := "  "
		if i == cursor {
			marker = cursorMarkerStyle.Render("▌") + " "
		}
		fmt.Fprintf(&b, "%s%s:%d\n", marker, rel, l.Line)
		fmt.Fprintf(&b, "%s  %s\n", marker, truncateOneLine(applyHighlight(l.Snippet), width-4))
	}
	return b.String()
}

// applyHighlight replaces snippetHighlightOpenChar/CloseChar markers
// with SGR codes for visible bold/yellow display.
func applyHighlight(s string) string {
	hi := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	out := s
	for {
		i := strings.Index(out, snippetHighlightOpenChar)
		j := strings.Index(out, snippetHighlightCloseChar)
		if i < 0 || j < 0 || j < i {
			break
		}
		out = out[:i] + hi.Render(out[i+1:j]) + out[j+1:]
	}
	return out
}

// refreshBacklinksModal repopulates m.modalVP from the vault for the
// currently-open file. Called when opening the backlinks modal.
func (m *Model) refreshBacklinksModal(currentPath string) {
	if m.vault == nil || currentPath == "" {
		m.backlinks = nil
		m.modalVP.SetContent("")
		return
	}
	m.resizeModalVP()
	links := m.vault.Backlinks(currentPath)
	m.backlinks = links
	m.modalVP.SetContent(formatBacklinks(links, m.root, m.modalVP.Width, m.backlinkCursor))
}

// ensureCursorVisible adjusts vp's YOffset so the two-row entry at
// m.backlinkCursor is fully on-screen. Called after every cursor
// mutation. Each backlink takes 2 visible rows.
func (m *Model) ensureCursorVisible(vp *viewport.Model) {
	const rowsPerEntry = 2
	cursorTop := m.backlinkCursor * rowsPerEntry
	cursorBottom := cursorTop + rowsPerEntry - 1

	if cursorTop < vp.YOffset {
		vp.SetYOffset(cursorTop)
		return
	}
	if cursorBottom >= vp.YOffset+vp.Height {
		vp.SetYOffset(cursorBottom - vp.Height + 1)
	}
}

// truncateOneLine collapses internal newlines into spaces and clips
// to width with an ellipsis if needed.
func truncateOneLine(s string, width int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if width <= 0 || len(s) <= width {
		return s
	}
	if width <= 1 {
		return s[:width]
	}
	return s[:width-1] + "…"
}

// activeBacklinksSurface reports which backlinks surface is currently
// receiving the user's navigation input. Used at follow time so the
// returnCursor records where to restore on Back.
//
// Modal takes precedence: if both are open and we're following from
// the modal, we want to come back to the modal.
func (m Model) activeBacklinksSurface() backlinksSurface {
	if m.modalOpen == modalBacklinks {
		return surfaceModal
	}
	return surfacePane
}

// followBacklink navigates to the SourceFile of the currently selected
// backlink, recording return state for a subsequent h (Back) restore.
// No-op if no backlink is selected (e.g. empty list).
func (m *Model) followBacklink() {
	if m.backlinkCursor < 0 || m.backlinkCursor >= len(m.backlinks) {
		return
	}
	bl := m.backlinks[m.backlinkCursor]

	// Save return state BEFORE openFile mutates history.
	m.returnCursor = &returnCursor{
		sourceFile: m.history.Current(),
		cursor:     m.backlinkCursor,
		surface:    m.activeBacklinksSurface(),
	}

	// Close modal if active; persistent pane stays open and
	// re-populates for the new file's own backlinks.
	if m.modalOpen == modalBacklinks {
		m.modalOpen = modalNone
	}
	m.focus = focusContent

	m.openFile(bl.SourceFile)
	m.scrollToLine(bl.Line)
}

// maybeRestoreReturnCursor checks if a returnCursor was set and the
// path we just navigated to matches it. If so, restores the cursor
// position and the surface (focus on pane, or reopen modal). Consumes
// the slot regardless of the surface restore actually being possible
// (e.g. the user closed the pane while away).
func (m *Model) maybeRestoreReturnCursor(path string) {
	if m.returnCursor == nil || path != m.returnCursor.sourceFile {
		return
	}
	rc := m.returnCursor
	m.returnCursor = nil

	m.refreshBacklinks(path)
	m.backlinkCursor = clamp(rc.cursor, 0, len(m.backlinks)-1)

	switch rc.surface {
	case surfacePane:
		if m.shouldShowBacklinks() {
			m.focus = focusBacklinks
		}
		m.refreshBacklinks(path) // re-render with cursor highlighted
	case surfaceModal:
		m.modalOpen = modalBacklinks
		m.refreshBacklinksModal(path)
	}
}

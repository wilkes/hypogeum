package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/wilkes/hypogeum/internal/highlight"
	"github.com/wilkes/hypogeum/internal/vault"
)

// backlinksUIState bundles the backlinks modal's render state. cursor
// indexes into items; items is cached so cursor moves don't re-query the
// vault; returnCursor is set when following a backlink and consumed on
// the next matching Back navigation so the modal reopens at the saved
// cursor position.
type backlinksUIState struct {
	cursor       int
	items        []vault.Backlink
	returnCursor *returnCursor
}

// returnCursor remembers where the user was in the backlinks list
// before following a backlink. Single-slot: we only restore on the
// next Back navigation, and only if it lands on the file we recorded.
type returnCursor struct {
	sourceFile string
	cursor     int
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

// cursorMarkerStyle is the left-edge highlight for the selected entry.
// Distinct from the snippet's yellow highlight (which marks the matched
// display text) — this one signals structural position in the list.
var cursorMarkerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)

// formatBacklinks renders a slice of vault.Backlink as the two-row-per-
// entry text used by the backlinks modal. If cursor is in [0, len(links)),
// the row at that index gets a left-edge marker; pass -1 for no selection.
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

// applyHighlight replaces the highlight.Open/Close markers (\x11/\x12,
// defined in internal/highlight) embedded in a snippet with SGR codes
// for visible bold/yellow display.
func applyHighlight(s string) string {
	hi := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	out := s
	for {
		i := strings.Index(out, highlight.Open)
		j := strings.Index(out, highlight.Close)
		if i < 0 || j < 0 || j < i {
			break
		}
		out = out[:i] + hi.Render(out[i+1:j]) + out[j+1:]
	}
	return out
}

// refreshBacklinksModal repopulates m.modals.vp from the vault for the
// currently-open file. Called when opening the backlinks modal.
func (m *Model) refreshBacklinksModal(currentPath string) {
	if m.vault == nil || currentPath == "" {
		m.backlinks.items = nil
		m.modals.vp.SetContent("")
		return
	}
	m.resizeModalVP()
	links := m.vault.Backlinks(currentPath)
	m.backlinks.items = links
	m.modals.vp.SetContent(formatBacklinks(links, m.root, m.modals.vp.Width, m.backlinks.cursor))
}

// ensureCursorVisible adjusts vp's YOffset so the two-row entry at
// m.backlinks.cursor is fully on-screen. Called after every cursor
// mutation. Each backlink takes 2 visible rows.
func (m *Model) ensureCursorVisible(vp *viewport.Model) {
	const rowsPerEntry = 2
	viewportClamp(vp, m.backlinks.cursor, rowsPerEntry)
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

// followBacklink navigates to the SourceFile of the currently selected
// backlink, recording return state for a subsequent h (Back) restore.
// No-op if no backlink is selected (e.g. empty list). Closes the modal
// and re-derives tree expansion for the new file.
func (m *Model) followBacklink() {
	if m.backlinks.cursor < 0 || m.backlinks.cursor >= len(m.backlinks.items) {
		return
	}
	bl := m.backlinks.items[m.backlinks.cursor]

	// Save return state BEFORE openFile mutates history.
	m.backlinks.returnCursor = &returnCursor{
		sourceFile: m.history.Current(),
		cursor:     m.backlinks.cursor,
	}

	_ = m.closeModal()
	m.focus = focusContent

	// Pre-select the inline link in the source file that points back to
	// the file we're leaving, plus any active range highlight, so the
	// destination's refreshContent can reapply it. Mirrors the capture
	// in Back/Forward (input.go) — every navigation-out path must
	// capture both fields so the invariant holds uniformly.
	m.pending.preselectTarget = m.history.Current()
	m.pending.preselectRange = m.content.rangeHighlight

	m.openFile(bl.SourceFile)
	// Re-derive tree expansion from the new file's ancestor chain so the
	// tree modal opens on a focused view next time. selectInTree also
	// re-flattens and moves the cursor.
	m.selectInTree(bl.SourceFile)

	// If the pre-select succeeded, scrollToLink already placed the link
	// in view; skip the source-line scroll which would scroll away.
	if m.content.linkCursor < 0 {
		m.scrollToLine(bl.Line)
	}
}

// maybeRestoreReturnCursor checks if a returnCursor was set and the
// path we just navigated to matches it. If so, reopens the backlinks
// modal at the saved cursor position. Consumes the slot regardless.
func (m *Model) maybeRestoreReturnCursor(path string) {
	if m.backlinks.returnCursor == nil || path != m.backlinks.returnCursor.sourceFile {
		return
	}
	rc := m.backlinks.returnCursor
	m.backlinks.returnCursor = nil

	if m.modals.kind == modalNone {
		m.modals.prevFocus = m.focus
	}
	m.modals.kind = modalBacklinks
	// Refresh first to populate items; clamp the saved cursor to the
	// (possibly shrunk) list; refresh again so the cursor highlight
	// appears on the correct row.
	m.refreshBacklinksModal(path)
	m.backlinks.cursor = clamp(rc.cursor, 0, len(m.backlinks.items)-1)
	m.refreshBacklinksModal(path)
}

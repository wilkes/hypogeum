package tui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wilkes/hypogeum/internal/recent"
)

// recentUIState bundles the "recently opened" modal's render state.
// cursor indexes into items; items is the visit-ordered list captured when
// the modal opens so cursor moves don't re-query the store. Unlike the file
// finder (which ranks by edit-recency / mtime), this modal lists only files
// the user has actually opened, most-recently-visited first.
type recentUIState struct {
	cursor int
	items  []recent.Ranked
}

// refreshRecentModal ranks the visit history — scoped to the current vault's
// markdown files, ordered most-recently-opened first, visited-only — into
// m.recentList.items and renders it. This re-queries the store, so it runs
// only when the modal opens; cursor moves use renderRecentModal (render-only)
// to repaint the cached list without re-walking the vault on every keystroke.
func (m *Model) refreshRecentModal() {
	var items []recent.Ranked
	if m.recent != nil {
		items = m.recent.RankByVisit(m.allVaultMarkdownPaths())
	}
	m.recentList.items = items
	m.recentList.cursor = clamp(m.recentList.cursor, 0, len(items)-1)
	m.renderRecentModal()
}

// renderRecentModal repaints the cached visit-ordered list into the modal
// viewport (the cursor marker may have moved) without re-querying the store.
// Cursor moves call this; only opening the modal calls refreshRecentModal.
func (m *Model) renderRecentModal() {
	m.resizeModalVP()
	m.modals.vp.SetContent(formatRecent(m.recentList.items, m.root, m.modals.vp.Width, m.recentList.cursor))
	m.ensureRecentCursorVisible()
}

// ensureRecentCursorVisible scrolls the shared modal viewport so the
// one-row-per-entry cursor stays on screen.
func (m *Model) ensureRecentCursorVisible() {
	viewportClamp(&m.modals.vp, m.recentList.cursor, 1)
}

// formatRecent renders the visit-ordered list as one row per file, each
// "<relative-path>" with a left-edge marker on the selected row. Empty list
// shows a faint placeholder, mirroring the backlinks modal's empty state.
func formatRecent(items []recent.Ranked, root string, width, cursor int) string {
	if len(items) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("(no recently opened files)")
	}
	var b strings.Builder
	for i, it := range items {
		rel, err := filepath.Rel(root, it.Path)
		if err != nil {
			rel = it.Path
		}
		marker := "  "
		if i == cursor {
			marker = cursorMarkerStyle.Render("▌") + " "
		}
		b.WriteString(marker + truncateOneLine(rel, width-2))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// followRecent navigates to the file under the recent-modal cursor, closing
// the modal first (history-aware navigateTo). No-op when the list is empty
// or the cursor is out of range.
func (m *Model) followRecent() tea.Cmd {
	if m.recentList.cursor < 0 || m.recentList.cursor >= len(m.recentList.items) {
		return nil
	}
	path := m.recentList.items[m.recentList.cursor].Path
	cmd := m.closeModal()
	m.focus = focusContent
	m.navigateTo(path)
	return cmd
}

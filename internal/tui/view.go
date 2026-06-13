package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// treeUIState bundles the tree's render state. flat is the pre-flattened
// row list; cursor indexes into it; vp scrolls the tree modal when flat
// exceeds modal height; expanded records which directories are currently
// open (path → true means open; missing means closed). The tree renders
// only inside the modal opened with `^b`.
type treeUIState struct {
	flat     []treeRow
	cursor   int
	vp       viewport.Model
	expanded map[string]bool
}

func (m Model) View() string {
	if m.width == 0 {
		return "" // wait for first WindowSizeMsg
	}

	content := m.content.viewport.View()

	contentHeight := m.height - 4
	body := zone.Mark(zoneContentPane, paneStyle(true).
		Width(m.content.viewport.Width).
		Height(contentHeight).
		Render(content))

	footer := m.renderFooter()
	composed := lipgloss.JoinVertical(lipgloss.Left, body, footer)
	if m.modals.kind != modalNone {
		var modalBody string
		switch m.modals.kind {
		case modalPicker:
			modalBody = m.modals.picker.View()
		case modalTree:
			modalBody = m.tree.vp.View()
		case modalSearch:
			modalBody = m.searchView()
		default:
			modalBody = m.modals.vp.View()
		}
		// Splice modal first, scan last — BubbleZone must see the final
		// screen coordinates so tree-row zones rendered inside the modal
		// resolve correctly for mouse hit-testing.
		return zone.Scan(overlayModal(composed, m.renderModal(modalBody), m.width, m.height))
	}
	return zone.Scan(composed)
}

// refreshTreeVP populates the tree viewport with the current rendered
// rows and scrolls so that m.tree.cursor is within the visible window.
// Call after every write to m.tree.flat or m.tree.cursor.
func (m *Model) refreshTreeVP() {
	m.tree.vp.SetContent(m.renderTree())
	viewportClamp(&m.tree.vp, m.tree.cursor, 1)
}

func (m Model) renderTree() string {
	var b strings.Builder
	for i, row := range m.tree.flat {
		indent := strings.Repeat("  ", row.depth)
		marker := " "
		if i == m.tree.cursor {
			marker = ">"
		}
		name := row.node.Name
		if row.node.IsDir {
			chevron := "▸ "
			if m.isExpanded(row.node) {
				chevron = "▾ "
			}
			name = chevron + name + "/"
		} else {
			name = "  " + name
		}
		// Wrap the whole row (minus its trailing newline) in a per-row
		// zone so a click anywhere on the row routes to that index.
		line := fmt.Sprintf("%s%s %s", marker, indent, name)
		b.WriteString(zone.Mark(treeRowZoneID(i), line))
		b.WriteByte('\n')
	}
	return b.String()
}

func (m Model) renderFooter() string {
	help := "?: help  q: quit"

	// A transient footer message (error or prompt) takes the location
	// slot in place of the current path; otherwise the current file path
	// shows. This mirrors the single overloaded field this replaced.
	loc := m.currentPath
	if m.footerMessage != "" {
		loc = m.footerMessage
	}
	if loc != "" {
		// Show path relative to root for brevity.
		if rel, err := filepath.Rel(m.root, loc); err == nil {
			loc = rel
		}
	}

	transientActive := false
	transientStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	if m.diag != nil {
		if e, ok := m.diag.transientStatus(); ok {
			loc = transientStyle.Render(e.Severity.String() + ": " + e.Message)
			transientActive = true
		}
	}

	hasLink := false
	if sel := m.selectedLink(); sel != nil {
		loc = fmt.Sprintf("%s%s [%d/%d] %s", linkFooterMarker, loc, m.content.linkCursor+1, len(m.content.links), linkLabel(*sel, m.root))
		hasLink = true
	}

	if !transientActive && m.content.brokenCount > 0 {
		brokenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Faint(true)
		loc += brokenStyle.Render(fmt.Sprintf(" ⚠ %d broken", m.content.brokenCount))
	}
	// The help row is always faint. The location row is faint by default
	// but gets full brightness when a link is selected, since it's the
	// only signal that link-cycling is active.
	helpStyle := lipgloss.NewStyle().Faint(true)
	locStyle := helpStyle
	if hasLink {
		locStyle = lipgloss.NewStyle()
	}
	return fmt.Sprintf("%s\n%s", locStyle.Render(loc), helpStyle.Render(help))
}

// resizeTreeModalVP sizes the tree viewport to the modal interior.
// Called on each WindowSizeMsg so the tree adapts to terminal changes
// even when the modal isn't currently open.
func (m *Model) resizeTreeModalVP() {
	_, _, w, h := modalGeometry(m.width, m.height)
	m.tree.vp.Width = w - 2
	if m.tree.vp.Width < 1 {
		m.tree.vp.Width = 1
	}
	m.tree.vp.Height = h - 2
	if m.tree.vp.Height < 1 {
		m.tree.vp.Height = 1
	}
}

func paneStyle(focused bool) lipgloss.Style {
	border := lipgloss.RoundedBorder()
	color := lipgloss.Color("240")
	if focused {
		color = lipgloss.Color("62")
	}
	return lipgloss.NewStyle().Border(border).BorderForeground(color)
}

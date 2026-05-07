package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// twoPaneMinWidth is the minimum terminal width at which the tree
// pane is shown alongside content. Mirrors backlinksMinTotalHeight on
// the height axis.
const twoPaneMinWidth = 80

// shouldShowTree gates m.treeVisible (user intent) on terminal width.
// Below twoPaneMinWidth the tree is force-hidden so content gets the
// full window. Parallels shouldShowBacklinks on the height axis.
func (m Model) shouldShowTree() bool {
	return m.treeVisible && m.width >= twoPaneMinWidth
}

func (m Model) View() string {
	if m.width == 0 {
		return "" // wait for first WindowSizeMsg
	}

	content := m.viewport.View()

	contentHeight := m.height - 4
	if m.shouldShowBacklinks() {
		contentHeight -= backlinksHeight
	}
	contentStyled := zone.Mark(zoneContentPane, paneStyle(m.focus == focusContent).
		Width(m.viewport.Width).
		Height(contentHeight).
		Render(content))

	contentColumn := contentStyled
	if bl := m.renderBacklinks(); bl != "" {
		contentColumn = lipgloss.JoinVertical(lipgloss.Left, contentStyled, bl)
	}

	var body string
	if m.shouldShowTree() {
		treeStyled := zone.Mark(zoneTreePane, paneStyle(m.focus == focusTree).
			Width(m.treeWidth()).
			Height(m.height-4).
			Render(m.renderTree()))
		body = lipgloss.JoinHorizontal(lipgloss.Top, treeStyled, contentColumn)
	} else {
		body = contentColumn
	}
	footer := m.renderFooter()
	// Scan must run on the final composed output so BubbleZone records
	// each zone's absolute screen position.
	base := zone.Scan(lipgloss.JoinVertical(lipgloss.Left, body, footer))
	if m.modalOpen != modalNone {
		body := m.modalVP.View()
		if m.modalOpen == modalPicker {
			body = m.picker.View()
		}
		return overlayModal(base, m.renderModal(body), m.width, m.height)
	}
	return base
}

func (m Model) renderTree() string {
	var b strings.Builder
	for i, row := range m.flatTree {
		indent := strings.Repeat("  ", row.depth)
		marker := " "
		if i == m.treeCursor {
			marker = ">"
		}
		name := row.node.Name
		if row.node.IsDir {
			chevron := "▾ "
			if m.isCollapsed(row.node.Path) {
				chevron = "▸ "
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
	keys := []string{
		"tab: switch", "↑↓/jk: move", "enter: open",
		"n/p: link", "esc: clear",
		"b: backlinks", "B: modal", "?: logs",
		"^p: open", "h/←: back", "l/→: forward", "q: quit",
	}
	help := strings.Join(keys, "  ")

	loc := m.status
	if loc != "" {
		// Show path relative to root for brevity.
		if rel, err := filepath.Rel(m.root, loc); err == nil {
			loc = rel
		}
	}

	transientStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	if m.diag != nil {
		if e, ok := m.diag.transientStatus(); ok {
			loc = transientStyle.Render(e.Severity.String() + ": " + e.Message)
		}
	}

	hasLink := false
	if sel := m.selectedLink(); sel != nil {
		loc = fmt.Sprintf("%s%s [%d/%d] %s", linkFooterMarker, loc, m.linkCursor+1, len(m.links), linkLabel(*sel, m.root))
		hasLink = true
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

func (m Model) treeWidth() int {
	if !m.shouldShowTree() {
		return 0
	}
	return clamp(m.width/4, 16, 40)
}

func paneStyle(focused bool) lipgloss.Style {
	border := lipgloss.RoundedBorder()
	color := lipgloss.Color("240")
	if focused {
		color = lipgloss.Color("62")
	}
	return lipgloss.NewStyle().Border(border).BorderForeground(color)
}

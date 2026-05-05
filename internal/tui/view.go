package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

func (m Model) View() string {
	if m.width == 0 {
		return "" // wait for first WindowSizeMsg
	}

	tree := m.renderTree()
	content := m.viewport.View()

	treeStyled := zone.Mark(zoneTreePane, paneStyle(m.focus == focusTree).
		Width(m.treeWidth()).
		Height(m.height-2).
		Render(tree))
	contentStyled := zone.Mark(zoneContentPane, paneStyle(m.focus == focusContent).
		Width(m.viewport.Width).
		Height(m.height-2).
		Render(content))

	body := lipgloss.JoinHorizontal(lipgloss.Top, treeStyled, contentStyled)
	footer := m.renderFooter()
	// Scan must run on the final composed output so BubbleZone records
	// each zone's absolute screen position.
	return zone.Scan(lipgloss.JoinVertical(lipgloss.Left, body, footer))
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
			name = name + "/"
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
		"h/←: back", "l/→: forward", "q: quit",
	}
	help := strings.Join(keys, "  ")
	loc := m.status
	if loc != "" {
		// Show path relative to root for brevity.
		if rel, err := filepath.Rel(m.root, loc); err == nil {
			loc = rel
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
	w := m.width / 4
	if w < 20 {
		w = 20
	}
	if w > 40 {
		w = 40
	}
	return w
}

func paneStyle(focused bool) lipgloss.Style {
	border := lipgloss.NormalBorder()
	color := lipgloss.Color("240")
	if focused {
		color = lipgloss.Color("62")
	}
	return lipgloss.NewStyle().Border(border).BorderForeground(color)
}

package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// twoPaneMinWidth is the minimum terminal width at which the tree
// pane is shown alongside content. Mirrors backlinksMinTotalHeight on
// the height axis.
const twoPaneMinWidth = 80

// treeUIState bundles the left tree pane's render state. flat is the
// pre-flattened row list; cursor indexes into it; vp scrolls the pane
// when flat exceeds pane height; visible is the user-facing intent
// (gated by width via shouldShowTree); expanded stores deviations
// from the default-expanded folder state.
type treeUIState struct {
	flat     []treeRow
	cursor   int
	vp       viewport.Model
	visible  bool
	expanded map[string]bool
}

// shouldShowTree gates m.tree.visible (user intent) on terminal width.
// Below twoPaneMinWidth the tree is force-hidden so content gets the
// full window. Parallels shouldShowBacklinks on the height axis.
func (m Model) shouldShowTree() bool {
	return m.tree.visible && m.width >= twoPaneMinWidth
}

func (m Model) View() string {
	if m.width == 0 {
		return "" // wait for first WindowSizeMsg
	}

	content := m.content.viewport.View()

	contentHeight := m.height - 4
	if m.shouldShowBacklinks() {
		contentHeight -= backlinksHeight
	}
	contentStyled := zone.Mark(zoneContentPane, paneStyle(m.focus == focusContent).
		Width(m.content.viewport.Width).
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
			Render(m.tree.vp.View()))
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

// refreshTreeVP populates the tree viewport with the current rendered
// rows and scrolls so that m.tree.cursor is within the visible window.
// Call after every write to m.tree.flat or m.tree.cursor.
func (m *Model) refreshTreeVP() {
	m.tree.vp.SetContent(m.renderTree())
	if m.tree.cursor < m.tree.vp.YOffset {
		m.tree.vp.SetYOffset(m.tree.cursor)
	} else if last := m.tree.vp.YOffset + m.tree.vp.Height - 1; m.tree.cursor > last {
		m.tree.vp.SetYOffset(m.tree.cursor - m.tree.vp.Height + 1)
	}
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
	help := "?: help  q: quit"

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
		loc = fmt.Sprintf("%s%s [%d/%d] %s", linkFooterMarker, loc, m.content.linkCursor+1, len(m.content.links), linkLabel(*sel, m.root))
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

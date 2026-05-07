package tui

import (
	"fmt"
	"path/filepath"
	"strings"

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

// shouldShowBacklinks returns true when the persistent pane is open
// AND the terminal is tall enough for it.
func (m Model) shouldShowBacklinks() bool {
	return m.backlinksOpen && m.height >= backlinksMinTotalHeight
}

// refreshBacklinks repopulates m.backlinksVP from the vault for the
// currently-open file. Called on file change and on toggle.
func (m *Model) refreshBacklinks(currentPath string) {
	if m.vault == nil || currentPath == "" {
		m.backlinksVP.SetContent("")
		return
	}
	links := m.vault.Backlinks(currentPath)
	m.backlinksVP.SetContent(formatBacklinks(links, m.root, m.viewport.Width))
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

// formatBacklinks renders a slice of vault.Backlink as the two-row-per-
// entry text used in both the persistent pane and the modal.
func formatBacklinks(links []vault.Backlink, root string, width int) string {
	if len(links) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("(no backlinks)")
	}
	var b strings.Builder
	for _, l := range links {
		rel, err := filepath.Rel(root, l.SourceFile)
		if err != nil {
			rel = l.SourceFile
		}
		fmt.Fprintf(&b, "%s:%d\n", rel, l.Line)
		fmt.Fprintf(&b, "  %s\n", truncateOneLine(applyHighlight(l.Snippet), width-2))
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

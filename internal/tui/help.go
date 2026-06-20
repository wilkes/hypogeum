package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// refreshHelpModal repopulates m.modals.vp with the keybinding cheat sheet,
// grouped into logical sections. The section list in formatHelp is
// curated for UX (ordering and grouping matter), so a new keyMap field
// won't surface here automatically — add it to the right section below.
func (m *Model) refreshHelpModal() {
	m.resizeModalVP()
	m.modals.vp.SetContent(formatHelp(m.keys))
}

// formatHelp renders the keymap into a sectioned cheat sheet.
func formatHelp(k keyMap) string {
	sections := []struct {
		title string
		rows  []key.Binding
	}{
		{"Navigation", []key.Binding{k.Back, k.Forward, k.Up, k.Down, k.Open, k.Quit}},
		{"Scrolling", []key.Binding{k.HalfPageDown, k.HalfPageUp, k.Top, k.Bottom}},
		{"Tree", []key.Binding{k.ToggleTree, k.ToggleFolder}},
		{"Links", []key.Binding{k.NextLink, k.PrevLink}},
		{"Selection", []key.Binding{k.EnterVisual, k.BeginSelect}},
		{"Clipboard", []key.Binding{k.CopyPath}},
		{"Modals", []key.Binding{k.OpenPicker, k.OpenSearch, k.OpenBacklinksModal, k.OpenRecentModal, k.OpenLogsModal, k.OpenHelpModal, k.ClearLink}},
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Underline(true)
	keyStyle := lipgloss.NewStyle().Bold(true)

	var b strings.Builder
	for i, s := range sections {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(titleStyle.Render(s.title))
		b.WriteByte('\n')
		for _, kb := range s.rows {
			h := kb.Help()
			fmt.Fprintf(&b, "  %s  %s\n", keyStyle.Render(padRight(h.Key, 8)), h.Desc)
		}
	}
	return b.String()
}

// padRight pads s with spaces to visible width w (no truncation). Uses
// cell width rather than byte length so multi-byte runes like `↑/k` align
// correctly with ASCII labels like `enter`.
func padRight(s string, w int) string {
	width := ansi.StringWidth(s)
	if width >= w {
		return s
	}
	return s + strings.Repeat(" ", w-width)
}

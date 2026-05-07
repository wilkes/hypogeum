package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// refreshLogsModal repopulates m.modalVP with the diagnostic ring
// buffer formatted for display.
func (m *Model) refreshLogsModal() {
	m.resizeModalVP()
	entries := m.diag.snapshot()
	m.modalVP.SetContent(formatLogEntries(entries))
}

// formatLogEntries renders the ring buffer as one line per entry,
// with severity-colored prefix and timestamp.
func formatLogEntries(entries []diagEntry) string {
	if len(entries) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("(no entries)")
	}
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s %s %s\n",
			e.Timestamp.Format("15:04:05"),
			styleSeverity(e.Severity),
			e.Message,
		)
	}
	return b.String()
}

func styleSeverity(s severity) string {
	switch s {
	case sevError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("ERR ")
	case sevWarn:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("WARN")
	default:
		return lipgloss.NewStyle().Faint(true).Render("INFO")
	}
}

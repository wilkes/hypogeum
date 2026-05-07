package tui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// debugMouse logs every left-click to /tmp/hypogeum-mouse.log for
// diagnostic purposes. Enabled via HYPOGEUM_DEBUG=mouse env var.
func debugMouse(msg tea.MouseMsg, m *Model) {
	if os.Getenv("HYPOGEUM_DEBUG") != "mouse" {
		return
	}
	f, err := os.OpenFile("/tmp/hypogeum-mouse.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "click X=%d Y=%d  treePane=", msg.X, msg.Y)
	if z := zone.Get(zoneTreePane); !z.IsZero() {
		fmt.Fprintf(f, "(%d,%d)-(%d,%d)", z.StartX, z.StartY, z.EndX, z.EndY)
	}
	fmt.Fprintf(f, "  rows:")
	for i := range m.flatTree {
		z := zone.Get(treeRowZoneID(i))
		if z.IsZero() {
			continue
		}
		hit := ""
		if z.InBounds(msg) {
			hit = "*"
		}
		fmt.Fprintf(f, " [%d%s y=%d %s]", i, hit, z.StartY, m.flatTree[i].node.Name)
	}
	fmt.Fprintln(f)
}

// All Update-path helpers below take a pointer receiver and return
// (tea.Model, tea.Cmd) where the model is *m. Update itself stays on a
// value receiver to satisfy tea.Model; it dereferences once before
// dispatching here. This keeps mutation sites unambiguous: if a method
// changes m, it's a *Model method, and the returned tea.Model is *m.

// handleMouse routes a mouse event to the pane (or link) it lands in
// using BubbleZone hit-testing. Wheel events go straight to the viewport
// (it scrolls regardless of click position). Left-button presses are
// dispatched in priority order: tree row first, then link in content,
// then content pane fall-through to viewport.
func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if tea.MouseEvent(msg).IsWheel() {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return *m, cmd
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return *m, nil
	}

	debugMouse(msg, m)

	// Tree row hit. Iterate visible rows; the first that contains the
	// click wins. Stops at len(m.flatTree) so out-of-range zones from a
	// previous longer document don't match.
	for i := range m.flatTree {
		if zone.Get(treeRowZoneID(i)).InBounds(msg) {
			return m.clickTree(i)
		}
	}

	// Content link hit. Each link's visible text is a separate zone so a
	// click on any cell of (possibly word-wrapped) link text follows it.
	for i, l := range m.links {
		if zone.Get(linkZoneID(i)).InBounds(msg) {
			m.focus = focusContent
			m.linkCursor = i
			m.followLink(l)
			return *m, nil
		}
	}

	// Content pane fall-through: any click inside the content pane that
	// missed a link gives focus and forwards to the viewport (so future
	// viewport features like text selection keep working).
	if zone.Get(zoneContentPane).InBounds(msg) {
		m.focus = focusContent
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return *m, cmd
	}

	return *m, nil
}

// clickTree selects the tree row at index row (relative to the visible
// flatTree top), opens it if it's a file, and switches focus.
func (m *Model) clickTree(row int) (tea.Model, tea.Cmd) {
	if row < 0 || row >= len(m.flatTree) {
		return *m, nil
	}
	m.focus = focusTree
	m.treeCursor = row
	if !m.flatTree[row].node.IsDir {
		m.openFile(m.flatTree[row].node.Path)
	}
	return *m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Modal-toggle keys take priority — they open/close modals regardless
	// of which pane has focus. They must run before the modal-forwarding
	// block below so that pressing a toggle while a modal is open swaps
	// or closes it.
	if key.Matches(msg, m.keys.OpenBacklinksModal) {
		if m.modalOpen == modalBacklinks {
			m.modalOpen = modalNone
		} else {
			m.modalOpen = modalBacklinks
			m.refreshBacklinksModal(m.history.Current())
		}
		return *m, nil
	}

	if key.Matches(msg, m.keys.OpenLogsModal) {
		if m.modalOpen == modalLogs {
			m.modalOpen = modalNone
		} else {
			m.modalOpen = modalLogs
			m.refreshLogsModal()
		}
		return *m, nil
	}

	// While a modal is open, Esc closes it; other keys go to the modal viewport.
	if m.modalOpen != modalNone {
		if key.Matches(msg, m.keys.ClearLink) { // Esc
			m.modalOpen = modalNone
			return *m, nil
		}
		var cmd tea.Cmd
		m.modalVP, cmd = m.modalVP.Update(msg)
		return *m, cmd
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return *m, tea.Quit

	case key.Matches(msg, m.keys.FocusTog):
		if m.focus == focusTree {
			m.focus = focusContent
		} else {
			m.focus = focusTree
		}
		return *m, nil

	case key.Matches(msg, m.keys.Back):
		if path, ok := m.history.Back(); ok {
			m.refreshContent(path)
			m.selectInTree(path)
		}
		return *m, nil

	case key.Matches(msg, m.keys.Forward):
		if path, ok := m.history.Forward(); ok {
			m.refreshContent(path)
			m.selectInTree(path)
		}
		return *m, nil

	case key.Matches(msg, m.keys.ToggleBacklinks):
		if m.backlinksOpen {
			m.backlinksOpen = false
			m.focus = m.prevFocus
		} else {
			m.backlinksOpen = true
			m.prevFocus = m.focus
			m.focus = focusBacklinks
			m.backlinkCursor = 0
			m.refreshBacklinks(m.history.Current())
		}
		return *m, nil
	}

	switch m.focus {
	case focusTree:
		return m.handleTreeKey(msg)
	case focusBacklinks:
		return m.handleBacklinksKey(msg)
	default:
		return m.handleContentKey(msg)
	}
}

// handleContentKey routes keystrokes received while the content pane has
// focus. Link-cycling bindings (n/p/Esc) and Enter (when a link is
// selected) are intercepted; everything else falls through to the
// viewport so its scrolling bindings keep working.
func (m *Model) handleContentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.NextLink):
		m.cycleLink(+1)
		return *m, nil
	case key.Matches(msg, m.keys.PrevLink):
		m.cycleLink(-1)
		return *m, nil
	case key.Matches(msg, m.keys.ClearLink):
		m.linkCursor = -1
		return *m, nil
	case key.Matches(msg, m.keys.Open):
		if sel := m.selectedLink(); sel != nil {
			m.followLink(*sel)
			return *m, nil
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return *m, cmd
}

// handleBacklinksKey routes keystrokes received while the persistent
// backlinks pane has focus. j/k move the cursor; Enter follows
// (added in Task 9); Esc returns focus to prevFocus.
func (m *Model) handleBacklinksKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.backlinkCursor < len(m.backlinks)-1 {
			m.backlinkCursor++
			m.refreshBacklinks(m.history.Current())
			m.ensureCursorVisible(&m.backlinksVP)
		}
		return *m, nil
	case key.Matches(msg, m.keys.Up):
		if m.backlinkCursor > 0 {
			m.backlinkCursor--
			m.refreshBacklinks(m.history.Current())
			m.ensureCursorVisible(&m.backlinksVP)
		}
		return *m, nil
	}
	return *m, nil
}

func (m *Model) handleTreeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.treeCursor > 0 {
			m.treeCursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.treeCursor < len(m.flatTree)-1 {
			m.treeCursor++
		}
	case key.Matches(msg, m.keys.Open):
		if m.treeCursor < len(m.flatTree) {
			row := m.flatTree[m.treeCursor]
			if !row.node.IsDir {
				m.openFile(row.node.Path)
			}
		}
	}
	return *m, nil
}

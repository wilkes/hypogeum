package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// All Update-path helpers below take a pointer receiver and return
// (tea.Model, tea.Cmd) where the model is *m. Update itself stays on a
// value receiver to satisfy tea.Model; it dereferences once before
// dispatching here. This keeps mutation sites unambiguous: if a method
// changes m, it's a *Model method, and the returned tea.Model is *m.

// handleMouse routes a mouse event by coordinate to the pane it lands in.
// Wheel events go straight to the viewport (it scrolls regardless of
// click position). Left-button presses select a tree row, follow a link,
// or — failing both — pass through to the viewport's own click handling.
func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if tea.MouseEvent(msg).IsWheel() {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return *m, cmd
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return *m, nil
	}

	// Pane interior bounds. Each Lip Gloss pane has a 1-char border on
	// every side, so interior cells start at (paneX+1, 1) and the body
	// ends at row m.height-3 (last row before the 2-line footer).
	treeW := m.treeWidth()
	bodyTop, bodyBottom := 1, m.height-3
	if msg.Y < bodyTop || msg.Y > bodyBottom {
		return *m, nil // footer or top border
	}
	row := msg.Y - bodyTop

	switch {
	case msg.X >= 1 && msg.X <= treeW: // inside tree pane (border + interior)
		return m.clickTree(row)
	case msg.X >= treeW+2: // inside content pane (skip both borders)
		return m.clickContent(row, msg)
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

// clickContent finds a link on the clicked row and follows it. Falls
// through to the viewport's own click handling otherwise (so future
// viewport features like text selection keep working).
func (m *Model) clickContent(row int, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m.focus = focusContent
	docRow := row + m.viewport.YOffset
	for i, l := range m.links {
		if l.Row == docRow {
			m.linkCursor = i
			m.followLink(l)
			return *m, nil
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return *m, cmd
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	}

	if m.focus == focusTree {
		return m.handleTreeKey(msg)
	}
	return m.handleContentKey(msg)
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

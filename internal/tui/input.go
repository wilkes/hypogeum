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
	for i := range m.tree.flat {
		z := zone.Get(treeRowZoneID(i))
		if z.IsZero() {
			continue
		}
		hit := ""
		if z.InBounds(msg) {
			hit = "*"
		}
		fmt.Fprintf(f, " [%d%s y=%d %s]", i, hit, z.StartY, m.tree.flat[i].node.Name)
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
		m.content.viewport, cmd = m.content.viewport.Update(msg)
		return *m, cmd
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return *m, nil
	}

	debugMouse(msg, m)

	// Tree row hit. Iterate visible rows; the first that contains the
	// click wins. Stops at len(m.tree.flat) so out-of-range zones from a
	// previous longer document don't match.
	for i := range m.tree.flat {
		if zone.Get(treeRowZoneID(i)).InBounds(msg) {
			return m.clickTree(i)
		}
	}

	// Content link hit. Each link's visible text is a separate zone so a
	// click on any cell of (possibly word-wrapped) link text follows it.
	for i, l := range m.content.links {
		if zone.Get(linkZoneID(i)).InBounds(msg) {
			m.focus = focusContent
			m.content.linkCursor = i
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
		m.content.viewport, cmd = m.content.viewport.Update(msg)
		return *m, cmd
	}

	return *m, nil
}

// clickTree selects the tree row at index row (relative to the visible
// flat tree top), opens it if it's a file, and switches focus.
func (m *Model) clickTree(row int) (tea.Model, tea.Cmd) {
	if row < 0 || row >= len(m.tree.flat) {
		return *m, nil
	}
	m.focus = focusTree
	m.tree.cursor = row
	m.refreshTreeVP()
	if !m.tree.flat[row].node.IsDir {
		m.openFile(m.tree.flat[row].node.Path)
	}
	return *m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Modal-toggle keys take priority — they open/close modals regardless
	// of which pane has focus. They must run before the modal-forwarding
	// block below so that pressing a toggle while a modal is open swaps
	// or closes it.
	switch {
	case key.Matches(msg, m.keys.OpenBacklinksModal):
		return *m, m.toggleModal(modalBacklinks, func() tea.Cmd {
			m.backlinks.cursor = 0
			m.refreshBacklinksModal(m.history.Current())
			return nil
		})
	case key.Matches(msg, m.keys.OpenLogsModal):
		return *m, m.toggleModal(modalLogs, func() tea.Cmd {
			m.refreshLogsModal()
			return nil
		})
	case key.Matches(msg, m.keys.OpenHelpModal):
		// Help is anchored: pressing `?` while a *different* modal is
		// open is a no-op (unlike B/^l which swap). Surface the no-op
		// as a footer transient so the user knows why nothing happened
		// instead of wondering if `?` is broken. `?` while help is
		// already open still toggles it closed via the toggleModal path.
		if m.modals.kind != modalNone && m.modals.kind != modalHelp {
			if m.diag != nil {
				m.diag.Info("close current modal (esc) before opening help")
			}
			return *m, nil
		}
		return *m, m.toggleModal(modalHelp, func() tea.Cmd {
			m.refreshHelpModal()
			return nil
		})
	case key.Matches(msg, m.keys.OpenPicker):
		return *m, m.toggleModal(modalPicker, func() tea.Cmd {
			// Each open starts fresh: cursor at top, all dirs collapsed.
			m.modals.picker.reset(m.rootNode)
			return nil
		})
	}

	// Toggle the tree pane. Synthesize a resize so the renderer and
	// viewport widths recompute through the existing WindowSizeMsg path.
	if key.Matches(msg, m.keys.ToggleTree) {
		m.tree.visible = !m.tree.visible
		return m.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	}

	// While a modal is open, Esc closes it. Backlinks modal gets explicit
	// cursor handling so j/k move the selection rather than scroll the
	// viewport. Logs modal keeps the viewport-scroll fall-through.
	if m.modals.kind != modalNone {
		if m.modals.kind == modalPicker {
			switch {
			case key.Matches(msg, m.keys.ClearLink): // Esc closes from any depth
				m.modals.kind = modalNone
				m.focus = m.modals.prevFocus
			case key.Matches(msg, m.keys.Up):
				if m.modals.picker.cursor > 0 {
					m.modals.picker.cursor--
					m.modals.picker.refreshVP()
				}
			case key.Matches(msg, m.keys.Down):
				if m.modals.picker.cursor < len(m.modals.picker.flat)-1 {
					m.modals.picker.cursor++
					m.modals.picker.refreshVP()
				}
			case key.Matches(msg, m.keys.ToggleFolder):
				m.modals.picker.toggleAt(m.rootNode)
			case key.Matches(msg, m.keys.Open):
				if path, ok := m.modals.picker.selectedFile(); ok {
					m.modals.kind = modalNone
					m.focus = m.modals.prevFocus
					m.navigateTo(path)
				} else {
					// On a directory: Enter expands/collapses it, same as space.
					m.modals.picker.toggleAt(m.rootNode)
				}
			}
			return *m, nil
		}
		if key.Matches(msg, m.keys.ClearLink) { // Esc
			m.modals.kind = modalNone
			m.focus = m.modals.prevFocus
			return *m, nil
		}
		if m.modals.kind == modalBacklinks {
			switch {
			case key.Matches(msg, m.keys.Down):
				if m.backlinks.cursor < len(m.backlinks.items)-1 {
					m.backlinks.cursor++
					m.refreshBacklinksModal(m.history.Current())
					m.ensureCursorVisible(&m.modals.vp)
				}
				return *m, nil
			case key.Matches(msg, m.keys.Up):
				if m.backlinks.cursor > 0 {
					m.backlinks.cursor--
					m.refreshBacklinksModal(m.history.Current())
					m.ensureCursorVisible(&m.modals.vp)
				}
				return *m, nil
			case key.Matches(msg, m.keys.Open):
				m.followBacklink()
				return *m, nil
			}
			// Fall through to viewport scroll for any other key.
		}
		var cmd tea.Cmd
		m.modals.vp, cmd = m.modals.vp.Update(msg)
		return *m, cmd
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return *m, tea.Quit

	case key.Matches(msg, m.keys.FocusTog):
		m.focus = m.nextFocus()
		return *m, nil

	case key.Matches(msg, m.keys.Back):
		if path, ok := m.history.Back(); ok {
			m.refreshContent(path)
			m.selectInTree(path)
			m.maybeRestoreReturnCursor(path)
		}
		return *m, nil

	case key.Matches(msg, m.keys.Forward):
		if path, ok := m.history.Forward(); ok {
			m.refreshContent(path)
			m.selectInTree(path)
		}
		return *m, nil

	case key.Matches(msg, m.keys.ToggleBacklinks):
		if m.backlinks.open {
			m.backlinks.open = false
			m.focus = m.modals.prevFocus
		} else {
			m.backlinks.open = true
			m.modals.prevFocus = m.focus
			m.focus = focusBacklinks
			m.backlinks.cursor = 0
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
		m.content.linkCursor = -1
		return *m, nil
	case key.Matches(msg, m.keys.Open):
		if sel := m.selectedLink(); sel != nil {
			m.followLink(*sel)
			return *m, nil
		}
	}
	var cmd tea.Cmd
	m.content.viewport, cmd = m.content.viewport.Update(msg)
	return *m, cmd
}

// handleBacklinksKey routes keystrokes received while the persistent
// backlinks pane has focus. j/k move the cursor; Enter follows
// (added in Task 9); Esc restores focus to prevFocus without closing the pane.
func (m *Model) handleBacklinksKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.ClearLink): // Esc
		m.focus = m.modals.prevFocus
		return *m, nil
	case key.Matches(msg, m.keys.Down):
		if m.backlinks.cursor < len(m.backlinks.items)-1 {
			m.backlinks.cursor++
			m.refreshBacklinks(m.history.Current())
			m.ensureCursorVisible(&m.backlinks.vp)
		}
		return *m, nil
	case key.Matches(msg, m.keys.Up):
		if m.backlinks.cursor > 0 {
			m.backlinks.cursor--
			m.refreshBacklinks(m.history.Current())
			m.ensureCursorVisible(&m.backlinks.vp)
		}
		return *m, nil
	case key.Matches(msg, m.keys.Open):
		m.followBacklink()
		return *m, nil
	}
	return *m, nil
}

func (m *Model) handleTreeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.tree.cursor > 0 {
			m.tree.cursor--
			m.refreshTreeVP()
		}
	case key.Matches(msg, m.keys.Down):
		if m.tree.cursor < len(m.tree.flat)-1 {
			m.tree.cursor++
			m.refreshTreeVP()
		}
	case key.Matches(msg, m.keys.ToggleFolder):
		if row, ok := m.cursorRow(); ok && row.node.IsDir {
			m.toggleFolder(row.node.Path)
		}
	case key.Matches(msg, m.keys.Open):
		row, ok := m.cursorRow()
		if !ok {
			return *m, nil
		}
		if row.node.IsDir {
			m.toggleFolder(row.node.Path)
		} else {
			m.openFile(row.node.Path)
		}
	}
	return *m, nil
}

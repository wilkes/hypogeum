package tui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/wilkes/hypogeum/internal/recent"
	"github.com/wilkes/hypogeum/internal/tree"
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
	fmt.Fprintf(f, "click X=%d Y=%d  rows:", msg.X, msg.Y)
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
	// previous longer document don't match. Active only when the tree
	// modal is open — BubbleZone keeps stale zones across re-renders,
	// so a closed modal mustn't catch clicks.
	if m.modals.kind == modalTree {
		for i := range m.tree.flat {
			if zone.Get(treeRowZoneID(i)).InBounds(msg) {
				return m.clickTree(i)
			}
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
// flat tree top). On a directory it toggles collapse; on a file it
// opens the file and closes the modal (matching keyboard Enter).
func (m *Model) clickTree(row int) (tea.Model, tea.Cmd) {
	if row < 0 || row >= len(m.tree.flat) {
		return *m, nil
	}
	m.tree.cursor = row
	m.refreshTreeVP()
	node := m.tree.flat[row].node
	if node.IsDir {
		m.toggleFolder(node.Path)
		return *m, nil
	}
	m.closeModal()
	m.openFile(node.Path)
	return *m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// The picker's text input claims every printable keystroke. Route
	// printable keys to it first so global modal-toggle keys that are
	// plain letters (b, ?) don't swap the picker out when the user
	// types them into the query. ^P / ^L still toggle modals (they're
	// not text-input candidates), so the user can close the picker the
	// same way they opened it.
	if m.modals.kind == modalPicker && msg.Type == tea.KeyRunes {
		return m.handlePickerKey(msg)
	}
	if m.modals.kind == modalSearch && msg.Type == tea.KeyRunes {
		return m.handleSearchKey(msg)
	}

	// Modal-toggle keys take priority — they open/close modals regardless
	// of which pane has focus. They must run before the modal-forwarding
	// block below so that pressing a toggle while a modal is open swaps
	// or closes it.
	switch {
	case key.Matches(msg, m.keys.OpenBacklinksModal):
		return *m, m.openModalWith(modalBacklinks, func() {
			m.backlinks.cursor = 0
			m.refreshBacklinksModal(m.history.Current())
		})
	case key.Matches(msg, m.keys.OpenLogsModal):
		return *m, m.openModalWith(modalLogs, func() {
			m.refreshLogsModal()
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
		return *m, m.openModalWith(modalHelp, func() {
			m.refreshHelpModal()
		})
	case key.Matches(msg, m.keys.OpenPicker):
		return *m, m.openModalWith(modalPicker, func() {
			paths := m.allVaultMarkdownPaths()
			ranked := []recent.Ranked{}
			if m.recent != nil {
				ranked = m.recent.Rank(paths)
			}
			m.modals.picker.reset(ranked, m.root)
		})
	case key.Matches(msg, m.keys.OpenSearch):
		return *m, m.openModalWith(modalSearch, func() {
			m.modals.search.reset(m.allVaultMarkdownPaths())
		})
	case key.Matches(msg, m.keys.ToggleTree):
		return *m, m.openModalWith(modalTree, func() {
			// Ensure the cursor points at the file currently open so the
			// modal lands the user where they are in the vault.
			if cur := m.history.Current(); cur != "" {
				m.selectInTree(cur)
			} else {
				m.refreshTreeVP()
			}
		})
	}

	// While a modal is open, Esc closes it. Backlinks modal gets explicit
	// cursor handling so j/k move the selection rather than scroll the
	// viewport. Logs modal keeps the viewport-scroll fall-through.
	if m.modals.kind != modalNone {
		if m.modals.kind == modalPicker {
			switch {
			case key.Matches(msg, m.keys.ClearLink): // Esc
				if m.modals.picker.input.Value() != "" {
					m.modals.picker.input.SetValue("")
					m.modals.picker.refilter()
					return *m, nil
				}
				m.closeModal()
				return *m, nil
			case key.Matches(msg, m.keys.Open):
				if path, ok := m.modals.picker.selectedPath(); ok {
					m.closeModal()
					m.navigateTo(path)
				}
				return *m, nil
			case key.Matches(msg, m.keys.Up),
				key.Matches(msg, m.keys.PickerCursorUp):
				if m.modals.picker.cursor > 0 {
					m.modals.picker.cursor--
					m.modals.picker.refreshVP()
				}
				return *m, nil
			case key.Matches(msg, m.keys.Down),
				key.Matches(msg, m.keys.PickerCursorDown):
				lim := len(m.modals.picker.ranked)
				if lim > pickerMaxVisible {
					lim = pickerMaxVisible
				}
				if m.modals.picker.cursor < lim-1 {
					m.modals.picker.cursor++
					m.modals.picker.refreshVP()
				}
				return *m, nil
			}
			// Forward anything else to the textinput; refilter on change.
			before := m.modals.picker.input.Value()
			var cmd tea.Cmd
			m.modals.picker.input, cmd = m.modals.picker.input.Update(msg)
			if m.modals.picker.input.Value() != before {
				m.modals.picker.refilter()
			}
			return *m, cmd
		}
		if key.Matches(msg, m.keys.ClearLink) { // Esc
			m.closeModal()
			return *m, nil
		}
		if m.modals.kind == modalTree {
			return m.handleTreeModalKey(msg)
		}
		if m.modals.kind == modalBacklinks {
			refresh := func() {
				m.refreshBacklinksModal(m.history.Current())
				m.ensureCursorVisible(&m.modals.vp)
			}
			switch {
			case key.Matches(msg, m.keys.Down):
				cursorMoveAndRefresh(&m.backlinks.cursor, len(m.backlinks.items), +1, refresh)
				return *m, nil
			case key.Matches(msg, m.keys.Up):
				cursorMoveAndRefresh(&m.backlinks.cursor, len(m.backlinks.items), -1, refresh)
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

	case key.Matches(msg, m.keys.Back):
		leaving := m.history.Current()
		leavingRange := m.content.rangeHighlight
		if path, ok := m.history.Back(); ok {
			m.pendingPreselectTarget = leaving
			m.pendingPreselectRange = leavingRange
			m.refreshContent(path)
			m.selectInTree(path)
			m.maybeRestoreReturnCursor(path)
		}
		return *m, nil

	case key.Matches(msg, m.keys.Forward):
		leaving := m.history.Current()
		leavingRange := m.content.rangeHighlight
		if path, ok := m.history.Forward(); ok {
			m.pendingPreselectTarget = leaving
			m.pendingPreselectRange = leavingRange
			m.refreshContent(path)
			m.selectInTree(path)
		}
		return *m, nil
	}

	return m.handleContentKey(msg)
}

// handlePickerKey forwards printable runes to the picker's text input
// and refilters the result list. Called from handleKey's pre-dispatch
// guard so plain-letter modal-toggle keys (b, ?) don't swap the picker
// out when the user is typing into the query. Non-rune picker keys
// (Esc, Enter, Up/Down, ^j/^k) still flow through the main modal block
// below in handleKey.
func (m *Model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	before := m.modals.picker.input.Value()
	var cmd tea.Cmd
	m.modals.picker.input, cmd = m.modals.picker.input.Update(msg)
	if m.modals.picker.input.Value() != before {
		m.modals.picker.refilter()
	}
	return *m, cmd
}

// handleContentKey routes keystrokes received while the content pane has
// focus. Link-cycling bindings (n/p/Esc) and Enter (when a link is
// selected) are intercepted; everything else falls through to the
// viewport so its scrolling bindings keep working.
func (m *Model) handleContentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// One-keystroke confirm for external URL handoff: if a previous
	// Enter on an external link armed pendingExternal, a second Enter
	// exec's the opener. Any other keystroke cancels the prompt and
	// then falls through to normal dispatch so the user doesn't lose
	// the keystroke they pressed.
	if m.pendingExternal != "" {
		armed := m.pendingExternal
		m.pendingExternal = ""
		if key.Matches(msg, m.keys.Open) {
			if err := m.openExternal(armed); err != nil {
				m.status = "open failed: " + err.Error()
			} else {
				m.status = "opened: " + armed
			}
			return *m, nil
		}
		m.status = ""
		// Fall through: process this keystroke as if nothing was armed.
	}

	switch {
	case key.Matches(msg, m.keys.NextLink):
		m.cycleLink(+1)
		return *m, nil
	case key.Matches(msg, m.keys.PrevLink):
		m.cycleLink(-1)
		return *m, nil
	case key.Matches(msg, m.keys.ClearLink):
		// Esc cascade (most-specific to least-specific):
		// 1. If the open file is a non-markdown source viewed with a
		//    range highlight, clearing the highlight is the user's
		//    likely intent. Fires before link-cursor clear so the
		//    next Esc still resets the link cursor for markdown.
		cur := m.history.Current()
		if m.content.rangeHighlight != nil && !tree.IsMarkdown(cur) {
			offset := m.content.viewport.YOffset
			m.content.rangeHighlight = nil
			m.refreshContent(cur)
			m.content.viewport.SetYOffset(offset)
			return *m, nil
		}
		offset := m.content.viewport.YOffset
		m.content.linkCursor = -1
		m.refreshContent(cur)
		m.content.viewport.SetYOffset(offset)
		return *m, nil
	case key.Matches(msg, m.keys.Open):
		if m.selectedLink() != nil {
			m.followCurrentLink()
			return *m, nil
		}
	}
	var cmd tea.Cmd
	m.content.viewport, cmd = m.content.viewport.Update(msg)
	return *m, cmd
}


// handleTreeModalKey routes keystrokes while the tree modal is open.
// Up/Down/k/j move the cursor; Space toggles a folder; Left/h collapses
// an expanded directory; Right/l expands a collapsed directory; Enter
// opens a file (closing the modal) or toggles a folder. Back/Forward
// are intercepted here so they shadow their usual history meaning only
// while the tree modal is open.
func (m *Model) handleTreeModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	case key.Matches(msg, m.keys.Back):
		if row, ok := m.cursorRow(); ok && row.node.IsDir && m.tree.expanded[row.node.Path] {
			m.toggleFolder(row.node.Path)
		}
	case key.Matches(msg, m.keys.Forward):
		if row, ok := m.cursorRow(); ok && row.node.IsDir && !m.tree.expanded[row.node.Path] {
			m.toggleFolder(row.node.Path)
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
			return *m, nil
		}
		m.closeModal()
		m.openFile(row.node.Path)
	}
	return *m, nil
}

package tui

import (
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/wilkes/hypogeum/internal/markdown"
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

// handleMouse routes a mouse event using BubbleZone hit-testing. Wheel
// events scroll the viewport regardless of position. A left-press inside
// the content pane (no modal open) arms a text selection, remembering any
// link under the press but not following it yet. Motion while armed
// extends the selection and repaints the highlight; release either copies
// the selected text (if the pointer moved) or, on a no-motion click,
// follows the remembered link. While the tree modal is open, a press on a
// tree row is dispatched to clickTree instead.
func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if tea.MouseEvent(msg).IsWheel() {
		var cmd tea.Cmd
		m.content.viewport, cmd = m.content.viewport.Update(msg)
		return *m, cmd
	}

	// Motion / release only matter while a content-pane selection is
	// being tracked. Gating on `anchored` (not the button, which some
	// terminals report as None on release) keeps this robust.
	switch msg.Action {
	case tea.MouseActionMotion:
		if m.content.selection.anchored {
			return m.dragSelect(msg)
		}
		return *m, nil
	case tea.MouseActionRelease:
		if m.content.selection.anchored {
			return m.endSelect()
		}
		return *m, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return *m, nil
	}

	debugMouse(msg, m)

	// Tree row hit: dispatch to the first visible row containing the
	// press. Gated on the tree modal being open — BubbleZone keeps stale
	// zones across re-renders, so a closed modal mustn't catch clicks.
	if m.modals.kind == modalTree {
		for i := range m.tree.flat {
			if zone.Get(treeRowZoneID(i)).InBounds(msg) {
				return m.clickTree(i)
			}
		}
	}

	// Content-pane press: start a potential selection. Only when no modal
	// is open (modals keep their own click behavior). Remember a link
	// under the press so a no-motion release still follows it; do NOT
	// follow it here — the first motion event turns this into a drag.
	if m.modals.kind == modalNone && zone.Get(zoneContentPane).InBounds(msg) {
		m.clearSelection() // drop any prior finalized highlight or visual-mode caret
		m.focus = focusContent
		pos := m.screenToContent(msg.X, msg.Y)
		link := -1
		for i := range m.content.links {
			if zone.Get(linkZoneID(i)).InBounds(msg) {
				link = i
				break
			}
		}
		m.content.selection = selection{
			anchored:    true,
			anchor:      pos,
			cursor:      pos,
			pendingLink: link,
		}
		return *m, nil
	}

	return *m, nil
}

// dragSelect extends the in-progress selection to the motion point and
// repaints the highlight.
func (m *Model) dragSelect(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m.content.selection.cursor = m.screenToContent(msg.X, msg.Y)
	m.content.selection.moved = true
	m.applySelectionHighlight()
	return *m, nil
}

// endSelect finalizes the selection on button release. With motion, it
// copies the selected text (keeping the highlight) and toasts. Without
// motion, it was a click: follow the remembered link if any.
func (m *Model) endSelect() (tea.Model, tea.Cmd) {
	sel := m.content.selection
	if sel.moved {
		text := m.extractSelection()
		if n := utf8.RuneCountInString(text); n > 0 {
			m.copyToClipboard(text)
			m.diag.Info(fmt.Sprintf("Copied %d chars", n))
			m.finalizeSelection()
			return *m, nil
		}
		// Zero-width drag → treat as a click with no link.
		m.clearSelection()
		return *m, nil
	}

	link := sel.pendingLink
	m.clearSelection()
	if link >= 0 && link < len(m.content.links) {
		m.focus = focusContent
		m.content.linkCursor = link
		m.followLink(m.content.links[link])
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
	cmd := m.closeModal()
	m.openFile(node.Path)
	return *m, cmd
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// A finalized selection keeps its highlight until the user acts.
	// Any keystroke drops it; the key still performs its normal action.
	if m.content.selection.copied {
		m.clearSelection()
	}

	// Keyboard visual mode intercepts every key while active — before any
	// modal-toggle or global binding. Visual mode is content-pane only and
	// never coexists with an open modal.
	if m.content.selection.visual {
		return m.handleVisualKey(msg)
	}

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
			m.refreshSearchVP()
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
				return *m, m.closeModal()
			case key.Matches(msg, m.keys.Open):
				var cmd tea.Cmd
				if path, ok := m.modals.picker.selectedPath(); ok {
					cmd = m.closeModal()
					m.navigateTo(path)
				}
				return *m, cmd
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
		if m.modals.kind == modalSearch {
			switch {
			case key.Matches(msg, m.keys.ClearLink): // Esc
				if m.modals.search.input.Value() != "" {
					m.modals.search.input.SetValue("")
					m.modals.search.hits = nil
					m.modals.search.cursor = 0
					if m.modals.search.scanStop != nil {
						m.modals.search.scanStop()
						m.modals.search.scanStop = nil
					}
					m.modals.search.inFlight = false
					m.refreshSearchVP()
					return *m, nil
				}
				return *m, m.closeModal()
			case key.Matches(msg, m.keys.Open): // Enter
				var cmd tea.Cmd
				if 0 <= m.modals.search.cursor && m.modals.search.cursor < len(m.modals.search.hits) {
					h := m.modals.search.hits[m.modals.search.cursor]
					cmd = m.closeModal()
					m.pending.preselectRange = &markdown.LineRange{Start: h.Line, End: h.Line}
					m.navigateTo(h.Path)
				}
				return *m, cmd
			case key.Matches(msg, m.keys.SearchCursorDown):
				cursorMoveAndRefresh(&m.modals.search.cursor, len(m.modals.search.hits), 1, m.refreshSearchVP)
				return *m, nil
			case key.Matches(msg, m.keys.SearchCursorUp):
				cursorMoveAndRefresh(&m.modals.search.cursor, len(m.modals.search.hits), -1, m.refreshSearchVP)
				return *m, nil
			case key.Matches(msg, m.keys.Up):
				cursorMoveAndRefresh(&m.modals.search.cursor, len(m.modals.search.hits), -1, m.refreshSearchVP)
				return *m, nil
			case key.Matches(msg, m.keys.Down):
				cursorMoveAndRefresh(&m.modals.search.cursor, len(m.modals.search.hits), 1, m.refreshSearchVP)
				return *m, nil
			}
			// Forward unhandled keys (Backspace, Delete, ←/→) to the
			// textinput so the user can edit the query. Mirrors picker.
			return m.handleSearchKey(msg)
		}
		if key.Matches(msg, m.keys.ClearLink) { // Esc
			return *m, m.closeModal()
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
			m.pending.preselectTarget = leaving
			m.pending.preselectRange = leavingRange
			m.refreshContent(path)
			m.selectInTree(path)
			m.maybeRestoreReturnCursor(path)
		}
		return *m, nil

	case key.Matches(msg, m.keys.Forward):
		leaving := m.history.Current()
		leavingRange := m.content.rangeHighlight
		if path, ok := m.history.Forward(); ok {
			m.pending.preselectTarget = leaving
			m.pending.preselectRange = leavingRange
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

// copyCurrentPath copies the absolute path of the currently-viewed file
// (or directory) to the clipboard and toasts the result in the footer.
// No-op when nothing is open.
func (m *Model) copyCurrentPath() {
	path := m.history.Current()
	if path == "" {
		return
	}
	m.copyToClipboard(path)
	if m.diag != nil {
		m.diag.Info("Copied path: " + path)
	}
}

// handleContentKey routes keystrokes received while the content pane has
// focus. Link-cycling bindings (n/p/Esc) and Enter (when a link is
// selected) are intercepted; everything else falls through to the
// viewport so its scrolling bindings keep working.
func (m *Model) handleContentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// One-keystroke confirm for external URL handoff: if a previous
	// Enter on an external link armed pending.externalURL, a second Enter
	// exec's the opener. Any other keystroke cancels the prompt and
	// then falls through to normal dispatch so the user doesn't lose
	// the keystroke they pressed.
	if m.pending.externalURL != "" {
		armed := m.pending.externalURL
		m.pending.externalURL = ""
		if key.Matches(msg, m.keys.Open) {
			if err := m.openExternal(armed); err != nil {
				m.footerMessage = "open failed: " + err.Error()
			} else {
				m.footerMessage = "opened: " + armed
			}
			return *m, nil
		}
		m.footerMessage = ""
		// Fall through: process this keystroke as if nothing was armed.
	}

	switch {
	case key.Matches(msg, m.keys.NextLink):
		m.cycleLink(+1)
		return *m, nil
	case key.Matches(msg, m.keys.PrevLink):
		m.cycleLink(-1)
		return *m, nil
	case key.Matches(msg, m.keys.EnterVisual):
		m.enterVisual()
		return *m, nil
	case key.Matches(msg, m.keys.CopyPath):
		m.copyCurrentPath()
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
	case key.Matches(msg, m.keys.Top):
		m.content.viewport.GotoTop()
		return *m, nil
	case key.Matches(msg, m.keys.Bottom):
		m.content.viewport.GotoBottom()
		return *m, nil
	case key.Matches(msg, m.keys.HalfPageDown):
		m.content.viewport.HalfViewDown()
		return *m, nil
	case key.Matches(msg, m.keys.HalfPageUp):
		m.content.viewport.HalfViewUp()
		return *m, nil
	}
	var cmd tea.Cmd
	m.content.viewport, cmd = m.content.viewport.Update(msg)
	return *m, cmd
}

// handleVisualKey routes every keystroke while keyboard visual mode is
// active. Char/line motions are matched on the raw key (h/j/k/l + arrows)
// rather than the Back/Forward/Up/Down keyMap fields, because the modern
// dialect binds Back/Forward to alt+arrows — plain arrows must still move
// the caret. Jumps (g/G, ^d/^u) reuse the dialect-aware Top/Bottom/HalfPage
// fields. Yank reuses the dialect's copy key; Space drops the anchor; Esc
// cancels. Any other key is inert.
func (m *Model) handleVisualKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.contentLines()) == 0 {
		return *m, nil
	}
	cur := m.content.selection.cursor
	half := m.content.viewport.Height / 2
	if half < 1 {
		half = 1
	}
	last := len(m.contentLines()) - 1

	switch {
	case key.Matches(msg, m.keys.Quit): // ^c / q exits the app from visual mode too
		return *m, tea.Quit
	case key.Matches(msg, m.keys.ClearLink): // Esc
		m.clearSelection()
		return *m, nil
	case key.Matches(msg, m.keys.CopyPath): // y / ^y → yank
		m.yankVisual()
		return *m, nil
	case key.Matches(msg, m.keys.BeginSelect): // Space → drop anchor
		m.content.selection.selecting = true
		return *m, nil
	case key.Matches(msg, m.keys.Top): // g
		m.placeCaret(0, 0)
		return *m, nil
	case key.Matches(msg, m.keys.Bottom): // G → end of the last line (doc bottom)
		m.placeCaret(last, m.content.lineWidths[last])
		return *m, nil
	case key.Matches(msg, m.keys.HalfPageDown): // ^d
		m.placeCaret(cur.line+half, cur.col)
		return *m, nil
	case key.Matches(msg, m.keys.HalfPageUp): // ^u
		m.placeCaret(cur.line-half, cur.col)
		return *m, nil
	}

	switch msg.String() {
	case "h", "left":
		m.placeCaret(cur.line, cur.col-1)
	case "l", "right":
		m.placeCaret(cur.line, cur.col+1)
	case "k", "up":
		m.placeCaret(cur.line-1, cur.col)
	case "j", "down":
		m.placeCaret(cur.line+1, cur.col)
	}
	return *m, nil
}

// yankVisual copies the current selection to the clipboard, toasts the
// count, and finalizes the selection so its highlight persists until the
// user's next action. A zero-width selection (still positioning, or a
// collapsed span) copies nothing and just exits.
func (m *Model) yankVisual() {
	text := m.extractSelection()
	if n := utf8.RuneCountInString(text); n > 0 {
		m.copyToClipboard(text)
		m.diag.Info(fmt.Sprintf("Copied %d chars", n))
		m.finalizeSelection()
		return
	}
	m.clearSelection()
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
		cmd := m.closeModal()
		m.openFile(row.node.Path)
		return *m, cmd
	}
	return *m, nil
}

package tui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/viewport"
	zone "github.com/lrstanley/bubblezone"

	"github.com/wilkes/hypogeum/internal/markdown"
	"github.com/wilkes/hypogeum/internal/tree"
	"github.com/wilkes/hypogeum/internal/watch"
)

// contentUIState bundles the right content pane's render state. viewport
// scrolls the rendered markdown; renderer is rebuilt at every WindowSizeMsg
// so wrap width tracks pane width; links is the per-document link list
// from the latest render; linkCursor indexes into links (-1 when nothing
// is selected).
type contentUIState struct {
	viewport   viewport.Model
	renderer   *markdown.Renderer
	links      []markdown.Link
	linkCursor int
}

// linkZoneID returns the BubbleZone id used to track the i-th link in
// the rendered content. Stable across re-renders of the same document
// because zones are re-Marked every render; transient between documents
// because the link count and meaning change on each open.
func linkZoneID(i int) string {
	return fmt.Sprintf("link:%d", i)
}

// linkZoneMarker is the markdown.LinkMarker passed into RenderWithLinks.
// It returns the bubblezone open/close sentinel pair for the i-th link
// so a click on rendered link text can be matched to the link index
// without coordinate math.
//
// BubbleZone's Mark(id, body) emits "<gid>body<gid>" where <gid> is the
// same on both sides. To get the bare sentinel, we mark a placeholder
// and split around it. Mark(id, "") short-circuits to "", so we have to
// use a non-empty placeholder.
func linkZoneMarker(i int) (string, string) {
	const placeholder = "\x00"
	wrapped := zone.Mark(linkZoneID(i), placeholder)
	if wrapped == placeholder {
		// Zone manager disabled — emit no markers; downstream still works.
		return "", ""
	}
	mid := len(wrapped) / 2 // wrapped == gid + placeholder + gid; placeholder is 1 byte
	return wrapped[:mid], wrapped[mid+len(placeholder):]
}

// openFile records a visit in history and renders the file.
func (m *Model) openFile(path string) {
	m.history.Visit(path)
	if m.recent != nil {
		if err := m.recent.Record(path); err != nil && m.diag != nil {
			m.diag.Warn("recent: " + err.Error())
		}
	}
	m.refreshContent(path)
}

// navigateTo opens path and moves the tree cursor to its row. Used
// anywhere a file is opened by user action other than Back/Forward
// (those have their own path because they don't push history).
func (m *Model) navigateTo(path string) {
	m.openFile(path)
	m.selectInTree(path)
}

// refreshContent re-renders the file at path into the viewport without
// touching history. Used by back/forward and on resize. Also refreshes
// the link list and clears any active link selection.
func (m *Model) refreshContent(path string) {
	// Single-shot pre-select: clear the field unconditionally before any
	// early return, so a read or render failure here can't leak a stale
	// target into the next refreshContent.
	target := m.pendingPreselectTarget
	m.pendingPreselectTarget = ""

	src, err := os.ReadFile(path)
	if err != nil {
		m.status = err.Error()
		m.content.viewport.SetContent(fmt.Sprintf("Error: %v", err))
		m.content.links = nil
		m.content.linkCursor = -1
		return
	}
	m.content.renderer.SetFromFile(path)
	out, links, err := m.content.renderer.RenderWithLinks(string(src), path, linkZoneMarker)
	if err != nil {
		m.status = err.Error()
		m.content.viewport.SetContent(fmt.Sprintf("Error: %v", err))
		m.content.links = nil
		m.content.linkCursor = -1
		return
	}
	m.status = path
	m.content.viewport.SetContent(out)
	m.content.viewport.GotoTop()
	m.content.links = links

	m.content.linkCursor = -1
	if target != "" {
		for i, l := range links {
			if l.Resolved.Kind == markdown.LinkLocalFile && l.Resolved.Target == target {
				m.content.linkCursor = i
				break
			}
		}
	}
	if m.content.linkCursor >= 0 {
		m.scrollToLink(m.content.links[m.content.linkCursor])
		m.applyLinkHighlight()
	}

	m.refreshBacklinks(path)
}

// handleFSEvent reacts to a debounced filesystem event. Structure changes
// trigger a tree re-walk; file writes trigger a content refresh only if
// the changed path is the one currently displayed.
//
// Cursor and viewport scroll position are preserved across both kinds of
// refresh so live edits don't yank the user back to the top.
func (m *Model) handleFSEvent(ev watch.Event) {
	switch ev.Kind {
	case watch.StructureChanged:
		if m.vault != nil {
			if err := m.vault.Rebuild(); err != nil {
				m.diag.Warn("vault rebuild failed: " + err.Error())
			}
		}
		selectedPath := ""
		if m.tree.cursor < len(m.tree.flat) {
			selectedPath = m.tree.flat[m.tree.cursor].node.Path
		}
		newRoot, err := tree.Walk(m.root)
		if err != nil {
			m.status = err.Error()
			return
		}
		m.rootNode = newRoot
		m.tree.flat = m.flattenVisible()
		// Restore cursor by path; if the previously selected node is gone,
		// clamp to a valid index rather than dangling past the end.
		m.tree.cursor = 0
		if i := m.rowIndexByPath(selectedPath); i >= 0 {
			m.tree.cursor = i
		}
		if m.tree.cursor >= len(m.tree.flat) {
			m.tree.cursor = len(m.tree.flat) - 1
		}
		if m.tree.cursor < 0 {
			m.tree.cursor = 0
		}
		m.refreshTreeVP()

	case watch.FileModified:
		if m.vault != nil {
			for _, p := range ev.Paths {
				if err := m.vault.RefreshFile(p); err != nil {
					m.diag.Warn("vault refresh failed: " + err.Error())
				}
			}
		}
		cur := m.history.Current()
		if cur == "" {
			return
		}
		for _, p := range ev.Paths {
			if p == cur {
				offset := m.content.viewport.YOffset
				m.refreshContent(cur)
				// refreshContent calls GotoTop; restore scroll so a save
				// in your editor doesn't jump the reader to the start.
				m.content.viewport.SetYOffset(offset)
				return
			}
		}
	}
}

// scrollToLine positions line n of the rendered output about 25% from
// the top of the viewport. n is 1-indexed and matches what
// vault.Backlink.Line carries (a source-file line number).
//
// Caveat: source-file line numbers don't perfectly correspond to
// rendered-output line numbers (Glamour adjusts for headings, code
// fences, etc.). The user lands "near" the reference, not exactly on
// it; the snippet shown in the backlinks pane gives them a visual
// landmark to confirm.
func (m *Model) scrollToLine(n int) {
	if n < 1 {
		n = 1
	}
	total := m.content.viewport.TotalLineCount()
	if n > total {
		n = total
	}
	// Position the target line ~25% from the top of the viewport so the
	// user sees the lines preceding the reference for context.
	pad := m.content.viewport.Height / 4
	target := n - 1 - pad
	if target < 0 {
		target = 0
	}
	maxOffset := total - m.content.viewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if target > maxOffset {
		target = maxOffset
	}
	m.content.viewport.SetYOffset(target)
}

// allVaultMarkdownPaths walks m.rootNode and returns every markdown file
// as an absolute path. Tree was already pruned to markdown-only by tree.Walk.
func (m *Model) allVaultMarkdownPaths() []string {
	if m.rootNode == nil {
		return nil
	}
	var out []string
	var walk func(n *tree.Node)
	walk = func(n *tree.Node) {
		if !n.IsDir {
			out = append(out, n.Path)
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(m.rootNode)
	return out
}

package tui

import (
	"fmt"
	"os"
	"path/filepath"

	zone "github.com/lrstanley/bubblezone"

	"github.com/wilkes/hypogeum/internal/tree"
	"github.com/wilkes/hypogeum/internal/watch"
)

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
	m.refreshContent(path)
}

// navigateTo opens path and moves the tree cursor to its row. Used
// anywhere a file is opened by user action other than Back/Forward
// (those have their own path because they don't push history).
func (m *Model) navigateTo(path string) {
	m.openFile(path)
	m.selectInTree(path)
}

// normalizeFocus repairs focus when it points at a pane that isn't
// rendered. Called from anywhere that might hide the tree (resize,
// ^b toggle, startup on a narrow terminal). When the tree disappears
// from under focusTree, keystrokes would route to an invisible pane;
// snapping to focusContent keeps them effective.
func (m *Model) normalizeFocus() {
	if m.focus == focusTree && !m.shouldShowTree() {
		m.focus = focusContent
	}
}

// refreshContent re-renders the file at path into the viewport without
// touching history. Used by back/forward and on resize. Also refreshes
// the link list and clears any active link selection.
func (m *Model) refreshContent(path string) {
	src, err := os.ReadFile(path)
	if err != nil {
		m.status = err.Error()
		m.viewport.SetContent(fmt.Sprintf("Error: %v", err))
		m.links = nil
		m.linkCursor = -1
		return
	}
	m.renderer.SetFromFile(path)
	out, links, err := m.renderer.RenderWithLinks(string(src), path, linkZoneMarker)
	if err != nil {
		m.status = err.Error()
		m.viewport.SetContent(fmt.Sprintf("Error: %v", err))
		m.links = nil
		m.linkCursor = -1
		return
	}
	m.status = path
	m.viewport.SetContent(out)
	m.viewport.GotoTop()
	m.links = links
	m.linkCursor = -1
	m.refreshBacklinks(path)
}

// selectInTree moves the tree cursor to the row matching path, expanding
// any collapsed ancestors first so a history navigation into a collapsed
// subtree lands on a visible row.
func (m *Model) selectInTree(path string) {
	if m.expandAncestors(path) {
		m.tree.flat = m.flattenVisible()
	}
	if i := m.rowIndexByPath(path); i >= 0 {
		m.tree.cursor = i
	}
	m.refreshTreeVP()
}

// rowIndexByPath returns the index of the visible tree row whose node
// path equals path, or -1 if no such row is visible.
func (m *Model) rowIndexByPath(path string) int {
	for i, row := range m.tree.flat {
		if row.node.Path == path {
			return i
		}
	}
	return -1
}

// cursorRow returns the row under the tree cursor, or zero-value+false
// when the flat tree is empty or the cursor is out of bounds.
func (m *Model) cursorRow() (treeRow, bool) {
	if m.tree.cursor < 0 || m.tree.cursor >= len(m.tree.flat) {
		return treeRow{}, false
	}
	return m.tree.flat[m.tree.cursor], true
}

// toggleFolder flips the collapsed state of the directory at path,
// rebuilds the flat tree, and keeps the cursor on that directory's row.
func (m *Model) toggleFolder(path string) {
	if m.isCollapsed(path) {
		delete(m.tree.expanded, path) // back to default-expanded
	} else {
		m.tree.expanded[path] = false
	}
	m.tree.flat = m.flattenVisible()
	if i := m.rowIndexByPath(path); i >= 0 {
		m.tree.cursor = i
	}
	m.refreshTreeVP()
}

// expandAncestors removes "collapsed" overrides on every directory that
// contains path, returning true if anything changed.
func (m *Model) expandAncestors(path string) bool {
	changed := false
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if v, ok := m.tree.expanded[dir]; ok && !v {
			delete(m.tree.expanded, dir)
			changed = true
		}
		// Stop at the configured root, or at the filesystem root where
		// filepath.Dir is its own fixed point ("/" on unix, "C:\" on win).
		if dir == m.root || dir == filepath.Dir(dir) {
			return changed
		}
	}
}

// firstTopLevelFile returns the first non-directory child of root, or nil if
// every top-level entry is a directory. Used to pick the landing file so that
// users see something at the top of the tree rather than the deepest leaf.
func firstTopLevelFile(root *tree.Node) *tree.Node {
	if root == nil {
		return nil
	}
	for _, c := range root.Children {
		if !c.IsDir {
			return c
		}
	}
	return nil
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
				offset := m.viewport.YOffset
				m.refreshContent(cur)
				// refreshContent calls GotoTop; restore scroll so a save
				// in your editor doesn't jump the reader to the start.
				m.viewport.SetYOffset(offset)
				return
			}
		}
	}
}

// flattenVisible produces a depth-tagged linear list of rows, skipping
// children of collapsed directories. The root is always included.
func (m *Model) flattenVisible() []treeRow {
	if m.rootNode == nil {
		return nil
	}
	var rows []treeRow
	var walk func(n *tree.Node, depth int)
	walk = func(n *tree.Node, depth int) {
		rows = append(rows, treeRow{node: n, depth: depth})
		if n.IsDir && !m.isCollapsed(n.Path) {
			for _, c := range n.Children {
				walk(c, depth+1)
			}
		}
	}
	walk(m.rootNode, 0)
	return rows
}

// isCollapsed reports whether the directory at path is currently collapsed.
func (m *Model) isCollapsed(path string) bool {
	v, ok := m.tree.expanded[path]
	return ok && !v
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
	total := m.viewport.TotalLineCount()
	if n > total {
		n = total
	}
	// Position the target line ~25% from the top of the viewport so the
	// user sees the lines preceding the reference for context.
	pad := m.viewport.Height / 4
	target := n - 1 - pad
	if target < 0 {
		target = 0
	}
	maxOffset := total - m.viewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if target > maxOffset {
		target = maxOffset
	}
	m.viewport.SetYOffset(target)
}

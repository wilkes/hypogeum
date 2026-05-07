package tui

import (
	"fmt"
	"os"

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
}

// selectInTree moves the tree cursor to the row matching path, if present.
func (m *Model) selectInTree(path string) {
	for i, row := range m.flatTree {
		if row.node.Path == path {
			m.treeCursor = i
			return
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
		selectedPath := ""
		if m.treeCursor < len(m.flatTree) {
			selectedPath = m.flatTree[m.treeCursor].node.Path
		}
		newRoot, err := tree.Walk(m.root)
		if err != nil {
			m.status = err.Error()
			return
		}
		m.rootNode = newRoot
		m.flatTree = flatten(newRoot, 0)
		// Restore cursor by path; if the previously selected node is gone,
		// clamp to a valid index rather than dangling past the end.
		m.treeCursor = 0
		if selectedPath != "" {
			for i, row := range m.flatTree {
				if row.node.Path == selectedPath {
					m.treeCursor = i
					break
				}
			}
		}
		if m.treeCursor >= len(m.flatTree) {
			m.treeCursor = len(m.flatTree) - 1
		}
		if m.treeCursor < 0 {
			m.treeCursor = 0
		}

	case watch.FileModified:
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

// flatten produces a depth-tagged linear list from a tree, used for
// keyboard navigation. The root itself is included.
func flatten(n *tree.Node, depth int) []treeRow {
	if n == nil {
		return nil
	}
	rows := []treeRow{{node: n, depth: depth}}
	for _, c := range n.Children {
		rows = append(rows, flatten(c, depth+1)...)
	}
	return rows
}

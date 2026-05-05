package tui

import (
	"fmt"
	"os"

	"github.com/wilkes/hypogeum/internal/tree"
)

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
	out, links, err := m.renderer.RenderWithLinks(string(src), path)
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

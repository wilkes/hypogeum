package tui

import (
	"github.com/wilkes/hypogeum/internal/tree"
)

// selectInTree moves the tree cursor to the row matching path, collapsing
// every directory not on path's ancestor chain. The map is derived from
// the current file, not user state: navigating to a new file resets which
// directories are open, so the tree always shows a focused view of where
// the user is in the vault. Manual Space-toggles persist only until the
// next navigation.
func (m *Model) selectInTree(path string) {
	m.expandAncestorsOf(path)
	m.tree.flat = m.flattenVisible()
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

// toggleFolder flips the expanded state of the directory at path,
// rebuilds the flat tree, and keeps the cursor on that directory's row.
// Manual expansions persist only until the next navigation: openFile and
// selectInTree both call expandAncestorsOf, which clears the map and
// re-derives it from the new file's ancestor chain.
func (m *Model) toggleFolder(path string) {
	if m.tree.expanded[path] {
		delete(m.tree.expanded, path)
	} else {
		m.tree.expanded[path] = true
	}
	m.tree.flat = m.flattenVisible()
	if i := m.rowIndexByPath(path); i >= 0 {
		m.tree.cursor = i
	}
	m.refreshTreeVP()
}

// expandAncestorsOf collapses everything and then expands only the
// directories on path's ancestor chain. The result is a tree showing the
// path from root down to path's parent, with all sibling branches closed.
//
// It walks the actual node tree rather than doing filepath.Dir string math
// so it works for both single-root trees (absolute directory keys) and
// merged overlay trees (relative directory keys), where a file may live
// under a different real root than the one that first named its merged
// parent directory.
func (m *Model) expandAncestorsOf(path string) {
	for k := range m.tree.expanded {
		delete(m.tree.expanded, k)
	}
	if path == "" || m.rootNode == nil {
		return
	}
	chain, ok := nodeChain(m.rootNode, path)
	if !ok {
		return
	}
	for _, d := range chain {
		m.tree.expanded[d.Path] = true
	}
}

// nodeChain returns the directory nodes from n down to (but excluding) the
// node whose Path == target, or false if no such node exists in the subtree.
// For a file target the returned slice is exactly its directory ancestors.
func nodeChain(n *tree.Node, target string) ([]*tree.Node, bool) {
	if n == nil {
		return nil, false
	}
	if n.Path == target {
		return nil, true
	}
	if !n.IsDir {
		return nil, false
	}
	for _, c := range n.Children {
		if sub, ok := nodeChain(c, target); ok {
			return append([]*tree.Node{n}, sub...), true
		}
	}
	return nil, false
}

// flattenVisible produces a depth-tagged linear list of rows, skipping
// children of collapsed directories. The root is always treated as
// expanded so its top-level entries are visible even before any
// navigation has expanded an ancestor chain.
func (m *Model) flattenVisible() []treeRow {
	if m.rootNode == nil {
		return nil
	}
	var rows []treeRow
	var walk func(n *tree.Node, depth int)
	walk = func(n *tree.Node, depth int) {
		rows = append(rows, treeRow{node: n, depth: depth})
		if n.IsDir && m.isExpanded(n) {
			for _, c := range n.Children {
				walk(c, depth+1)
			}
		}
	}
	walk(m.rootNode, 0)
	return rows
}

// isExpanded reports whether the directory node is currently expanded.
// The root is always expanded so the tree's top level is visible without
// requiring any prior navigation to seed the map. Every other directory
// is expanded only if explicitly recorded in m.tree.expanded — either by
// expandAncestorsOf (current file's chain) or by user toggleFolder.
func (m *Model) isExpanded(n *tree.Node) bool {
	if n == m.rootNode {
		return true
	}
	return m.tree.expanded[n.Path]
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

package tui

import (
	"path/filepath"
	"strings"

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
func (m *Model) expandAncestorsOf(path string) {
	for k := range m.tree.expanded {
		delete(m.tree.expanded, k)
	}
	if path == "" {
		return
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		m.tree.expanded[dir] = true
		// Stop at the configured root, or at the filesystem root where
		// filepath.Dir is its own fixed point ("/" on unix, "C:\" on win).
		if dir == m.root || dir == filepath.Dir(dir) {
			return
		}
	}
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
	if n.Path == m.root {
		return true
	}
	return m.tree.expanded[n.Path]
}

// firstTopLevelFile picks the landing file from root's direct children,
// preferring a conventional overview file over alphabetical order. It scans
// the top level only (never descending into subdirectories) in three passes:
//
//  1. the first child whose basename stem equals "index" (case-insensitive),
//  2. else the first whose stem equals "readme" (case-insensitive),
//  3. else the first non-directory child (the historical fallback).
//
// Returns nil if every top-level entry is a directory. The tree walker has
// already filtered to markdown files, so this matches on the stem alone and
// does not filter by extension. Among multiple matches the existing
// (alphabetical) child order decides — no special tie-breaking.
func firstTopLevelFile(root *tree.Node) *tree.Node {
	if root == nil {
		return nil
	}
	if n := firstTopLevelStem(root, "index"); n != nil {
		return n
	}
	if n := firstTopLevelStem(root, "readme"); n != nil {
		return n
	}
	for _, c := range root.Children {
		if !c.IsDir {
			return c
		}
	}
	return nil
}

// firstTopLevelStem returns the first non-directory child of root whose
// basename stem (filename minus extension) case-insensitively equals stem,
// or nil if there is none.
func firstTopLevelStem(root *tree.Node, stem string) *tree.Node {
	for _, c := range root.Children {
		if c.IsDir {
			continue
		}
		if strings.ToLower(strings.TrimSuffix(c.Name, filepath.Ext(c.Name))) == stem {
			return c
		}
	}
	return nil
}

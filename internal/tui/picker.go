package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/wilkes/hypogeum/internal/tree"
)

// pickerState is a vault-rooted file picker rendered as a modal. It
// reuses m.rootNode (the same pruned *tree.Node the left pane displays),
// so directories without markdown descendants don't appear and files
// outside MarkdownExts don't appear — that filtering happens at
// tree.Walk time, not here.
//
// The picker holds its own cursor and expansion state independent from
// the left pane, so collapsing a folder in the picker doesn't affect
// the left pane (or vice versa). Expansion state is reset on each open.
type pickerState struct {
	cursor   int
	expanded map[string]bool // path → true if expanded; missing = collapsed (opposite of m.tree.expanded)
	flat     []treeRow
	vp       viewport.Model
}

func newPicker() pickerState {
	return pickerState{
		expanded: map[string]bool{},
		vp:       viewport.New(0, 0),
	}
}

// reset prepares the picker for a fresh open: cursor at top, all
// directories collapsed, flat list rebuilt from root. The default-
// collapsed policy is opposite to the left pane (which defaults
// expanded) — picker users want to descend selectively, not stare at a
// fully expanded vault.
func (p *pickerState) reset(root *tree.Node) {
	p.cursor = 0
	p.expanded = map[string]bool{}
	p.flat = pickerFlatten(root, p.expanded)
	p.refreshVP()
}

// pickerFlatten produces the picker's visible row list. Directories
// expand only if their path is in expanded[]. The root itself is always
// included as the first row (depth 0); the user can collapse the root
// to a single-row picker, but that's harmless.
func pickerFlatten(root *tree.Node, expanded map[string]bool) []treeRow {
	if root == nil {
		return nil
	}
	var rows []treeRow
	var walk func(n *tree.Node, depth int)
	walk = func(n *tree.Node, depth int) {
		rows = append(rows, treeRow{node: n, depth: depth})
		if n.IsDir && expanded[n.Path] {
			for _, c := range n.Children {
				walk(c, depth+1)
			}
		}
	}
	walk(root, 0)
	return rows
}

// toggleAt flips the expansion of the directory at the cursor and
// rebuilds the flat list. The cursor stays on that directory's row by
// path lookup. No-op if the cursor is on a file.
func (p *pickerState) toggleAt(root *tree.Node) {
	if p.cursor < 0 || p.cursor >= len(p.flat) {
		return
	}
	row := p.flat[p.cursor]
	if !row.node.IsDir {
		return
	}
	if p.expanded[row.node.Path] {
		delete(p.expanded, row.node.Path)
	} else {
		p.expanded[row.node.Path] = true
	}
	p.flat = pickerFlatten(root, p.expanded)
	for i, r := range p.flat {
		if r.node.Path == row.node.Path {
			p.cursor = i
			break
		}
	}
	p.refreshVP()
}

// refreshVP regenerates the viewport content and scrolls so the cursor
// row is in view. Mirrors Model.refreshTreeVP for the left pane.
func (p *pickerState) refreshVP() {
	p.vp.SetContent(p.renderRows())
	if p.cursor < p.vp.YOffset {
		p.vp.SetYOffset(p.cursor)
	} else if last := p.vp.YOffset + p.vp.Height - 1; p.cursor > last {
		p.vp.SetYOffset(p.cursor - p.vp.Height + 1)
	}
}

// renderRows builds the picker's display string: chevron-prefixed
// directory rows and indented file rows, with a `>` cursor marker on
// the active row.
func (p *pickerState) renderRows() string {
	var b strings.Builder
	for i, row := range p.flat {
		indent := strings.Repeat("  ", row.depth)
		marker := " "
		if i == p.cursor {
			marker = ">"
		}
		name := row.node.Name
		if row.node.IsDir {
			chevron := "▸ "
			if p.expanded[row.node.Path] {
				chevron = "▾ "
			}
			name = chevron + name + "/"
		} else {
			name = "  " + name
		}
		fmt.Fprintf(&b, "%s%s %s\n", marker, indent, name)
	}
	return b.String()
}

// View returns the picker's renderable string for placement inside the
// modal frame. The caller has already sized the viewport via
// resizePicker on the most recent WindowSizeMsg.
func (p *pickerState) View() string {
	if len(p.flat) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("(no markdown files in vault)")
	}
	return p.vp.View()
}

// resizePicker fits the picker viewport into the modal interior.
// modalGeometry returns the modal's outer dimensions; subtract its
// border (2) to get the inside.
func (m *Model) resizePicker() {
	_, _, w, h := modalGeometry(m.width, m.height)
	pw := w - 2
	ph := h - 2
	if pw < 1 {
		pw = 1
	}
	if ph < 1 {
		ph = 1
	}
	m.modals.picker.vp.Width = pw
	m.modals.picker.vp.Height = ph
	m.modals.picker.refreshVP()
}

// pickerSelectedFile returns the file path under the picker cursor, or
// ("", false) if the cursor is on a directory or out of range.
func (p *pickerState) selectedFile() (string, bool) {
	if p.cursor < 0 || p.cursor >= len(p.flat) {
		return "", false
	}
	row := p.flat[p.cursor]
	if row.node.IsDir {
		return "", false
	}
	return row.node.Path, true
}


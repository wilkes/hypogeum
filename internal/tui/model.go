// Package tui contains the Bubble Tea Model that wires the directory tree,
// the markdown viewport, and the navigation history together.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/wilkes/hypogeum/internal/markdown"
	"github.com/wilkes/hypogeum/internal/nav"
	"github.com/wilkes/hypogeum/internal/tree"
)

// Focus indicates which pane currently receives keyboard input for movement.
type focus int

const (
	focusTree focus = iota
	focusContent
)

// keyMap collects every keybinding the model knows about. Centralizing them
// makes the help footer trivial to render and the bindings easy to change.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Open     key.Binding
	Back     key.Binding
	Forward  key.Binding
	FocusTog key.Binding
	Quit     key.Binding

	NextLink  key.Binding
	PrevLink  key.Binding
	ClearLink key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Open:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:     key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "back")),
		Forward:  key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "forward")),
		FocusTog: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch pane")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

		NextLink:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next link")),
		PrevLink:  key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev link")),
		ClearLink: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear link")),
	}
}

// Model is the top-level Bubble Tea model.
type Model struct {
	root       string
	rootNode   *tree.Node
	flatTree   []treeRow // pre-flattened for keyboard navigation
	treeCursor int

	viewport viewport.Model
	renderer *markdown.Renderer

	history *nav.History
	focus   focus

	links       []markdown.Link // links extracted from the currently rendered file
	linkCursor  int             // -1 when no link is selected (Phase 1: always -1)

	width, height int
	keys          keyMap
	status        string // last error or info message
}

// linkFooterMarker is rendered into the footer when a link is selected.
// Defined as a constant so tests can assert on its presence/absence.
const linkFooterMarker = "→ "

// treeRow is a flattened tree row used for cursor-driven navigation. Tracking
// depth here avoids re-walking the tree on every keystroke.
type treeRow struct {
	node  *tree.Node
	depth int
}

// New constructs a Model rooted at root. If initialFile is non-empty, that
// file is opened on startup.
func New(root, initialFile string) (Model, error) {
	rootNode, err := tree.Walk(root)
	if err != nil {
		return Model{}, fmt.Errorf("walk %s: %w", root, err)
	}

	r, err := markdown.NewRenderer(80)
	if err != nil {
		return Model{}, err
	}

	m := Model{
		root:       root,
		rootNode:   rootNode,
		viewport:   viewport.New(0, 0),
		renderer:   r,
		history:    nav.New(),
		focus:      focusTree,
		keys:       defaultKeys(),
		linkCursor: -1,
	}
	m.flatTree = flatten(rootNode, 0)

	if initialFile != "" {
		m.openFile(initialFile)
		m.selectInTree(initialFile)
	} else if first := firstTopLevelFile(rootNode); first != nil {
		m.openFile(first.Path)
		m.selectInTree(first.Path)
	}

	return m, nil
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		treeWidth := m.treeWidth()
		contentWidth := m.width - treeWidth - 2 // borders / padding
		if contentWidth < 20 {
			contentWidth = 20
		}
		m.viewport.Width = contentWidth
		m.viewport.Height = m.height - 2 // header + footer
		// Re-render with the new wrap width.
		if r, err := markdown.NewRenderer(contentWidth); err == nil {
			m.renderer = r
		}
		if cur := m.history.Current(); cur != "" {
			m.refreshContent(cur)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward other messages (mouse, etc.) to the viewport when content has focus.
	if m.focus == focusContent {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.FocusTog):
		if m.focus == focusTree {
			m.focus = focusContent
		} else {
			m.focus = focusTree
		}
		return m, nil

	case key.Matches(msg, m.keys.Back):
		if path, ok := m.history.Back(); ok {
			m.refreshContent(path)
			m.selectInTree(path)
		}
		return m, nil

	case key.Matches(msg, m.keys.Forward):
		if path, ok := m.history.Forward(); ok {
			m.refreshContent(path)
			m.selectInTree(path)
		}
		return m, nil
	}

	if m.focus == focusTree {
		return m.handleTreeKey(msg)
	}
	return m.handleContentKey(msg)
}

// handleContentKey routes keystrokes received while the content pane has
// focus. Link-cycling bindings (n/p/Esc) and Enter (when a link is
// selected) are intercepted; everything else falls through to the
// viewport so its scrolling bindings keep working.
func (m Model) handleContentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.NextLink):
		m.cycleLink(+1)
		return m, nil
	case key.Matches(msg, m.keys.PrevLink):
		m.cycleLink(-1)
		return m, nil
	case key.Matches(msg, m.keys.ClearLink):
		m.linkCursor = -1
		return m, nil
	case key.Matches(msg, m.keys.Open):
		if sel := m.selectedLink(); sel != nil {
			m.followLink(*sel)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// cycleLink moves the link cursor by step, wrapping at both ends. From
// the unselected state (-1), +1 selects the first link and -1 selects
// the last. No-op when there are no links on the page.
func (m *Model) cycleLink(step int) {
	if len(m.links) == 0 {
		return
	}
	switch {
	case m.linkCursor < 0 && step > 0:
		m.linkCursor = 0
	case m.linkCursor < 0 && step < 0:
		m.linkCursor = len(m.links) - 1
	default:
		m.linkCursor = (m.linkCursor + step + len(m.links)) % len(m.links)
	}
	m.scrollToLink(m.links[m.linkCursor])
}

// followLink performs whatever navigation a link's kind warrants.
// Phase 1: local files navigate (recording history); external URLs
// surface the URL in the status bar; anchors are no-ops with a status
// message.
func (m *Model) followLink(l markdown.Link) {
	switch l.Resolved.Kind {
	case markdown.LinkLocalFile:
		m.openFile(l.Resolved.Target)
		m.selectInTree(l.Resolved.Target)
	case markdown.LinkExternal:
		m.status = "external link not opened: " + l.Href
	case markdown.LinkAnchor:
		m.status = "anchor navigation not implemented: #" + l.Resolved.Anchor
	default:
		m.status = "unrecognized link: " + l.Href
	}
}

// scrollToLink ensures the link's row is visible in the viewport. Pads
// by one line above so the link isn't flush with the top edge.
func (m *Model) scrollToLink(l markdown.Link) {
	top := m.viewport.YOffset
	bottom := top + m.viewport.Height - 1
	switch {
	case l.Row < top:
		m.viewport.SetYOffset(max(0, l.Row-1))
	case l.Row > bottom:
		m.viewport.SetYOffset(l.Row - m.viewport.Height + 2)
	}
}

func (m Model) handleTreeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.treeCursor > 0 {
			m.treeCursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.treeCursor < len(m.flatTree)-1 {
			m.treeCursor++
		}
	case key.Matches(msg, m.keys.Open):
		if m.treeCursor < len(m.flatTree) {
			row := m.flatTree[m.treeCursor]
			if !row.node.IsDir {
				m.openFile(row.node.Path)
			}
		}
	}
	return m, nil
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

func (m Model) View() string {
	if m.width == 0 {
		return "" // wait for first WindowSizeMsg
	}

	tree := m.renderTree()
	content := m.viewport.View()

	treeStyled := paneStyle(m.focus == focusTree).
		Width(m.treeWidth()).
		Height(m.height - 2).
		Render(tree)
	contentStyled := paneStyle(m.focus == focusContent).
		Width(m.viewport.Width).
		Height(m.height - 2).
		Render(content)

	body := lipgloss.JoinHorizontal(lipgloss.Top, treeStyled, contentStyled)
	footer := m.renderFooter()
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}

func (m Model) renderTree() string {
	var b strings.Builder
	for i, row := range m.flatTree {
		indent := strings.Repeat("  ", row.depth)
		marker := " "
		if i == m.treeCursor {
			marker = ">"
		}
		name := row.node.Name
		if row.node.IsDir {
			name = name + "/"
		}
		fmt.Fprintf(&b, "%s%s %s\n", marker, indent, name)
	}
	return b.String()
}

func (m Model) renderFooter() string {
	keys := []string{
		"tab: switch", "↑↓/jk: move", "enter: open",
		"n/p: link", "esc: clear",
		"h/←: back", "l/→: forward", "q: quit",
	}
	help := strings.Join(keys, "  ")
	loc := m.status
	if loc != "" {
		// Show path relative to root for brevity.
		if rel, err := filepath.Rel(m.root, loc); err == nil {
			loc = rel
		}
	}
	if sel := m.selectedLink(); sel != nil {
		loc = fmt.Sprintf("%s%s [%d/%d] %s", linkFooterMarker, loc, m.linkCursor+1, len(m.links), linkLabel(*sel, m.root))
	}
	footerStyle := lipgloss.NewStyle().Faint(true)
	return footerStyle.Render(fmt.Sprintf("%s\n%s", loc, help))
}

// selectedLink returns a pointer to the currently selected link, or nil
// if no link is selected.
func (m Model) selectedLink() *markdown.Link {
	if m.linkCursor < 0 || m.linkCursor >= len(m.links) {
		return nil
	}
	return &m.links[m.linkCursor]
}

// linkLabel formats a link's target for footer display: relative path
// for local files (against the tree root for brevity), raw href otherwise.
func linkLabel(l markdown.Link, root string) string {
	switch l.Resolved.Kind {
	case markdown.LinkLocalFile:
		if rel, err := filepath.Rel(root, l.Resolved.Target); err == nil {
			return rel
		}
		return l.Resolved.Target
	case markdown.LinkAnchor:
		return "#" + l.Resolved.Anchor
	default:
		return l.Href
	}
}

func (m Model) treeWidth() int {
	w := m.width / 4
	if w < 20 {
		w = 20
	}
	if w > 40 {
		w = 40
	}
	return w
}

func paneStyle(focused bool) lipgloss.Style {
	border := lipgloss.NormalBorder()
	color := lipgloss.Color("240")
	if focused {
		color = lipgloss.Color("62")
	}
	return lipgloss.NewStyle().Border(border).BorderForeground(color)
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

// Package tui contains the Bubble Tea Model that wires the directory tree,
// the markdown viewport, and the navigation history together.
package tui

import (
	"fmt"
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

	width, height int
	keys          keyMap
	status        string // last error or info message
}

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
		root:     root,
		rootNode: rootNode,
		viewport: viewport.New(0, 0),
		renderer: r,
		history:  nav.New(),
		focus:    focusTree,
		keys:     defaultKeys(),
	}
	m.flatTree = flatten(rootNode, 0)

	if initialFile != "" {
		m.openFile(initialFile)
		m.selectInTree(initialFile)
	} else if len(m.flatTree) > 0 {
		// Find the first file (skip the root directory header).
		for i, row := range m.flatTree {
			if !row.node.IsDir {
				m.treeCursor = i
				m.openFile(row.node.Path)
				break
			}
		}
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

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
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
// touching history. Used by back/forward and on resize.
func (m *Model) refreshContent(path string) {
	out, err := m.renderer.RenderFile(path)
	if err != nil {
		m.status = err.Error()
		m.viewport.SetContent(fmt.Sprintf("Error: %v", err))
		return
	}
	m.status = path
	m.viewport.SetContent(out)
	m.viewport.GotoTop()
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
	footerStyle := lipgloss.NewStyle().Faint(true)
	return footerStyle.Render(fmt.Sprintf("%s\n%s", loc, help))
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

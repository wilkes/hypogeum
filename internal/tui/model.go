// Package tui contains the Bubble Tea Model that wires the directory tree,
// the markdown viewport, and the navigation history together.
package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/wilkes/hypogeum/internal/markdown"
	"github.com/wilkes/hypogeum/internal/nav"
	"github.com/wilkes/hypogeum/internal/tree"
	"github.com/wilkes/hypogeum/internal/watch"
)

// fsEventMsg carries a debounced filesystem event from the watcher into the
// Bubble Tea update loop. It is the only TUI-side reference to internal/watch
// so that tests can synthesize one without spinning up a real watcher.
type fsEventMsg watch.Event

// Focus indicates which pane currently receives keyboard input for movement.
type focus int

const (
	focusTree focus = iota
	focusContent
)

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

	links      []markdown.Link // links extracted from the currently rendered file
	linkCursor int             // -1 when no link is selected (Phase 1: always -1)

	width, height int
	keys          keyMap
	status        string // last error or info message

	// watcher observes the tree for live updates. nil if construction
	// failed (we degrade gracefully — the browser still works without it).
	watcher *watch.Watcher
}

// linkFooterMarker is rendered into the footer when a link is selected.
// Defined as a constant so tests can assert on its presence/absence.
const linkFooterMarker = "→ "

// maxRenderWidth caps Glamour's word-wrap width. The viewport pane can
// be wider than this (uses whatever space is left after the tree); this
// only affects where lines break. Keeps prose at a comfortable reading
// width even on ultra-wide terminals.
const maxRenderWidth = 80

// treeRow is a flattened tree row used for cursor-driven navigation. Tracking
// depth here avoids re-walking the tree on every keystroke.
type treeRow struct {
	node  *tree.Node
	depth int
}

// Pane and content zone IDs used by the View to mark hit-test regions.
// BubbleZone resolves these to bounding boxes during Scan, so Update can
// route mouse events without computing pane geometry by hand.
const (
	zoneTreePane    = "pane:tree"
	zoneContentPane = "pane:content"
)

// treeRowZoneID returns the BubbleZone id for the i-th visible tree row.
// One zone per row keeps clicks unambiguous even when the tree pane gets
// resized or scrolled in future versions.
func treeRowZoneID(i int) string {
	return fmt.Sprintf("tree:%d", i)
}

// New constructs a Model rooted at root. If initialFile is non-empty, that
// file is opened on startup.
func New(root, initialFile string) (Model, error) {
	// Initialize the global zone manager. Idempotent — calling NewGlobal
	// on a manager that's already running is a no-op, so it's safe in
	// tests that construct multiple models in one process.
	zone.NewGlobal()

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

	// A watcher is best-effort: if it fails (e.g. inotify limits hit on
	// Linux), we silently fall back to the previous reload-on-navigate
	// behavior rather than refusing to start.
	if w, err := watch.New(root); err == nil {
		m.watcher = w
	}

	if initialFile != "" {
		m.openFile(initialFile)
		m.selectInTree(initialFile)
	} else if first := firstTopLevelFile(rootNode); first != nil {
		m.openFile(first.Path)
		m.selectInTree(first.Path)
	}

	return m, nil
}

func (m Model) Init() tea.Cmd { return m.waitForFSEvent() }

// waitForFSEvent returns a tea.Cmd that blocks until the watcher emits an
// event, then surfaces it as fsEventMsg. The Update path re-issues this
// command so the loop keeps listening. Returns nil if there is no watcher
// (which also means no rescheduled command — the channel select below
// stays quiet for the rest of the session).
func (m Model) waitForFSEvent() tea.Cmd {
	if m.watcher == nil {
		return nil
	}
	ch := m.watcher.Events()
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return fsEventMsg(ev)
	}
}

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
		// Leave room for the pane's top+bottom borders (2) and the
		// two-line footer (2) so View() fits within m.height.
		m.viewport.Height = m.height - 4
		// Cap the renderer's wrap width so prose stays readable on wide
		// terminals; the viewport pane keeps the full available width.
		renderWidth := min(contentWidth, maxRenderWidth)
		if r, err := markdown.NewRenderer(renderWidth); err == nil {
			m.renderer = r
		}
		if cur := m.history.Current(); cur != "" {
			m.refreshContent(cur)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case fsEventMsg:
		m.handleFSEvent(watch.Event(msg))
		return m, m.waitForFSEvent()
	}

	// Forward other messages to the viewport when content has focus.
	if m.focus == focusContent {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

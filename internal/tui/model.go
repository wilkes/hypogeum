// Package tui contains the Bubble Tea Model that wires the directory tree,
// the markdown viewport, and the navigation history together.
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/wilkes/hypogeum/internal/markdown"
	"github.com/wilkes/hypogeum/internal/nav"
	"github.com/wilkes/hypogeum/internal/recent"
	"github.com/wilkes/hypogeum/internal/tree"
	"github.com/wilkes/hypogeum/internal/vault"
	"github.com/wilkes/hypogeum/internal/watch"
)

// fsEventMsg carries a debounced filesystem event from the watcher into the
// Bubble Tea update loop. It is the only TUI-side reference to internal/watch
// so that tests can synthesize one without spinning up a real watcher.
type fsEventMsg watch.Event

// transientClearMsg is delivered ~1s after each tick, asking the model
// to clear the footer transient if it's older than 3s. The handler
// re-issues the tick so the loop keeps going.
type transientClearMsg struct{}

func clearTransientAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return transientClearMsg{} })
}

// Focus indicates which pane currently receives keyboard input for movement.
type focus int

const (
	focusTree focus = iota
	focusContent
	focusBacklinks
)

// Model is the top-level Bubble Tea model.
type Model struct {
	root     string
	rootNode *tree.Node

	tree      treeUIState
	content   contentUIState
	backlinks backlinksUIState
	modals    modalUIState

	history *nav.History
	focus   focus

	width, height int
	keys          keyMap
	status        string // last error or info message

	// watcher observes the tree for live updates. nil if construction
	// failed (we degrade gracefully — the browser still works without it).
	watcher *watch.Watcher

	vault  *vault.Vault
	recent *recent.Store
	diag   *diagnostics

	// pendingPreselectTarget is the absolute path of a file whose inline
	// link should be pre-selected on the next refreshContent. Set by any
	// navigation that has a meaningful "the link you were looking at"
	// notion: backlink-follow, Back, Forward. Cleared by refreshContent
	// after consumption (whether or not a match was found).
	pendingPreselectTarget string
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

	diag := newDiagnostics(diagOpts{LogPath: defaultLogPath()})
	var v *vault.Vault
	if vv, err := vault.Build(root, diag); err == nil {
		v = vv
	} else {
		diag.Warn("vault build failed: " + err.Error())
	}

	stateFile, sferr := recent.DefaultStateFile()
	if sferr != nil {
		diag.Warn("recent: cannot determine state file path: " + sferr.Error())
	}
	rstore, rerr := recent.New(stateFile)
	if rerr != nil {
		diag.Warn("recent: " + rerr.Error())
	}

	var rOpts []markdown.Option
	if v != nil {
		rOpts = append(rOpts, markdown.WithResolver(v))
	}
	r, err := markdown.NewRenderer(80, rOpts...)
	if err != nil {
		return Model{}, err
	}

	m := Model{
		root:     root,
		rootNode: rootNode,
		history:  nav.New(),
		focus:    focusContent,
		keys:     defaultKeys(),
		vault:    v,
		recent:   rstore,
		diag:     diag,
		tree: treeUIState{
			vp:       viewport.New(0, 0),
			visible:  false,
			expanded: map[string]bool{},
		},
		content: contentUIState{
			viewport:   viewport.New(0, 0),
			renderer:   r,
			linkCursor: -1,
		},
	}
	m.tree.flat = m.flattenVisible()
	m.backlinks.vp = viewport.New(0, 0)
	m.modals.vp = newModalViewport()
	m.modals.picker = newPicker()

	// A watcher is best-effort: if it fails (e.g. inotify limits hit on
	// Linux), we silently fall back to the previous reload-on-navigate
	// behavior rather than refusing to start.
	w, werr := watch.New(root)
	if werr == nil {
		m.watcher = w
	} else {
		diag.Warn("filesystem watcher unavailable; live updates disabled: " + werr.Error())
	}

	if initialFile != "" {
		m.navigateTo(initialFile)
	} else if first := firstTopLevelFile(rootNode); first != nil {
		m.navigateTo(first.Path)
	}

	return m, nil
}

func (m Model) Init() tea.Cmd {
	tick := clearTransientAfter(time.Second)
	if cmd := m.waitForFSEvent(); cmd != nil {
		return tea.Batch(cmd, tick)
	}
	return tick
}

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
		m.normalizeFocus()
		treeWidth := m.treeWidth()
		contentWidth := m.width - treeWidth
		if treeWidth > 0 {
			contentWidth -= 2 // pane borders only count when both panes are rendered
		}
		if contentWidth < 20 {
			contentWidth = 20
		}
		m.content.viewport.Width = contentWidth
		// Leave room for the pane's top+bottom borders (2) and the
		// two-line footer (2) so View() fits within m.height.
		m.content.viewport.Height = m.height - 4
		m.backlinks.vp.Width = contentWidth
		m.backlinks.vp.Height = backlinksHeight - 2
		// The tree viewport gets the inside of the tree pane: width
		// minus its border (2), height minus border + footer (4).
		m.tree.vp.Width = treeWidth - 2
		if m.tree.vp.Width < 0 {
			m.tree.vp.Width = 0
		}
		m.tree.vp.Height = m.height - 4 - 2 // pane border (2) on top of viewport's own
		if m.tree.vp.Height < 1 {
			m.tree.vp.Height = 1
		}
		m.refreshTreeVP()
		m.resizeModalVP()
		m.resizePicker()
		// Cap the renderer's wrap width so prose stays readable on wide
		// terminals; the viewport pane keeps the full available width.
		renderWidth := min(contentWidth, maxRenderWidth)
		var rOpts []markdown.Option
		if m.vault != nil {
			rOpts = append(rOpts, markdown.WithResolver(m.vault))
		}
		if r, err := markdown.NewRenderer(renderWidth, rOpts...); err == nil {
			m.content.renderer = r
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

	case transientClearMsg:
		if m.diag != nil {
			if e, ok := m.diag.transientStatus(); ok && time.Since(e.Timestamp) > 3*time.Second {
				m.diag.clearTransient()
			}
		}
		return m, clearTransientAfter(time.Second)
	}

	// Forward other messages to the viewport when content has focus.
	if m.focus == focusContent {
		var cmd tea.Cmd
		m.content.viewport, cmd = m.content.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

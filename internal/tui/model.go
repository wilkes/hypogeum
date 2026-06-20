// Package tui contains the Bubble Tea Model that wires the directory tree,
// the markdown viewport, and the navigation history together.
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/wilkes/hypogeum/internal/code"
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

// Focus indicates which pane currently receives keyboard input.
// Content is the only pane now — the tree is a modal (^b) and backlinks
// are a modal (b). focus is retained as a one-value enum because
// modalUIState.prevFocus saves/restores it across modal lifecycles; the
// type leaves room for future panes without churning the modal code.
type focus int

const (
	focusContent focus = iota
)

// pendingNav holds the in-flight navigation intent: which inline link to
// pre-select at the destination, an optional range to disambiguate, and an
// external URL awaiting a confirm keystroke.
//
//   - preselectTarget is the absolute path of a file whose inline link should
//     be pre-selected on the next refreshContent. Set by any navigation that
//     has a meaningful "the link you were looking at" notion: backlink-follow,
//     Back, Forward. Cleared by refreshContent after consumption (whether or
//     not a match was found).
//   - preselectRange disambiguates when several inline links share the same
//     target (e.g. two #L-range links into the same source file). When set,
//     refreshContent prefers a link whose Resolved.Range matches; otherwise it
//     falls back to the first target match. Set alongside preselectTarget and
//     cleared on the same lifecycle.
//   - externalURL is set when the user presses Enter on an external link. The
//     footer shows a "press Enter again to open <url>" prompt; a second Enter
//     exec's the opener and clears the field, any other keystroke clears it
//     without exec'ing.
type pendingNav struct {
	preselectTarget string
	preselectRange  *markdown.LineRange
	externalURL     string
}

// Model is the top-level Bubble Tea model.
type Model struct {
	root     string
	rootNode *tree.Node

	tree       treeUIState
	content    contentUIState
	backlinks  backlinksUIState
	recentList recentUIState
	modals     modalUIState

	history *nav.History
	focus   focus

	width, height int
	keys          keyMap

	// currentPath is the absolute path of the file or view currently
	// displayed in the content pane. The footer shows it (relative to
	// root) unless a footerMessage is overriding it.
	currentPath string
	// footerMessage is the last error or transient info message shown in
	// the footer (e.g. a render error, "press Enter again to open …",
	// "opened: …"). When non-empty it takes the footer's location slot in
	// place of currentPath. Cleared on a successful render and on confirm
	// cancellation.
	footerMessage string

	// watcher observes the tree for live updates. nil if construction
	// failed (we degrade gracefully — the browser still works without it).
	watcher *watch.Watcher

	vault  *vault.Vault
	recent *recent.Store
	diag   *diagnostics

	// pending holds the in-flight navigation intent: which inline link to
	// pre-select at the destination, an optional range to disambiguate,
	// and an external URL awaiting a confirm keystroke. See pendingNav.
	pending pendingNav

	// openExternal hands a URL off to the OS browser. Injected so tests
	// can substitute a fake that records calls instead of exec-ing.
	openExternal externalOpener

	// copyToClipboard writes selected text to the clipboard. Injected so
	// tests record calls instead of emitting a real OSC 52 escape.
	copyToClipboard clipboardWriter
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

// Pane and content zone IDs used by the View to mark hit-test regions.
// BubbleZone resolves these to bounding boxes during Scan, so Update can
// route mouse events without computing pane geometry by hand.
const (
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

	cr := code.NewRenderer(80)

	m := Model{
		root:            root,
		rootNode:        rootNode,
		history:         nav.New(),
		focus:           focusContent,
		keys:            defaultKeys(),
		vault:           v,
		recent:          rstore,
		diag:            diag,
		openExternal:    openExternalURL,
		copyToClipboard: defaultClipboardWriter,
		tree: treeUIState{
			vp:       viewport.New(0, 0),
			expanded: map[string]bool{},
		},
		content: contentUIState{
			viewport:     viewport.New(0, 0),
			renderer:     r,
			codeRenderer: cr,
			linkCursor:   -1,
		},
	}
	m.tree.flat = m.flattenVisible()
	m.modals.vp = newModalViewport()
	m.modals.picker = newPicker()
	m.modals.search = newSearch()

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
		contentWidth := m.width
		if contentWidth < 20 {
			contentWidth = 20
		}
		m.content.viewport.Width = contentWidth
		// Leave room for the two-line footer so View() fits within m.height.
		m.content.viewport.Height = m.height - 2
		m.resizeModalVP()
		m.resizePicker()
		m.resizeSearch()
		m.resizeTreeModalVP()
		m.refreshTreeVP()
		// Wrap prose at the full content width so it scales with the
		// terminal rather than capping at a fixed reading width.
		renderWidth := contentWidth
		var rOpts []markdown.Option
		if m.vault != nil {
			rOpts = append(rOpts, markdown.WithResolver(m.vault))
		}
		if r, err := markdown.NewRenderer(renderWidth, rOpts...); err == nil {
			m.content.renderer = r
		}
		m.content.codeRenderer = code.NewRenderer(renderWidth)
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

	case searchTickMsg:
		return m.handleSearchTick(msg)
	case searchResultsMsg:
		return m.handleSearchResults(msg)
	}

	// Forward other messages to the viewport when content has focus.
	if m.focus == focusContent {
		var cmd tea.Cmd
		m.content.viewport, cmd = m.content.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

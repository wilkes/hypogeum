package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"

	"github.com/wilkes/hypogeum/internal/code"
	"github.com/wilkes/hypogeum/internal/markdown"
	"github.com/wilkes/hypogeum/internal/tree"
	"github.com/wilkes/hypogeum/internal/watch"
)

// cellPos is a position in the rendered content: an absolute line index
// (independent of viewport scroll) and a visible column (0-based).
type cellPos struct {
	line int
	col  int
}

// selection tracks an in-progress or finalized text selection in the
// content pane, in absolute document coordinates.
type selection struct {
	anchored    bool    // a left-press landed in the content pane (mouse)
	moved       bool    // motion seen since press (mouse click-vs-drag)
	copied      bool    // released/yanked with text → highlight persists

	visual      bool    // keyboard visual mode is active (either phase)
	selecting   bool    // anchor dropped (Space) → caret movement extends
	anchor      cellPos // where the selection started
	cursor      cellPos // current end / caret position
	pendingLink int     // link index under a mouse press, or -1
}

// contentUIState bundles the right content pane's render state. viewport
// scrolls the rendered markdown; renderer is rebuilt at every WindowSizeMsg
// so wrap width tracks pane width; links is the per-document link list
// from the latest render; linkCursor indexes into links (-1 when nothing
// is selected).
type contentUIState struct {
	viewport     viewport.Model
	renderer     *markdown.Renderer
	codeRenderer *code.Renderer
	links        []markdown.Link
	linkCursor   int
	// brokenCount is the sum of unresolved wikilinks plus inline local
	// links whose target file is missing in the currently rendered
	// document. Recomputed by refreshContent; surfaced by renderFooter.
	brokenCount int
	// embedDeps holds the absolute paths of every source file embedded
	// in the currently displayed markdown. The TUI's handleFSEvent
	// FileModified branch re-renders the open file when a watcher event
	// arrives for any of these paths.
	embedDeps map[string]struct{}
	// rangeHighlight is non-nil when the open file is a non-markdown
	// source viewed via a range-link or embed navigation. It is cleared
	// by Esc, by opening any other file, and by following a different
	// range link.
	rangeHighlight *markdown.LineRange
	// rendered is the last full content string handed to the viewport
	// (link-highlight included). The selection overlay is drawn on top
	// of it and recomputed on every motion without re-running Glamour.
	rendered string
	// lines and lineWidths cache the split + per-line visible width of
	// rendered, recomputed once per setContent. The drag-select hot path
	// repaints the highlight on every mouse-motion event; reading these
	// caches keeps that path from re-splitting the whole document and
	// re-scanning each line's ANSI width on every event.
	lines      []string
	lineWidths []int
	// selection is the current content-pane text selection.
	selection selection
}

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

// setContent stores s as the selection overlay's base and hands it to
// the viewport. Every code path that displays real rendered content
// must go through here so content.rendered stays in sync with what the
// viewport shows.
func (m *Model) setContent(s string) {
	m.content.rendered = s
	m.content.lines = strings.Split(s, "\n")
	m.content.lineWidths = make([]int, len(m.content.lines))
	for i, ln := range m.content.lines {
		m.content.lineWidths[i] = ansi.StringWidth(ln)
	}
	m.content.viewport.SetContent(s)
}

// contentLines returns the cached split of the stored base render.
// Populated by setContent; do not mutate the result.
func (m *Model) contentLines() []string {
	return m.content.lines
}

// screenToContent maps a mouse cell (x, y) to a position in the stored
// base render. The content pane is borderless, so text begins at screen
// (0, 0); the viewport's YOffset accounts for scroll. Out-of-range
// coordinates clamp to a valid cell so drags that leave the pane or run
// past end-of-line still resolve.
func (m *Model) screenToContent(x, y int) cellPos {
	lines := m.contentLines()
	line := m.content.viewport.YOffset + y
	if line < 0 {
		line = 0
	}
	if line > len(lines)-1 {
		line = len(lines) - 1
	}
	col := x
	if col < 0 {
		col = 0
	}
	if w := m.content.lineWidths[line]; col > w {
		col = w
	}
	return cellPos{line: line, col: col}
}

// openFile records a visit in history and renders the file.
func (m *Model) openFile(path string) {
	m.history.Visit(path)
	if m.recent != nil {
		if err := m.recent.Record(path); err != nil && m.diag != nil {
			m.diag.Warn("recent: " + err.Error())
		}
	}
	m.refreshContent(path)
}

// navigateTo opens path and moves the tree cursor to its row. Used
// anywhere a file is opened by user action other than Back/Forward
// (those have their own path because they don't push history).
func (m *Model) navigateTo(path string) {
	m.openFile(path)
	m.selectInTree(path)
}

// refreshContent re-renders the file at path into the viewport without
// touching history. Used by back/forward and on resize. Also refreshes
// the link list and clears any active link selection.
func (m *Model) refreshContent(path string) {
	m.resetSelectionState()
	// Single-shot pre-select: clear the fields unconditionally before any
	// early return, so a read or render failure here can't leak a stale
	// target into the next refreshContent.
	target := m.pending.preselectTarget
	preselectRange := m.pending.preselectRange
	m.pending.preselectTarget = ""
	m.pending.preselectRange = nil

	var (
		src   []byte
		isDir bool
	)
	if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
		listing, dirErr := renderDirListing(path)
		if dirErr != nil {
			m.footerMessage = dirErr.Error()
			m.setContent(fmt.Sprintf("Error: %v", dirErr))
			m.content.links = nil
			m.content.linkCursor = -1
			m.content.brokenCount = 0
			return
		}
		src = []byte(listing)
		isDir = true
	} else {
		var err error
		src, err = os.ReadFile(path)
		if err != nil {
			m.footerMessage = err.Error()
			m.setContent(fmt.Sprintf("Error: %v", err))
			m.content.links = nil
			m.content.linkCursor = -1
			m.content.brokenCount = 0
			return
		}
	}

	if !isDir && !tree.IsMarkdown(path) {
		m.content.brokenCount = 0
		out, rerr := m.content.codeRenderer.RenderOpts(path, src, code.RenderOptions{
			Highlight: m.content.rangeHighlight,
		})
		if rerr != nil {
			m.footerMessage = rerr.Error()
			m.setContent(fmt.Sprintf("Error: %v", rerr))
		} else {
			m.currentPath = path
			m.footerMessage = ""
			m.setContent(out)
			m.content.viewport.GotoTop()
			if m.content.rangeHighlight != nil {
				m.scrollToLine(m.content.rangeHighlight.Start)
			}
		}
		m.content.links = nil
		m.content.linkCursor = -1
		m.content.embedDeps = nil
		_ = target // preselect doesn't apply to code files
		return
	}

	// Capture rangeHighlight (if any) BEFORE clearing — search-Enter
	// and any future caller can set this to ask the markdown render
	// path to scroll to a specific line after rendering. Then clear so
	// subsequent re-renders (e.g. on resize) don't keep re-scrolling.
	// NOTE: on the markdown path m.content.rangeHighlight is not set
	// from pending.preselectRange; use the local preselectRange instead.
	pendingScrollLine := 0
	if preselectRange != nil {
		pendingScrollLine = preselectRange.Start
	}
	m.content.rangeHighlight = nil
	m.content.renderer.SetFromFile(path)
	out, links, deps, err := m.content.renderer.RenderWithLinks(string(src), path, linkZoneMarker)
	if err != nil {
		m.footerMessage = err.Error()
		m.setContent(fmt.Sprintf("Error: %v", err))
		m.content.links = nil
		m.content.linkCursor = -1
		m.content.embedDeps = nil
		m.content.brokenCount = 0
		return
	}
	m.currentPath = path
	m.footerMessage = ""
	m.setContent(out)
	m.content.viewport.GotoTop()
	if pendingScrollLine > 0 {
		m.scrollToLine(pendingScrollLine)
	}
	m.content.links = links
	m.content.brokenCount = m.content.renderer.CountUnresolvedWikilinks(string(src))
	for _, l := range links {
		if l.Resolved.Kind != markdown.LinkLocalFile {
			continue
		}
		if _, err := os.Stat(l.Resolved.Target); err != nil {
			m.content.brokenCount++
		}
	}

	m.content.embedDeps = make(map[string]struct{}, len(deps))
	for _, p := range deps {
		m.content.embedDeps[p] = struct{}{}
		if m.watcher != nil {
			_ = m.watcher.AddPath(filepath.Dir(p))
		}
	}

	m.content.linkCursor = -1
	if target != "" {
		// Find the best match: same target, and if multiple, prefer the
		// one whose Range matches preselectRange (set by the originating
		// navigation). Falls back to first target match.
		best := -1
		for i, l := range links {
			if l.Resolved.Kind != markdown.LinkLocalFile || l.Resolved.Target != target {
				continue
			}
			if best < 0 {
				best = i
			}
			if rangesEqual(l.Resolved.Range, preselectRange) {
				best = i
				break
			}
		}
		if best >= 0 {
			m.content.linkCursor = best
		}
	}
	if m.content.linkCursor >= 0 {
		m.scrollToLink(m.content.links[m.content.linkCursor])
		m.applyLinkHighlight()
	}
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
		if m.vault != nil {
			if err := m.vault.Rebuild(); err != nil {
				m.diag.Warn("vault rebuild failed: " + err.Error())
			}
		}
		selectedPath := ""
		if m.tree.cursor < len(m.tree.flat) {
			selectedPath = m.tree.flat[m.tree.cursor].node.Path
		}
		newRoot, err := tree.Walk(m.root)
		if err != nil {
			m.footerMessage = err.Error()
			return
		}
		m.rootNode = newRoot
		m.tree.flat = m.flattenVisible()
		// Restore cursor by path; if the previously selected node is gone,
		// clamp to a valid index rather than dangling past the end.
		m.tree.cursor = 0
		if i := m.rowIndexByPath(selectedPath); i >= 0 {
			m.tree.cursor = i
		}
		if m.tree.cursor >= len(m.tree.flat) {
			m.tree.cursor = len(m.tree.flat) - 1
		}
		if m.tree.cursor < 0 {
			m.tree.cursor = 0
		}
		m.refreshTreeVP()

	case watch.FileModified:
		if m.vault != nil {
			for _, p := range ev.Paths {
				if err := m.vault.RefreshFile(p); err != nil {
					m.diag.Warn("vault refresh failed: " + err.Error())
				}
			}
		}
		cur := m.history.Current()
		if cur == "" {
			return
		}
		for _, p := range ev.Paths {
			matched := p == cur
			if !matched {
				if _, ok := m.content.embedDeps[p]; ok {
					matched = true
				}
			}
			if matched {
				offset := m.content.viewport.YOffset
				m.refreshContent(cur)
				// refreshContent calls GotoTop; restore scroll so a save
				// in your editor doesn't jump the reader to the start.
				m.content.viewport.SetYOffset(offset)
				return
			}
		}
	}
}

// scrollToLine positions line n of the rendered output about 25% from
// the top of the viewport. n is 1-indexed and matches what
// vault.Backlink.Line carries (a source-file line number).
//
// Caveat: source-file line numbers don't perfectly correspond to
// rendered-output line numbers (Glamour adjusts for headings, code
// fences, etc.). The user lands "near" the reference, not exactly on
// it; the snippet shown in the backlinks modal gives them a visual
// landmark to confirm.
func (m *Model) scrollToLine(n int) {
	if n < 1 {
		n = 1
	}
	total := m.content.viewport.TotalLineCount()
	if n > total {
		n = total
	}
	// Position the target line ~25% from the top of the viewport so the
	// user sees the lines preceding the reference for context.
	pad := m.content.viewport.Height / 4
	target := n - 1 - pad
	if target < 0 {
		target = 0
	}
	maxOffset := total - m.content.viewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if target > maxOffset {
		target = maxOffset
	}
	m.content.viewport.SetYOffset(target)
}

// rangesEqual reports whether two *LineRange values describe the same
// range. Two nil pointers are equal; one nil and one not are unequal.
func rangesEqual(a, b *markdown.LineRange) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Start == b.Start && a.End == b.End
}

// normalizeSel returns a, b ordered so start is at or before end in
// reading order, regardless of drag direction.
func normalizeSel(a, b cellPos) (start, end cellPos) {
	if a.line < b.line || (a.line == b.line && a.col <= b.col) {
		return a, b
	}
	return b, a
}

// selColBounds returns the [lo, hi) visible-column range selected on
// line i, given the normalized start/end and the line's visible width.
// First line starts at start.col; last line ends at end.col; middle
// lines span the whole line.
func selColBounds(i int, start, end cellPos, width int) (lo, hi int) {
	lo, hi = 0, width
	if i == start.line {
		lo = start.col
	}
	if i == end.line {
		hi = end.col
	}
	return lo, hi
}

// extractSelection returns the selected text as plain (ANSI-stripped)
// content, newline-joined across lines. Empty if the span is zero-width.
func (m *Model) extractSelection() string {
	start, end := normalizeSel(m.content.selection.anchor, m.content.selection.cursor)
	lines := m.contentLines()
	if start.line >= len(lines) {
		return ""
	}
	if end.line >= len(lines) {
		end.line = len(lines) - 1
		end.col = m.content.lineWidths[end.line]
	}
	var parts []string
	for i := start.line; i <= end.line; i++ {
		lo, hi := selColBounds(i, start, end, m.content.lineWidths[i])
		if hi < lo {
			hi = lo
		}
		parts = append(parts, strings.TrimRight(ansi.Strip(ansi.Cut(lines[i], lo, hi)), " \t"))
	}
	return strings.Join(parts, "\n")
}

// selectionStyle paints the selected span. Reverse-video swaps fg/bg so
// the selection reads like a GUI highlight regardless of theme.
var selectionStyle = lipgloss.NewStyle().Reverse(true)

// applySelectionHighlight redraws the base render with the selected span
// replaced by a uniform reverse-video block, then pushes it to the
// viewport (preserving scroll). The span is stripped before styling, so
// there are no inner escapes to cancel the reverse-video mid-span.
func (m *Model) applySelectionHighlight() {
	start, end := normalizeSel(m.content.selection.anchor, m.content.selection.cursor)
	lines := m.contentLines()
	var b strings.Builder
	b.Grow(len(m.content.rendered) + 16)
	for i, ln := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i < start.line || i > end.line {
			b.WriteString(ln)
			continue
		}
		lo, hi := selColBounds(i, start, end, m.content.lineWidths[i])
		if hi <= lo {
			// Zero-width: in keyboard visual mode draw a one-cell caret on
			// the caret's line so the user can see where they are; mouse
			// selections (visual=false) show nothing for a zero-width span.
			if m.content.selection.visual && i == m.content.selection.cursor.line {
				b.WriteString(renderCaretLine(ln, m.content.selection.cursor.col, m.content.lineWidths[i]))
			} else {
				b.WriteString(ln)
			}
			continue
		}
		b.WriteString(ansi.Cut(ln, 0, lo))
		b.WriteString(selectionStyle.Render(ansi.Strip(ansi.Cut(ln, lo, hi))))
		b.WriteString(ansi.Cut(ln, hi, m.content.lineWidths[i]))
	}
	m.setViewportPreservingScroll(b.String())
}

// renderCaretLine returns ln with a single reverse-video caret cell at
// visible column col. If col is at or past the line's content width, a
// reverse-video space is appended (caret on a blank or end-of-line).
func renderCaretLine(ln string, col, width int) string {
	if col >= width {
		return ln + selectionStyle.Render(" ")
	}
	var b strings.Builder
	b.WriteString(ansi.Cut(ln, 0, col))
	b.WriteString(selectionStyle.Render(ansi.Strip(ansi.Cut(ln, col, col+1))))
	b.WriteString(ansi.Cut(ln, col+1, width))
	return b.String()
}

// setViewportPreservingScroll replaces the viewport content while keeping
// the current scroll offset. Used by the selection-overlay paths, which
// push a transient string derived from content.rendered without redefining
// the base — so they bypass setContent on purpose.
func (m *Model) setViewportPreservingScroll(s string) {
	offset := m.content.viewport.YOffset
	m.content.viewport.SetContent(s)
	m.content.viewport.SetYOffset(offset)
}

// resetSelectionState zeroes the selection without touching the
// viewport. Used where content is about to be re-set anyway.
func (m *Model) resetSelectionState() {
	m.content.selection = selection{pendingLink: -1}
}

// finalizeSelection transitions a just-released drag into the "copied"
// state: the reverse-video highlight stays on screen until the user's
// next action, but the gesture is over, so a stray post-release motion
// event (gated on anchored) no longer extends the span.
func (m *Model) finalizeSelection() {
	m.content.selection.copied = true
	m.content.selection.anchored = false
	m.content.selection.moved = false
}

// clearSelection drops the selection and restores the un-highlighted
// base render (preserving scroll) if a highlight was showing.
func (m *Model) clearSelection() {
	had := m.content.selection.moved || m.content.selection.copied || m.content.selection.visual
	m.resetSelectionState()
	if had {
		m.setViewportPreservingScroll(m.content.rendered)
	}
}

// placeCaret moves the visual-mode caret to (line, col), clamped to valid
// cells. In the positioning phase (!selecting) the anchor tracks the caret
// so there is no span; in the extend phase only the cursor moves, growing
// the selection. Scrolls the caret into view and repaints.
func (m *Model) placeCaret(line, col int) {
	lines := m.contentLines()
	if len(lines) == 0 {
		return
	}
	if line < 0 {
		line = 0
	}
	if line > len(lines)-1 {
		line = len(lines) - 1
	}
	if col < 0 {
		col = 0
	}
	if w := m.content.lineWidths[line]; col > w {
		col = w
	}
	m.content.selection.cursor = cellPos{line: line, col: col}
	if !m.content.selection.selecting {
		m.content.selection.anchor = m.content.selection.cursor
	}
	m.scrollCaretIntoView()
	m.applySelectionHighlight()
}

// scrollCaretIntoView adjusts the viewport's YOffset so the caret's line is
// within the visible window. applySelectionHighlight (called right after)
// preserves whatever offset this sets.
func (m *Model) scrollCaretIntoView() {
	line := m.content.selection.cursor.line
	top := m.content.viewport.YOffset
	h := m.content.viewport.Height
	if h < 1 {
		return
	}
	if line < top {
		m.content.viewport.SetYOffset(line)
	} else if line >= top+h {
		m.content.viewport.SetYOffset(line - h + 1)
	}
}

// enterVisual starts keyboard visual mode in the positioning phase: a
// movable caret at the top-left of the visible area, no span yet. The
// caret is selection.cursor; anchor tracks it until Space drops the anchor.
func (m *Model) enterVisual() {
	line := m.content.viewport.YOffset
	if n := len(m.contentLines()); line > n-1 {
		line = n - 1
	}
	if line < 0 {
		line = 0
	}
	at := cellPos{line: line, col: 0}
	m.content.selection = selection{visual: true, anchor: at, cursor: at, pendingLink: -1}
	m.applySelectionHighlight()
}

// allVaultMarkdownPaths walks m.rootNode and returns every markdown file
// as an absolute path. Tree was already pruned to markdown-only by tree.Walk.
func (m *Model) allVaultMarkdownPaths() []string {
	if m.rootNode == nil {
		return nil
	}
	var out []string
	var walk func(n *tree.Node)
	walk = func(n *tree.Node) {
		if !n.IsDir {
			out = append(out, n.Path)
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(m.rootNode)
	return out
}

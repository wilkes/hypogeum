package tui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/viewport"
	zone "github.com/lrstanley/bubblezone"

	"github.com/wilkes/hypogeum/internal/code"
	"github.com/wilkes/hypogeum/internal/markdown"
	"github.com/wilkes/hypogeum/internal/tree"
	"github.com/wilkes/hypogeum/internal/watch"
)

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
	// Single-shot pre-select: clear the fields unconditionally before any
	// early return, so a read or render failure here can't leak a stale
	// target into the next refreshContent.
	target := m.pendingPreselectTarget
	preselectRange := m.pendingPreselectRange
	m.pendingPreselectTarget = ""
	m.pendingPreselectRange = nil

	var (
		src   []byte
		isDir bool
	)
	if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
		listing, dirErr := renderDirListing(path)
		if dirErr != nil {
			m.status = dirErr.Error()
			m.content.viewport.SetContent(fmt.Sprintf("Error: %v", dirErr))
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
			m.status = err.Error()
			m.content.viewport.SetContent(fmt.Sprintf("Error: %v", err))
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
			m.status = rerr.Error()
			m.content.viewport.SetContent(fmt.Sprintf("Error: %v", rerr))
		} else {
			m.status = path
			m.content.viewport.SetContent(out)
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
	// from pendingPreselectRange; use the local preselectRange instead.
	pendingScrollLine := 0
	if preselectRange != nil {
		pendingScrollLine = preselectRange.Start
	}
	m.content.rangeHighlight = nil
	m.content.renderer.SetFromFile(path)
	out, links, deps, err := m.content.renderer.RenderWithLinks(string(src), path, linkZoneMarker)
	if err != nil {
		m.status = err.Error()
		m.content.viewport.SetContent(fmt.Sprintf("Error: %v", err))
		m.content.links = nil
		m.content.linkCursor = -1
		m.content.embedDeps = nil
		m.content.brokenCount = 0
		return
	}
	m.status = path
	m.content.viewport.SetContent(out)
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
			continue
		}
		if l.Resolved.Anchor != "" && m.vault != nil {
			heading, block := splitAnchor(l.Resolved.Anchor)
			if _, ok := m.vault.ResolveAnchor(l.Resolved.Target, heading, block); !ok {
				m.content.brokenCount++
			}
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
			m.status = err.Error()
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

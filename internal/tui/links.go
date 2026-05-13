package tui

import (
	"os"
	"path/filepath"

	"github.com/wilkes/hypogeum/internal/markdown"
)

// cycleLink moves the link cursor by step, wrapping at both ends. From
// the unselected state (-1), +1 selects the first link and -1 selects
// the last. No-op when there are no links on the page.
func (m *Model) cycleLink(step int) {
	if len(m.content.links) == 0 {
		return
	}
	switch {
	case m.content.linkCursor < 0 && step > 0:
		m.content.linkCursor = 0
	case m.content.linkCursor < 0 && step < 0:
		m.content.linkCursor = len(m.content.links) - 1
	default:
		m.content.linkCursor = (m.content.linkCursor + step + len(m.content.links)) % len(m.content.links)
	}
	m.scrollToLink(m.content.links[m.content.linkCursor])
	m.applyLinkHighlight()
}

// applyLinkHighlight re-renders the current file with a reverse-video
// highlight around the selected link and updates the viewport content.
// The scroll position set by scrollToLink is preserved. On read or
// render failure, the status bar is updated and the existing viewport
// content is left unchanged.
func (m *Model) applyLinkHighlight() {
	path := m.history.Current()
	if path == "" {
		return
	}
	src, err := os.ReadFile(path)
	if err != nil {
		m.status = err.Error()
		return
	}
	m.content.renderer.SetFromFile(path)
	out, _, _, err := m.content.renderer.RenderWithLinks(string(src), path, markdown.HighlightMarker(m.content.linkCursor))
	if err != nil {
		m.status = err.Error()
		return
	}
	offset := m.content.viewport.YOffset
	m.content.viewport.SetContent(out)
	m.content.viewport.SetYOffset(offset)
}

// followLink performs whatever navigation a link's kind warrants.
// Local files navigate (recording history); external URLs arm the
// one-keystroke confirm flow (a second Enter exec's the opener);
// anchors are no-ops with a status message.
func (m *Model) followLink(l markdown.Link) {
	switch l.Resolved.Kind {
	case markdown.LinkLocalFile:
		m.navigateTo(l.Resolved.Target)
	case markdown.LinkExternal:
		m.pendingExternal = l.Href
		m.status = "press Enter again to open: " + l.Href
	case markdown.LinkAnchor:
		m.status = "anchor navigation not implemented: #" + l.Resolved.Anchor
	default:
		m.status = "unrecognized link: " + l.Href
	}
}

// scrollToLink ensures the link's row is visible in the viewport. Pads
// by one line above so the link isn't flush with the top edge.
func (m *Model) scrollToLink(l markdown.Link) {
	top := m.content.viewport.YOffset
	bottom := top + m.content.viewport.Height - 1
	switch {
	case l.Row < top:
		m.content.viewport.SetYOffset(max(0, l.Row-1))
	case l.Row > bottom:
		m.content.viewport.SetYOffset(l.Row - m.content.viewport.Height + 2)
	}
}

// selectedLink returns a pointer to the currently selected link, or nil
// if no link is selected.
func (m Model) selectedLink() *markdown.Link {
	if m.content.linkCursor < 0 || m.content.linkCursor >= len(m.content.links) {
		return nil
	}
	return &m.content.links[m.content.linkCursor]
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

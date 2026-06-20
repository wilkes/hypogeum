package tui

import (
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

// applyLinkHighlight re-renders the current document's reverse-video highlight
// around the selected link by re-applying the highlight marker to the cached
// render — no file read, no Glamour render. The scroll position set by
// scrollToLink is preserved. A nil render handle (code file / error state)
// is a no-op; such documents have no cyclable links.
func (m *Model) applyLinkHighlight() {
	if m.content.render == nil {
		return
	}
	offset := m.content.viewport.YOffset
	m.setContent(m.content.render.WithHighlight(m.content.linkCursor))
	m.content.viewport.SetYOffset(offset)
}

// followLink performs whatever navigation a link's kind warrants.
// Local files navigate (recording history); external URLs arm the
// one-keystroke confirm flow (a second Enter exec's the opener);
// anchors are no-ops with a status message.
//
// For LinkLocalFile, link.Resolved.Range is consulted: if non-nil,
// rangeHighlight is set before navigation so refreshContent picks it
// up and reverse-videos the gutter. A plain local link clears any
// stale highlight so a subsequent code-file open doesn't inherit one.
func (m *Model) followLink(l markdown.Link) {
	switch l.Resolved.Kind {
	case markdown.LinkLocalFile:
		if l.Resolved.Range != nil {
			m.content.rangeHighlight = l.Resolved.Range
		} else {
			m.content.rangeHighlight = nil
		}
		m.navigateTo(l.Resolved.Target)
	case markdown.LinkExternal:
		m.pending.externalURL = l.Href
		m.footerMessage = "press Enter again to open: " + l.Href
	case markdown.LinkAnchor:
		m.footerMessage = "anchor navigation not implemented: #" + l.Resolved.Anchor
	default:
		m.footerMessage = "unrecognized link: " + l.Href
	}
}

// followCurrentLink follows whatever link is at m.content.linkCursor.
// Thin wrapper over selectedLink + followLink, used by handleContentKey
// on Enter and by tests that exercise the Enter path directly.
func (m *Model) followCurrentLink() {
	if sel := m.selectedLink(); sel != nil {
		m.followLink(*sel)
	}
}

// scrollToLink ensures the link's row is visible in the viewport. Pads
// by one line above so the link isn't flush with the top edge.
func (m *Model) scrollToLink(l markdown.Link) {
	// Row < 0 is the embed-link sentinel: such links have no single
	// representative row in the rendered output (they're whole fenced
	// blocks). Move the cursor without disturbing scroll.
	if l.Row < 0 {
		return
	}
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

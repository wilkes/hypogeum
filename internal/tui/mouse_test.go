package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/wilkes/hypogeum/internal/markdown"
)

func TestModel_MouseClick_OnTreeRow_OpensFileAndClosesModal(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	// Open the tree modal so the row zones get registered.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.modals.kind != modalTree {
		t.Fatalf("^b should open tree modal, got kind=%v", m.modals.kind)
	}
	// Expand notes/ so its children are visible (default-collapsed).
	m.tree.expanded[filepath.Join(root, "notes")] = true
	m.tree.flat = m.flattenVisible()
	m.refreshTreeVP()
	renderAndScan(t, m, zoneContentPane)

	wantPath := filepath.Join(root, "notes", "first.md")
	target := -1
	for i, row := range m.tree.flat {
		if row.node.Path == wantPath {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("notes/first.md not in flat tree")
	}

	// Pull the row's actual screen position from BubbleZone instead of
	// hand-computing it; that keeps the test honest if the modal layout
	// changes.
	rowZone := zone.Get(treeRowZoneID(target))
	if rowZone.IsZero() {
		t.Fatalf("tree row zone for index %d not registered", target)
	}
	updated, _ := m.Update(leftClick(rowZone.StartX+1, rowZone.StartY))
	m = updated.(Model)

	if m.modals.kind != modalNone {
		t.Errorf("modal should close after file click, got kind=%v", m.modals.kind)
	}
	if m.focus != focusContent {
		t.Errorf("focus after tree click on a file = %v, want focusContent", m.focus)
	}
	if m.tree.cursor != target {
		t.Errorf("treeCursor after click = %d, want %d", m.tree.cursor, target)
	}
	if got := m.history.Current(); got != wantPath {
		t.Errorf("history.Current after click = %q, want %q", got, wantPath)
	}
}

func TestModel_MouseClick_OnContentLinkRow_FollowsLink(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	// Verify our setup assumption: the auto-opened doc has at least one link
	// and we know its row.
	if len(m.content.links) == 0 {
		t.Fatalf("fixture should yield links, got %d", len(m.content.links))
	}
	link := m.content.links[0]
	if link.Resolved.Kind != markdown.LinkLocalFile {
		t.Fatalf("first link should be local file, got %v", link.Resolved.Kind)
	}

	// Click on the link via its registered zone, not on a guessed coord.
	linkZone := zone.Get(linkZoneID(0))
	if linkZone.IsZero() {
		t.Fatalf("link zone 0 not registered after View()")
	}
	updated, _ := m.Update(leftClick(linkZone.StartX, linkZone.StartY))
	m = updated.(Model)
	updated, _ = m.Update(tea.MouseMsg{X: linkZone.StartX, Y: linkZone.StartY, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	m = updated.(Model)

	if m.focus != focusContent {
		t.Errorf("focus after content click = %v, want focusContent", m.focus)
	}
	if got := m.history.Current(); got != link.Resolved.Target {
		t.Errorf("history.Current after link click = %q, want %q", got, link.Resolved.Target)
	}
}

// A click well outside any registered zone — e.g. at (0, m.height-1)
// which is in the footer — must not change focus or history. This
// guards against the routing falling through to a wrong pane when no
// zone matches.
func TestModel_MouseClick_OutsideAnyZone_IsIgnored(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	beforePath := m.history.Current()
	beforeFocus := m.focus

	// Click in the footer area (y >= height-2). No zones live there.
	updated, _ := m.Update(leftClick(0, m.height-1))
	m = updated.(Model)

	if m.focus != beforeFocus {
		t.Errorf("footer click changed focus: %v -> %v", beforeFocus, m.focus)
	}
	if got := m.history.Current(); got != beforePath {
		t.Errorf("footer click changed history: %q -> %q", beforePath, got)
	}
}

func TestModel_MouseClick_OnContentNonLinkRow_DoesNotNavigate(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	beforePath := m.history.Current()

	// Click somewhere inside the content pane that is not on any link's
	// zone. We use the content pane's StartX,StartY (top-left interior
	// corner) which is virtually never on a link in our fixture.
	contentZone := zone.Get(zoneContentPane)
	if contentZone.IsZero() {
		t.Fatalf("content pane zone not registered")
	}
	updated, _ := m.Update(leftClick(contentZone.StartX+2, contentZone.StartY+2))
	m = updated.(Model)

	if got := m.history.Current(); got != beforePath {
		t.Errorf("non-link click changed history: %q -> %q", beforePath, got)
	}
}

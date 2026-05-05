package tui

import (
	"path/filepath"
	"testing"

	"github.com/wilkes/hypogeum/internal/markdown"
)

func TestModel_MouseClick_OnTreeRow_SelectsAndOpens(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	// Switch focus to content first so we can confirm a tree click moves it back.
	m = switchToContent(t, m)

	// Find a known row in the flat tree (notes/first.md) and compute where
	// it appears on screen: row index inside the tree pane + 1 for the top
	// border. X just needs to be inside the tree pane.
	wantPath := filepath.Join(root, "notes", "first.md")
	target := -1
	for i, row := range m.flatTree {
		if row.node.Path == wantPath {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("notes/first.md not in flat tree")
	}

	updated, _ := m.Update(leftClick(2, target+1))
	m = updated.(Model)

	if m.focus != focusTree {
		t.Errorf("focus after tree click = %v, want focusTree", m.focus)
	}
	if m.treeCursor != target {
		t.Errorf("treeCursor after click = %d, want %d", m.treeCursor, target)
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
	if len(m.links) == 0 {
		t.Fatalf("fixture should yield links, got %d", len(m.links))
	}
	link := m.links[0]
	if link.Resolved.Kind != markdown.LinkLocalFile {
		t.Fatalf("first link should be local file, got %v", link.Resolved.Kind)
	}

	// Compute screen Y for the link's row inside the content pane:
	// content pane starts at Y=1 (top border), and viewport.YOffset starts
	// at 0 for a freshly-opened doc.
	clickY := link.Row + 1
	clickX := m.treeWidth() + 5 // somewhere inside the content pane

	updated, _ := m.Update(leftClick(clickX, clickY))
	m = updated.(Model)

	if m.focus != focusContent {
		t.Errorf("focus after content click = %v, want focusContent", m.focus)
	}
	if got := m.history.Current(); got != link.Resolved.Target {
		t.Errorf("history.Current after link click = %q, want %q", got, link.Resolved.Target)
	}
}

func TestModel_MouseClick_OnContentNonLinkRow_DoesNotNavigate(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	beforePath := m.history.Current()

	// Find a content-pane row that has NO link on it.
	used := make(map[int]bool)
	for _, l := range m.links {
		used[l.Row] = true
	}
	noLinkRow := -1
	for r := 0; r < 30; r++ {
		if !used[r] {
			noLinkRow = r
			break
		}
	}
	if noLinkRow < 0 {
		t.Fatalf("could not find a row without a link")
	}

	updated, _ := m.Update(leftClick(m.treeWidth()+5, noLinkRow+1))
	m = updated.(Model)

	if got := m.history.Current(); got != beforePath {
		t.Errorf("non-link click changed history: %q -> %q", beforePath, got)
	}
}

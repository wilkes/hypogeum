package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wilkes/hypogeum/internal/markdown"
)

// writeFixture lays down a small markdown directory and returns its root.
func writeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"index.md":          "# Index\n\nSee [first](notes/first.md) and [external](https://x.test).\n",
		"notes/first.md":    "# First\n\nHello.\n",
		"notes/sub/deep.md": "# Deep\n\nNested.\n",
	}
	for rel, body := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// sized returns a model that has received an initial size message, so that
// View() produces real output rather than the empty pre-resize string.
func sized(t *testing.T, root, initialFile string) Model {
	t.Helper()
	m, err := New(root, initialFile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return updated.(Model)
}

func TestModel_BootsAndRendersFirstFile(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	view := m.View()
	if view == "" {
		t.Fatal("View returned empty string after WindowSizeMsg")
	}
	// Tree pane should mention the markdown files we wrote.
	if !strings.Contains(view, "index.md") {
		t.Errorf("expected tree to contain index.md, got:\n%s", view)
	}
	if !strings.Contains(view, "first.md") {
		t.Errorf("expected tree to contain first.md, got:\n%s", view)
	}
	// Auto-opened first file should land us on Index content.
	if !strings.Contains(view, "Index") {
		t.Errorf("expected rendered content to contain 'Index', got:\n%s", view)
	}
}

func TestModel_RefreshPopulatesLinks(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	// Auto-open lands on index.md, which has two links per writeFixture.
	if got := len(m.links); got != 2 {
		t.Fatalf("len(m.links) = %d, want 2 (index.md has [first] and [external])", got)
	}
	if m.links[0].Href != "notes/first.md" {
		t.Errorf("links[0].Href = %q, want notes/first.md", m.links[0].Href)
	}
	if m.links[1].Href != "https://x.test" {
		t.Errorf("links[1].Href = %q, want https://x.test", m.links[1].Href)
	}
}

func TestModel_FooterShowsNoLinkSelectedByDefault(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	// Phase 1: nothing selected yet, so the footer should NOT contain the
	// link-selection marker.
	if strings.Contains(m.View(), linkFooterMarker) {
		t.Errorf("expected no link-selection footer marker before any link is picked, got view:\n%s", m.View())
	}
}

func TestModel_OpensInitialFile(t *testing.T) {
	root := writeFixture(t)
	target := filepath.Join(root, "notes", "first.md")
	m := sized(t, root, target)

	if got := m.history.Current(); got != target {
		t.Errorf("history.Current = %q, want %q", got, target)
	}
	if !strings.Contains(m.View(), "First") {
		t.Errorf("expected rendered content to contain 'First'")
	}
}

// switchToContent presses Tab to move focus to the content pane.
// Used as a setup step for link-cursor tests.
func switchToContent(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.focus != focusContent {
		t.Fatalf("expected focusContent after Tab, got %v", m.focus)
	}
	return m
}

func TestModel_NextLink_SelectsFirstLink(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)
	if len(m.links) < 1 {
		t.Fatalf("fixture should yield at least one link, got %d", len(m.links))
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)
	if m.linkCursor != 0 {
		t.Errorf("linkCursor after 'n' = %d, want 0", m.linkCursor)
	}
	if !strings.Contains(m.View(), linkFooterMarker) {
		t.Errorf("expected footer marker after selecting a link")
	}
}

func TestModel_NextLink_WrapsAtEnd(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)
	for i := 0; i <= len(m.links); i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		m = updated.(Model)
	}
	// After len(m.links)+1 presses we should have wrapped from end back to 0.
	if m.linkCursor != 0 {
		t.Errorf("linkCursor after wrap = %d, want 0", m.linkCursor)
	}
}

func TestModel_PrevLink_FromUnselectedSelectsLast(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = updated.(Model)
	want := len(m.links) - 1
	if m.linkCursor != want {
		t.Errorf("linkCursor after 'p' from unselected = %d, want %d", m.linkCursor, want)
	}
}

func TestModel_EscClearsLinkSelection(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.linkCursor != -1 {
		t.Errorf("linkCursor after Esc = %d, want -1", m.linkCursor)
	}
	if strings.Contains(m.View(), linkFooterMarker) {
		t.Errorf("expected footer marker gone after Esc")
	}
}

func TestModel_EnterOnLocalLinkOpensFile(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)
	// First link in writeFixture's index.md is [first](notes/first.md).
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)
	if m.links[0].Resolved.Kind != markdown.LinkLocalFile {
		t.Fatalf("first link is not a local file, got %v", m.links[0].Resolved.Kind)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	want := filepath.Join(root, "notes", "first.md")
	if got := m.history.Current(); got != want {
		t.Errorf("history.Current after Enter = %q, want %q", got, want)
	}
	// New file means new link list and selection cleared.
	if m.linkCursor != -1 {
		t.Errorf("linkCursor after navigation = %d, want -1", m.linkCursor)
	}
}

func TestModel_EnterOnExternalLinkSetsStatus(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)
	// Cycle to the external link (second link in the fixture).
	for i := 0; i < 2; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		m = updated.(Model)
	}
	if m.links[m.linkCursor].Resolved.Kind != markdown.LinkExternal {
		t.Fatalf("expected external link selected, got %v", m.links[m.linkCursor].Resolved.Kind)
	}
	beforeHistory := m.history.Current()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.history.Current() != beforeHistory {
		t.Errorf("Enter on external link must not change history; was %q, now %q", beforeHistory, m.history.Current())
	}
	if !strings.Contains(m.status, "https://x.test") {
		t.Errorf("expected status to mention external URL, got %q", m.status)
	}
}

func TestModel_LinkKeysIgnoredWhenTreeFocused(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	// Don't switch focus — tree is focused by default.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)
	if m.linkCursor != -1 {
		t.Errorf("linkCursor after 'n' with tree focused = %d, want -1", m.linkCursor)
	}
}

// leftClick builds a tea.MouseMsg representing a left-button press at (x, y).
func leftClick(x, y int) tea.MouseMsg {
	return tea.MouseMsg{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}
}

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

func TestModel_TreeNavigationAndOpen(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")

	// Locate first.md in the flattened tree, then drive the cursor toward it
	// with up/down keystrokes (direction depends on where auto-open landed).
	want := filepath.Join(root, "notes", "first.md")
	target := -1
	for i, row := range m.flatTree {
		if row.node.Path == want {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("first.md not found in flattened tree: %+v", m.flatTree)
	}

	for m.treeCursor != target {
		var key tea.KeyMsg
		if m.treeCursor < target {
			key = tea.KeyMsg{Type: tea.KeyDown}
		} else {
			key = tea.KeyMsg{Type: tea.KeyUp}
		}
		prev := m.treeCursor
		updated, _ := m.Update(key)
		m = updated.(Model)
		if m.treeCursor == prev {
			t.Fatalf("cursor stuck at %d trying to reach %d", prev, target)
		}
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if got := m.history.Current(); got != want {
		t.Errorf("after Enter, history.Current = %q, want %q", got, want)
	}
}

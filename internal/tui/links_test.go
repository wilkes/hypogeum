package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wilkes/hypogeum/internal/markdown"
)

func TestModel_NextLink_SelectsFirstLink(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)
	if len(m.content.links) < 1 {
		t.Fatalf("fixture should yield at least one link, got %d", len(m.content.links))
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)
	if m.content.linkCursor != 0 {
		t.Errorf("linkCursor after 'n' = %d, want 0", m.content.linkCursor)
	}
	if !strings.Contains(m.View(), linkFooterMarker) {
		t.Errorf("expected footer marker after selecting a link")
	}
}

func TestModel_NextLink_WrapsAtEnd(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)
	for i := 0; i <= len(m.content.links); i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		m = updated.(Model)
	}
	// After len(m.content.links)+1 presses we should have wrapped from end back to 0.
	if m.content.linkCursor != 0 {
		t.Errorf("linkCursor after wrap = %d, want 0", m.content.linkCursor)
	}
}

func TestModel_PrevLink_FromUnselectedSelectsLast(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = switchToContent(t, m)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = updated.(Model)
	want := len(m.content.links) - 1
	if m.content.linkCursor != want {
		t.Errorf("linkCursor after 'p' from unselected = %d, want %d", m.content.linkCursor, want)
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
	if m.content.linkCursor != -1 {
		t.Errorf("linkCursor after Esc = %d, want -1", m.content.linkCursor)
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
	if m.content.links[0].Resolved.Kind != markdown.LinkLocalFile {
		t.Fatalf("first link is not a local file, got %v", m.content.links[0].Resolved.Kind)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	want := filepath.Join(root, "notes", "first.md")
	if got := m.history.Current(); got != want {
		t.Errorf("history.Current after Enter = %q, want %q", got, want)
	}
	// New file means new link list and selection cleared.
	if m.content.linkCursor != -1 {
		t.Errorf("linkCursor after navigation = %d, want -1", m.content.linkCursor)
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
	if m.content.links[m.content.linkCursor].Resolved.Kind != markdown.LinkExternal {
		t.Fatalf("expected external link selected, got %v", m.content.links[m.content.linkCursor].Resolved.Kind)
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
	if m.content.linkCursor != -1 {
		t.Errorf("linkCursor after 'n' with tree focused = %d, want -1", m.content.linkCursor)
	}
}

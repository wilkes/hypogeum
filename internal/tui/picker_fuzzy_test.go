package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"
)

// TestFuzzyDependencyAvailable is a sanity check that the matcher import
// works and the API shape matches what the picker code expects.
func TestFuzzyDependencyAvailable(t *testing.T) {
	matches := fuzzy.Find("hyp", []string{"hypogeum.md", "other.md"})
	if len(matches) != 1 {
		t.Fatalf("Find: got %d matches, want 1", len(matches))
	}
	if matches[0].Str != "hypogeum.md" {
		t.Errorf("Find: matched %q, want %q", matches[0].Str, "hypogeum.md")
	}
	if len(matches[0].MatchedIndexes) == 0 {
		t.Errorf("MatchedIndexes empty; expected positions in hypogeum.md")
	}
}

func TestPickerOpenInitializesQuery(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "b.md"), "# B")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	if got := m.modals.picker.input.Value(); got != "" {
		t.Errorf("input.Value on open: got %q want empty", got)
	}
	if !m.modals.picker.input.Focused() {
		t.Errorf("input should be focused on picker open")
	}
	if len(m.modals.picker.all) != 2 {
		t.Errorf("all: got %d entries, want 2", len(m.modals.picker.all))
	}
	if len(m.modals.picker.all) != len(m.modals.picker.ranked) {
		t.Errorf("ranked should equal all on open: %d vs %d",
			len(m.modals.picker.ranked), len(m.modals.picker.all))
	}
}

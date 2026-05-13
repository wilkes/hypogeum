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

func TestRefilterEmptyQueryRestoresAll(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "hyp.md"), "# H")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	m.modals.picker.input.SetValue("hyp")
	m.modals.picker.refilter()
	if len(m.modals.picker.ranked) != 1 {
		t.Fatalf("after 'hyp': %d matches, want 1", len(m.modals.picker.ranked))
	}

	m.modals.picker.input.SetValue("")
	m.modals.picker.refilter()
	if len(m.modals.picker.ranked) != 2 {
		t.Errorf("after clearing query: %d entries, want 2", len(m.modals.picker.ranked))
	}
	if m.modals.picker.matches != nil {
		t.Errorf("matches should be nil after clearing query")
	}
}

func TestRefilterScoresWithRecencyTiebreaker(t *testing.T) {
	// Two paths with identical fuzzy scores — the one earlier in `all`
	// (more recent) must win the stable tiebreak.
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a-hyp.md")
	p2 := filepath.Join(dir, "b-hyp.md")
	writePickerFile(t, p1, "# A")
	writePickerFile(t, p2, "# B")

	m := sized(t, dir, "")
	// Open p1 first; this bumps it ahead of p2 in `all`.
	m.openFile(p1)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	m.modals.picker.input.SetValue("hyp")
	m.modals.picker.refilter()
	if len(m.modals.picker.ranked) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(m.modals.picker.ranked))
	}
	if got := m.modals.picker.ranked[0].Path; got != p1 {
		t.Errorf("after recency tiebreak: top=%q, want %q", got, p1)
	}
}

func TestRefilterCursorResetsToZero(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "alpha.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "beta.md"), "# B")
	writePickerFile(t, filepath.Join(dir, "gamma.md"), "# G")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	m.modals.picker.cursor = 2
	m.modals.picker.input.SetValue("a")
	m.modals.picker.refilter()
	if got := m.modals.picker.cursor; got != 0 {
		t.Errorf("cursor after refilter: %d want 0", got)
	}
}

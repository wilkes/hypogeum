package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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
	// (more recently edited) must win the stable tiebreak. Source order is
	// the finder's mtime ranking, so make p1 the newer file.
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a-hyp.md")
	p2 := filepath.Join(dir, "b-hyp.md")
	writePickerFile(t, p1, "# A")
	writePickerFile(t, p2, "# B")
	base := time.Now()
	if err := os.Chtimes(p2, base.Add(-2*time.Hour), base.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p1, base.Add(-1*time.Hour), base.Add(-1*time.Hour)); err != nil {
		t.Fatal(err)
	}

	m := sized(t, dir, "")
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

func TestPickerTypingFiltersList(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "alpha.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "beta.md"), "# B")
	writePickerFile(t, filepath.Join(dir, "alphabet.md"), "# AB")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	m = pressRune(t, m, 'a')
	if got := m.modals.picker.input.Value(); got != "a" {
		t.Errorf("input.Value after 'a': %q want %q", got, "a")
	}
	if got := len(m.modals.picker.ranked); got != 3 {
		t.Errorf("ranked after 'a': %d want 3", got)
	}

	m = pressRune(t, m, 'l')
	if got := len(m.modals.picker.ranked); got != 2 {
		t.Errorf("ranked after 'al': %d want 2", got)
	}
}

func TestPickerEscClearsQueryBeforeClosing(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "b.md"), "# B")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m = pressRune(t, m, 'a')

	if m.modals.picker.input.Value() == "" {
		t.Fatal("setup: query should be non-empty")
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.modals.kind != modalPicker {
		t.Errorf("after first Esc: modal kind=%d, want modalPicker", m.modals.kind)
	}
	if m.modals.picker.input.Value() != "" {
		t.Errorf("after first Esc: query=%q, want empty", m.modals.picker.input.Value())
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.modals.kind != modalNone {
		t.Errorf("after second Esc: modal kind=%d, want modalNone", m.modals.kind)
	}
}

func TestPickerEnterOpensFilteredSelection(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "alpha.md")
	p2 := filepath.Join(dir, "beta.md")
	writePickerFile(t, p1, "# A")
	writePickerFile(t, p2, "# B")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	m = pressRune(t, m, 'b')
	if got := len(m.modals.picker.ranked); got != 1 {
		t.Fatalf("after 'b': %d matches, want 1", got)
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.history.Current(); got != p2 {
		t.Errorf("Enter after filter: opened %q want %q", got, p2)
	}
}

func TestPickerNoMatchState(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "alpha.md"), "# A")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	for _, r := range "xyzqq" {
		m = pressRune(t, m, r)
	}
	if got := len(m.modals.picker.ranked); got != 0 {
		t.Fatalf("ranked: got %d, want 0", got)
	}
	view := m.modals.picker.View()
	if !strings.Contains(view, `no match for "xyzqq"`) {
		t.Errorf("View should report no match; got:\n%s", view)
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.modals.kind != modalPicker {
		t.Errorf("Enter on no-match should not close picker; kind=%d", m.modals.kind)
	}
}

func TestPickerOverflowCap(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 250; i++ {
		name := "x" + strconv.Itoa(i) + ".md"
		writePickerFile(t, filepath.Join(dir, name), "# x")
	}

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m = pressRune(t, m, 'x')

	if got := len(m.modals.picker.ranked); got < 200 {
		t.Fatalf("setup: %d matches; need >=200", got)
	}
	overflow := len(m.modals.picker.ranked) - 200
	view := m.modals.picker.View()
	want := "… " + strconv.Itoa(overflow) + " more"
	if !strings.Contains(view, want) {
		t.Errorf("expected footer %q in View; got:\n%s", want, view)
	}
}

func TestPickerHighlightsMatchedChars(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "hypogeum.md"), "# H")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m = pressRune(t, m, 'h')
	m = pressRune(t, m, 'y')
	m = pressRune(t, m, 'p')

	view := m.modals.picker.View()
	if !strings.Contains(view, "\x1b[") {
		t.Errorf("expected ANSI escape in view; got:\n%q", view)
	}
	// The cursor row has highlight ANSI interspersed with the basename — use
	// ansi.Strip to assert the plain-text content is present regardless of styling.
	if !strings.Contains(ansi.Strip(view), "hypogeum.md") {
		t.Errorf("expected basename in view (stripped); got:\n%q", view)
	}
}

func TestPickerHighlightMultibyte(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "日本語.md"), "# JA")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'日'}})

	if got := len(m.modals.picker.ranked); got != 1 {
		t.Fatalf("expected 1 match for '日', got %d", got)
	}
	view := m.modals.picker.View()
	// ANSI highlight codes are interspersed in the basename; strip before asserting.
	if !strings.Contains(ansi.Strip(view), "日本語.md") {
		t.Errorf("expected multibyte basename in view; got:\n%q", view)
	}
}

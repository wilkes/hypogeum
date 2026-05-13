package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// writePickerFile writes content to p, creating parent dirs as needed.
// Distinct from helpers_test.writeFixture so each test can build its own
// minimal layout.
func writePickerFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPickerOpenPopulatesRanked(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "b.md"), "# B")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	if m.modals.kind != modalPicker {
		t.Fatalf("modal kind: got %d want %d", m.modals.kind, modalPicker)
	}
	if len(m.modals.picker.ranked) != 2 {
		t.Errorf("ranked: got %d want 2", len(m.modals.picker.ranked))
	}
}

func TestPickerEscClosesWithoutOpening(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")

	m := sized(t, dir, "")
	before := m.history.Current()

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.modals.kind != modalNone {
		t.Errorf("expected modalNone after Esc, got %d", m.modals.kind)
	}
	if m.history.Current() != before {
		t.Errorf("Esc should not have navigated; was %q now %q", before, m.history.Current())
	}
}

func TestPickerJKMovesCursor(t *testing.T) {
	dir := t.TempDir()
	writePickerFile(t, filepath.Join(dir, "a.md"), "# A")
	writePickerFile(t, filepath.Join(dir, "b.md"), "# B")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	if got := m.modals.picker.cursor; got != 0 {
		t.Fatalf("initial cursor: %d, want 0", got)
	}
	m = pressRune(t, m, 'j')
	if got := m.modals.picker.cursor; got != 1 {
		t.Errorf("after j: cursor=%d, want 1", got)
	}
	m = pressRune(t, m, 'k')
	if got := m.modals.picker.cursor; got != 0 {
		t.Errorf("after k: cursor=%d, want 0", got)
	}
	// k at top is clamped.
	m = pressRune(t, m, 'k')
	if got := m.modals.picker.cursor; got != 0 {
		t.Errorf("k at top: cursor=%d, want 0", got)
	}
}

func TestPickerEnterOpensSelected(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.md")
	p2 := filepath.Join(dir, "b.md")
	writePickerFile(t, p1, "# A")
	writePickerFile(t, p2, "# B")

	m := sized(t, dir, "")
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})

	// Whichever file is first in the ranked list is what Enter opens.
	want := m.modals.picker.ranked[0].Path
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.modals.kind != modalNone {
		t.Errorf("Enter should close picker, got modal kind %d", m.modals.kind)
	}
	if got := m.history.Current(); got != want {
		t.Errorf("history.Current after Enter: got %q want %q", got, want)
	}
}

func TestPickerEmptyVault(t *testing.T) {
	dir := t.TempDir()
	// No markdown files. New may fail because there's nothing to open.
	mRaw, err := New(dir, "")
	if err != nil {
		t.Skip("New on empty dir failed; not the picker's concern: " + err.Error())
	}
	updated, _ := mRaw.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m := updated.(Model)

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if len(m.modals.picker.ranked) != 0 {
		t.Errorf("expected empty ranked, got %d entries", len(m.modals.picker.ranked))
	}
	// Enter on empty list is a no-op (may or may not close the modal
	// depending on the early-exit path; we don't assert).
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	_ = m
}

func TestPickerRecentVisitBoostsRank(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.md")
	p2 := filepath.Join(dir, "b.md")
	writePickerFile(t, p1, "# A")
	writePickerFile(t, p2, "# B")
	// Make mtimes deliberately equal.
	now := time.Now()
	if err := os.Chtimes(p1, now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p2, now, now); err != nil {
		t.Fatal(err)
	}

	m := sized(t, dir, "")
	// Open p1 → its visit bumps its score above p2.
	m.openFile(p1)

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if len(m.modals.picker.ranked) < 2 {
		t.Fatalf("ranked: got %d, want >=2", len(m.modals.picker.ranked))
	}
	if got := m.modals.picker.ranked[0].Path; got != p1 {
		t.Errorf("top of rank after visiting p1: got %q want %q", got, p1)
	}
}

func TestHumanRecency(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", time.Time{}, "never"},
		{"30s", now.Add(-30 * time.Second), "just now"},
		{"5m", now.Add(-5 * time.Minute), "5m ago"},
		{"3h", now.Add(-3 * time.Hour), "3h ago"},
		{"30h", now.Add(-30 * time.Hour), "yesterday"},
		{"3d", now.Add(-3 * 24 * time.Hour), "3d ago"},
		{"3w", now.Add(-3 * 7 * 24 * time.Hour), "3w ago"},
		{"3mo", now.Add(-90 * 24 * time.Hour), "2026-02-11"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := humanRecency(now, c.t)
			if got != c.want {
				t.Errorf("humanRecency(%s): got %q want %q", c.name, got, c.want)
			}
		})
	}
}

func TestFormatPickerRowFits(t *testing.T) {
	out := formatPickerRow("a/b/c.md", "2h ago", 30)
	if w := len(out); w == 0 {
		t.Fatal("formatPickerRow returned empty")
	}
	if !strings.Contains(out, "a/b/c.md") {
		t.Errorf("row missing left content: %q", out)
	}
	if !strings.Contains(out, "2h ago") {
		t.Errorf("row missing right content: %q", out)
	}
}

func TestFormatPickerRowTruncates(t *testing.T) {
	long := "very/long/nested/path/to/some/note.md"
	out := formatPickerRow(long, "1h ago", 20)
	if !strings.Contains(out, "…") {
		t.Errorf("expected leading ellipsis in narrow row, got %q", out)
	}
	if !strings.Contains(out, "note.md") {
		t.Errorf("basename should remain visible: %q", out)
	}
}

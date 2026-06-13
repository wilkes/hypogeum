package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// writeTallContentFixture creates a single markdown file long enough to
// require scrolling at a 40-line terminal, so HalfViewDown/HalfViewUp /
// GotoTop / GotoBottom actually move the viewport. Returns the root and
// the absolute path to the tall file (pass as initialFile to sized()).
func writeTallContentFixture(t *testing.T) (root, initial string) {
	t.Helper()
	root = t.TempDir()
	var b strings.Builder
	b.WriteString("# Tall\n\n")
	for i := 0; i < 200; i++ {
		b.WriteString("paragraph ")
		b.WriteString(strings.Repeat("x", 3))
		b.WriteString("\n\n")
	}
	rel := "tall.md"
	full := filepath.Join(root, rel)
	if err := os.WriteFile(full, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, full
}

// TestModel_GotoTop_Pager asserts `g` scrolls the content viewport to top.
func TestModel_GotoTop_Pager(t *testing.T) {
	root, initial := writeTallContentFixture(t)
	m := sized(t, root, initial)
	m.content.viewport.SetYOffset(10)
	m = pressRune(t, m, 'g')
	if got := m.content.viewport.YOffset; got != 0 {
		t.Errorf("YOffset after g = %d, want 0", got)
	}
}

// TestModel_GotoBottom_Pager asserts `G` scrolls the content viewport to bottom.
func TestModel_GotoBottom_Pager(t *testing.T) {
	root, initial := writeTallContentFixture(t)
	m := sized(t, root, initial)
	m = pressRune(t, m, 'G')
	if !m.content.viewport.AtBottom() {
		t.Errorf("not at bottom: YOffset=%d, total=%d, height=%d",
			m.content.viewport.YOffset, m.content.viewport.TotalLineCount(), m.content.viewport.Height)
	}
}

// TestModel_HalfPageDown_Pager asserts ^d advances half a viewport.
func TestModel_HalfPageDown_Pager(t *testing.T) {
	root, initial := writeTallContentFixture(t)
	m := sized(t, root, initial)
	startOffset := m.content.viewport.YOffset
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	delta := m.content.viewport.YOffset - startOffset
	want := m.content.viewport.Height / 2
	if delta != want {
		t.Errorf("^d advanced by %d lines, want %d (height/2)", delta, want)
	}
}

// TestModel_HalfPageUp_Pager asserts ^u retreats half a viewport.
func TestModel_HalfPageUp_Pager(t *testing.T) {
	root, initial := writeTallContentFixture(t)
	m := sized(t, root, initial)
	m.content.viewport.SetYOffset(20)
	startOffset := m.content.viewport.YOffset
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	delta := startOffset - m.content.viewport.YOffset
	want := m.content.viewport.Height / 2
	if delta != want {
		t.Errorf("^u retreated by %d lines, want %d (height/2)", delta, want)
	}
}

func TestDefaultKeys_AllActionsBound(t *testing.T) {
	km := defaultKeys()
	v := reflect.ValueOf(km)
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i).Interface().(key.Binding)
		name := v.Type().Field(i).Name
		if len(f.Keys()) == 0 {
			t.Errorf("defaultKeys.%s has no keys bound", name)
		}
	}
}

func TestKeys_HelpTextNonEmpty(t *testing.T) {
	km := defaultKeys()
	v := reflect.ValueOf(km)
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		f := v.Field(i).Interface().(key.Binding)
		if len(f.Keys()) == 0 {
			continue
		}
		if f.Help().Desc == "" {
			t.Errorf("%s has empty help description", name)
		}
	}
}

func TestKeys_NoOverlappingActions(t *testing.T) {
	km := defaultKeys()
	v := reflect.ValueOf(km)
	seen := map[string]string{} // key spelling → field that owns it
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		f := v.Field(i).Interface().(key.Binding)
		for _, k := range f.Keys() {
			if other, dup := seen[k]; dup && other != name {
				if isAllowedKeyOverlap(name, other, k) {
					continue
				}
				t.Errorf("key %q bound to both %s and %s", k, name, other)
			}
			seen[k] = name
		}
	}
}

func TestVisualModeBindingsPresent(t *testing.T) {
	km := defaultKeys()
	if !slices.Contains(km.EnterVisual.Keys(), "v") {
		t.Errorf("EnterVisual = %v, want to include \"v\"", km.EnterVisual.Keys())
	}
	if !slices.Contains(km.BeginSelect.Keys(), " ") {
		t.Errorf("BeginSelect = %v, want to include \" \"", km.BeginSelect.Keys())
	}
}

// isAllowedKeyOverlap whitelists context-multiplexed bindings: the same
// physical key drives different actions in mutually exclusive modal
// states. Picker and search modals can't be open simultaneously (the
// single-modal-swap invariant in modals.go), so ctrl+j / ctrl+k are
// dispatched by whichever modal owns input at the moment. The keymap
// surfaces them as separate fields so help text can name each context,
// but at the key-spelling level they're the same chord.
func isAllowedKeyOverlap(a, b, key string) bool {
	pair := func(x, y string) bool {
		return (a == x && b == y) || (a == y && b == x)
	}
	if pair("PickerCursorDown", "SearchCursorDown") && key == "ctrl+j" {
		return true
	}
	if pair("PickerCursorUp", "SearchCursorUp") && key == "ctrl+k" {
		return true
	}
	// BeginSelect (Space) is active only in keyboard visual mode in the
	// content pane; ToggleFolder (Space) only fires while the tree modal is
	// open. The two states are mutually exclusive, so they safely share " ".
	if pair("BeginSelect", "ToggleFolder") && key == " " {
		return true
	}
	return false
}

package tui

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestPagerKeys_AllActionsBound(t *testing.T) {
	km := pagerKeys()
	v := reflect.ValueOf(km)
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i).Interface().(key.Binding)
		name := v.Type().Field(i).Name
		if len(f.Keys()) == 0 {
			t.Errorf("pagerKeys.%s has no keys bound", name)
		}
	}
}

// modernZeroFields lists the keyMap fields that modernKeys intentionally
// leaves as zero-value key.Binding{}. The dispatch in input.go matches
// these alongside arrow-key bindings (e.g. Up/Down), so leaving them
// empty in modern mode gives picker arrows-only navigation without any
// dispatch-code change.
var modernZeroFields = map[string]bool{
	"PickerCursorDown": true,
	"PickerCursorUp":   true,
	"SearchCursorDown": true,
	"SearchCursorUp":   true,
}

func TestModernKeys_AllActionsBound(t *testing.T) {
	km := modernKeys()
	v := reflect.ValueOf(km)
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		f := v.Field(i).Interface().(key.Binding)
		if modernZeroFields[name] {
			if len(f.Keys()) != 0 {
				t.Errorf("modernKeys.%s = %v, expected zero (intentionally disabled)", name, f.Keys())
			}
			continue
		}
		if len(f.Keys()) == 0 {
			t.Errorf("modernKeys.%s has no keys bound", name)
		}
	}
}

func TestKeysFor_Dispatch(t *testing.T) {
	cases := []struct {
		dialect    string
		wantBackTo string // a key in Back.Keys() that's unique to one dialect
	}{
		{"modern", "alt+left"},
		{"pager", "h"},
		{"", "h"},
		{"garbage", "h"},
	}
	for _, tc := range cases {
		km := keysFor(tc.dialect)
		found := false
		for _, k := range km.Back.Keys() {
			if k == tc.wantBackTo {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("keysFor(%q).Back = %v, want to include %q", tc.dialect, km.Back.Keys(), tc.wantBackTo)
		}
	}
}

func TestKeys_HelpTextNonEmpty(t *testing.T) {
	for _, dialect := range []string{"pager", "modern"} {
		km := keysFor(dialect)
		v := reflect.ValueOf(km)
		for i := 0; i < v.NumField(); i++ {
			name := v.Type().Field(i).Name
			f := v.Field(i).Interface().(key.Binding)
			if len(f.Keys()) == 0 {
				continue // zero-value bindings are intentional in modern
			}
			if f.Help().Desc == "" {
				t.Errorf("%s dialect: %s has empty help description", dialect, name)
			}
		}
	}
}

func TestKeys_NoOverlappingActions(t *testing.T) {
	for _, dialect := range []string{"pager", "modern"} {
		km := keysFor(dialect)
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
					t.Errorf("%s dialect: key %q bound to both %s and %s", dialect, k, name, other)
				}
				seen[k] = name
			}
		}
	}
}

func TestNew_OptionsSelectsDialect(t *testing.T) {
	root := writeFixture(t)
	isolatedHome(t)

	pager, err := New(root, "", Options{Dialect: "pager"})
	if err != nil {
		t.Fatalf("New pager: %v", err)
	}
	modern, err := New(root, "", Options{Dialect: "modern"})
	if err != nil {
		t.Fatalf("New modern: %v", err)
	}
	def, err := New(root, "", Options{})
	if err != nil {
		t.Fatalf("New default: %v", err)
	}

	if got := pager.keys.Back.Keys(); !contains(got, "h") {
		t.Errorf("pager.keys.Back = %v, want to include %q", got, "h")
	}
	if got := modern.keys.Back.Keys(); !contains(got, "alt+left") {
		t.Errorf("modern.keys.Back = %v, want to include %q", got, "alt+left")
	}
	if got := def.keys.Back.Keys(); !contains(got, "h") {
		t.Errorf("default opts.keys.Back = %v, want pager default %q", got, "h")
	}
}

func TestNew_OptionsSurfacesStartupWarnings(t *testing.T) {
	root := writeFixture(t)
	isolatedHome(t)

	m, err := New(root, "", Options{
		Dialect:         "pager",
		StartupWarnings: []string{"config: unknown dialect \"vim\""},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	entries := m.diag.snapshot()
	if len(entries) == 0 {
		t.Fatal("diagnostics ring is empty; want startup warning")
	}
	found := false
	for _, e := range entries {
		if strings.Contains(e.Message, `unknown dialect "vim"`) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("startup warning not in diag ring; entries=%+v", entries)
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
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
	return false
}

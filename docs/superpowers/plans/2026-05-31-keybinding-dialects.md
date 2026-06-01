# Keybinding dialects implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a TOML config file that selects between two keymap presets — `pager` (default, vim/less idioms) and `modern` (VS Code/browser idioms).

**Architecture:** New pure `internal/config/` package decodes a tiny TOML file into a `Config` struct. The `internal/tui` keymap factory splits into `pagerKeys()` and `modernKeys()`, selected by `keysFor(dialect)`. `tui.New` gains an `Options` struct that carries the dialect (and a slice of startup warnings surfaced via the existing `^l` log modal). `cmd/hypogeum/main.go` wires the two together with graceful degradation on any config error — hypogeum never refuses to start because of config.

**Tech Stack:** Go 1.21+, `github.com/BurntSushi/toml` (new dep), `github.com/charmbracelet/bubbles/key`, `github.com/charmbracelet/bubbletea`. Test pattern: standard `testing` package, table-driven, `t.TempDir()` for fixtures, `t.Setenv` for env var isolation.

**Spec:** [`docs/superpowers/specs/2026-05-31-keybinding-dialects-design.md`](../specs/2026-05-31-keybinding-dialects-design.md)

**Notes for the implementer:**

- Bubbles `key.WithKeys` accepts these string spellings (verified against bubbletea v1.3.x `key.go`): `"alt+left"`, `"alt+right"`, `"alt+b"`, `"alt+l"`, `"ctrl+home"`, `"ctrl+end"`, `"pgdown"`, `"pgup"`, `"shift+tab"`, `"ctrl+q"`, `"f1"`, `"backspace"`, `"tab"`.
- **No `ctrl+shift+letter` chords.** Terminal protocols encode `Ctrl+letter` as a single control byte (e.g. `Ctrl+B` is `\x02`); there is no byte left to additionally encode `Shift`. Bubbletea exposes `ctrl+shift+home/end/up/down/left/right` (those have distinct CSI sequences) but **not** `ctrl+shift+letter`. Modern dialect uses `Alt+letter` for chorded actions instead — terminals universally encode that as `ESC <letter>`.
- `internal/tui/diagnostics.go` exposes `(*diagnostics).Warn(msg string)`. Push startup warnings through it during model construction.
- Existing helper `sized(t, root, initialFile)` in `helpers_test.go` constructs a model — it will need a sibling that accepts `tui.Options`.

---

## File Structure

**Created:**
- `internal/config/config.go` — `Config`, `Default`, `DefaultPath`, `Load`
- `internal/config/config_test.go` — table-driven tests
- `internal/tui/keys_test.go` — keymap factory invariants

**Modified:**
- `internal/tui/keys.go` — rename `defaultKeys`→`pagerKeys`, add `modernKeys`, `keysFor`, new `keyMap` fields
- `internal/tui/model.go` — `Options` struct, new `New` signature, dialect-aware keymap init, startup warning surfacing
- `internal/tui/input.go` — dispatch for `Top`/`Bottom`/`HalfPageDown`/`HalfPageUp`
- `internal/tui/links.go` (or wherever `PrevLink` keys are tested) — update existing tests that pressed `p` to press `N`
- `internal/tui/helpers_test.go` — add `sizedWithOptions` sibling helper
- `internal/tui/input_test.go` (or new `dialect_test.go`) — modern-dialect dispatch tests
- `cmd/hypogeum/main.go` — `config.Load` wiring
- `go.mod` / `go.sum` — add `BurntSushi/toml`
- `CLAUDE.md` — Gotchas entry for dialect system
- `README.md` — config file path documentation
- `docs/index.md` — already updated alongside the spec

---

## Task 1: Bootstrap `internal/config` package — `Config`, `Default`, `DefaultPath`

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:

```go
package config

import (
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	got := Default()
	if got.Dialect != "pager" {
		t.Errorf("Default().Dialect = %q, want %q", got.Dialect, "pager")
	}
}

func TestDefaultPath(t *testing.T) {
	p, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if p == "" {
		t.Fatal("DefaultPath returned empty string with nil error")
	}
	if !strings.HasSuffix(p, "config.toml") {
		t.Errorf("DefaultPath = %q, want suffix %q", p, "hypogeum/config.toml")
	}
	if !strings.Contains(p, "hypogeum") {
		t.Errorf("DefaultPath = %q, want to contain %q", p, "hypogeum")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: build error (`config` package doesn't exist yet).

- [ ] **Step 3: Implement minimal package**

Create `internal/config/config.go`:

```go
// Package config loads hypogeum's user-config file from the
// OS-canonical user-config directory. The file is optional;
// missing or malformed configs degrade gracefully to defaults.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config is the parsed user config.
type Config struct {
	Dialect string // "pager" (default) | "modern"
}

// Default returns the zero-config defaults.
func Default() Config {
	return Config{Dialect: "pager"}
}

// DefaultPath returns the per-OS expected config location, using
// os.UserConfigDir as the base. On Linux that's $XDG_CONFIG_HOME (or
// ~/.config). On macOS, ~/Library/Application Support. On Windows,
// %AppData%. The hypogeum subdirectory is appended.
func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(base, "hypogeum", "config.toml"), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): bootstrap config package with Default and DefaultPath"
```

---

## Task 2: Implement `config.Load` — happy paths

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `go.mod` / `go.sum`

- [ ] **Step 1: Add the `BurntSushi/toml` dependency**

Run: `go get github.com/BurntSushi/toml@latest`
Expected: `go.mod` and `go.sum` updated; no other changes.

- [ ] **Step 2: Write failing tests for Load happy paths**

Append to `internal/config/config_test.go`:

```go
import (
	"os"
	"path/filepath"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_Missing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "does-not-exist.toml")
	cfg, warnings, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if cfg != (Default()) {
		t.Errorf("cfg = %+v, want %+v", cfg, Default())
	}
}

func TestLoad_DefaultDialect(t *testing.T) {
	p := writeConfig(t, "")
	cfg, warnings, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if cfg.Dialect != "pager" {
		t.Errorf("cfg.Dialect = %q, want %q", cfg.Dialect, "pager")
	}
}

func TestLoad_ValidPager(t *testing.T) {
	p := writeConfig(t, `dialect = "pager"`+"\n")
	cfg, warnings, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if cfg.Dialect != "pager" {
		t.Errorf("cfg.Dialect = %q, want %q", cfg.Dialect, "pager")
	}
}

func TestLoad_ValidModern(t *testing.T) {
	p := writeConfig(t, `dialect = "modern"`+"\n")
	cfg, warnings, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if cfg.Dialect != "modern" {
		t.Errorf("cfg.Dialect = %q, want %q", cfg.Dialect, "modern")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/config/...`
Expected: build error (`Load` not defined) or test failures.

- [ ] **Step 4: Implement Load**

Append to `internal/config/config.go`:

```go
import (
	"errors"

	"github.com/BurntSushi/toml"
)

// Load reads and validates a config file.
//
// Behavior contract:
//   - File missing → returns Default(), no warnings, nil error.
//   - File present and empty → returns Default(), no warnings.
//   - File present and valid TOML → parses dialect.
//     If dialect is not one of the recognized values, falls back to
//     "pager" and returns a warning naming the valid options.
//   - File present but malformed TOML or unreadable → returns Default()
//     with a non-nil error. The caller decides how to surface the error;
//     hypogeum's main.go logs it to stderr and continues with defaults.
func Load(path string) (Config, []string, error) {
	cfg := Default()

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil, nil
		}
		return Default(), nil, fmt.Errorf("read %s: %w", path, err)
	}

	var parsed struct {
		Dialect string `toml:"dialect"`
	}
	if _, err := toml.Decode(string(raw), &parsed); err != nil {
		return Default(), nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var warnings []string
	switch parsed.Dialect {
	case "":
		// Field omitted; keep default.
	case "pager", "modern":
		cfg.Dialect = parsed.Dialect
	default:
		warnings = append(warnings,
			fmt.Sprintf(`config: unknown dialect %q (valid options: "pager", "modern"); falling back to "pager"`, parsed.Dialect))
	}

	return cfg, warnings, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/...`
Expected: all four tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go go.mod go.sum
git commit -m "feat(config): add Load with happy-path TOML decoding"
```

---

## Task 3: Implement `config.Load` validation paths

**Files:**
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Add failing validation tests**

Append to `internal/config/config_test.go`:

```go
func TestLoad_UnknownDialect(t *testing.T) {
	p := writeConfig(t, `dialect = "vim"`+"\n")
	cfg, warnings, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Dialect != "pager" {
		t.Errorf("cfg.Dialect = %q, want fallback %q", cfg.Dialect, "pager")
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one warning", warnings)
	}
	if !strings.Contains(warnings[0], `"vim"`) {
		t.Errorf("warning %q should mention the invalid value", warnings[0])
	}
	if !strings.Contains(warnings[0], "pager") || !strings.Contains(warnings[0], "modern") {
		t.Errorf("warning %q should name valid options", warnings[0])
	}
}

func TestLoad_MalformedTOML(t *testing.T) {
	p := writeConfig(t, "dialect = =\n")
	cfg, _, err := Load(p)
	if err == nil {
		t.Fatal("Load: want error for malformed TOML, got nil")
	}
	if cfg != Default() {
		t.Errorf("cfg = %+v, want defaults on error", cfg)
	}
}

func TestLoad_UnreadablePerm(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read 0o000 files")
	}
	p := writeConfig(t, `dialect = "modern"`+"\n")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	cfg, _, err := Load(p)
	if err == nil {
		t.Fatal("Load: want error for unreadable file, got nil")
	}
	if cfg != Default() {
		t.Errorf("cfg = %+v, want defaults on error", cfg)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/config/...`
Expected: All three new tests PASS — the implementation from Task 2 already handles these cases. If `TestLoad_UnknownDialect` fails, double-check the warning message format includes both `"pager"` and `"modern"` literally.

- [ ] **Step 3: Commit**

```bash
git add internal/config/config_test.go
git commit -m "test(config): pin Load behavior for unknown dialect, malformed, unreadable"
```

---

## Task 4: Add new `keyMap` fields + rename `defaultKeys` → `pagerKeys` + dispatch

**Files:**
- Modify: `internal/tui/keys.go`
- Modify: `internal/tui/model.go:174`
- Modify: `internal/tui/input.go`
- Create: `internal/tui/keys_test.go`

This task adds the four new actions (`Top`, `Bottom`, `HalfPageDown`, `HalfPageUp`) end-to-end: keymap field, binding in pager factory, dispatch wiring, and tests. It also renames `defaultKeys` → `pagerKeys` so the next task can introduce `modernKeys` cleanly.

- [ ] **Step 1: Write failing dispatch tests**

Create `internal/tui/keys_test.go`:

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestModel_GotoTop_Pager asserts `g` scrolls the content viewport to top.
func TestModel_GotoTop_Pager(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.content.viewport.SetYOffset(10)
	m = pressRune(t, m, 'g')
	if got := m.content.viewport.YOffset; got != 0 {
		t.Errorf("YOffset after g = %d, want 0", got)
	}
}

// TestModel_GotoBottom_Pager asserts `G` scrolls the content viewport to bottom.
func TestModel_GotoBottom_Pager(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = pressRune(t, m, 'G')
	if got := m.content.viewport.YOffset; got != m.content.viewport.TotalLineCount()-m.content.viewport.Height && !m.content.viewport.AtBottom() {
		t.Errorf("not at bottom: YOffset=%d, total=%d, height=%d",
			got, m.content.viewport.TotalLineCount(), m.content.viewport.Height)
	}
}

// TestModel_HalfPageDown_Pager asserts ^d advances half a viewport.
func TestModel_HalfPageDown_Pager(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	startOffset := m.content.viewport.YOffset
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	delta := m.content.viewport.YOffset - startOffset
	half := m.content.viewport.Height / 2
	if delta < half-1 || delta > half+1 {
		t.Errorf("^d advanced by %d lines, want ~%d (height/2)", delta, half)
	}
}

// TestModel_HalfPageUp_Pager asserts ^u retreats half a viewport.
func TestModel_HalfPageUp_Pager(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m.content.viewport.SetYOffset(20)
	startOffset := m.content.viewport.YOffset
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	delta := startOffset - m.content.viewport.YOffset
	half := m.content.viewport.Height / 2
	if delta < half-1 || delta > half+1 {
		t.Errorf("^u retreated by %d lines, want ~%d (height/2)", delta, half)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run TestModel_GotoTop_Pager`
Expected: FAIL — `g` is currently unbound, so pressing it has no effect.

- [ ] **Step 3: Rename `defaultKeys` → `pagerKeys` and add new fields**

Edit `internal/tui/keys.go`. Replace the full file with:

```go
package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap collects every keybinding the model knows about. Centralizing
// them makes the help cheat sheet trivial to render and dialects easy
// to define as alternative factory functions.
type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Open    key.Binding
	Back    key.Binding
	Forward key.Binding
	Quit    key.Binding

	NextLink  key.Binding
	PrevLink  key.Binding
	ClearLink key.Binding

	OpenBacklinksModal key.Binding
	OpenLogsModal      key.Binding
	OpenHelpModal      key.Binding

	ToggleTree   key.Binding
	ToggleFolder key.Binding

	OpenPicker       key.Binding
	PickerCursorDown key.Binding
	PickerCursorUp   key.Binding

	OpenSearch       key.Binding
	SearchCursorDown key.Binding
	SearchCursorUp   key.Binding

	Top          key.Binding
	Bottom       key.Binding
	HalfPageDown key.Binding
	HalfPageUp   key.Binding
}

func pagerKeys() keyMap {
	return keyMap{
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Open:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:    key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "back")),
		Forward: key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "forward")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

		NextLink:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next link")),
		PrevLink:  key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev link")),
		ClearLink: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear link")),

		OpenBacklinksModal: key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "backlinks")),
		OpenLogsModal:      key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("^l", "logs")),
		OpenHelpModal:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),

		ToggleTree:   key.NewBinding(key.WithKeys("ctrl+b"), key.WithHelp("^b", "open tree")),
		ToggleFolder: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "expand/collapse")),

		OpenPicker:       key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("^p", "open file…")),
		PickerCursorDown: key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("^j", "picker: next")),
		PickerCursorUp:   key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("^k", "picker: prev")),

		OpenSearch:       key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("^s", "search…")),
		SearchCursorDown: key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("^j", "search: next")),
		SearchCursorUp:   key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("^k", "search: prev")),

		Top:          key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:       key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "half-page down")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "half-page up")),
	}
}
```

Note: `NextLink`/`PrevLink` keep `n`/`p` for now. The `p` → `N` change is a separate task (Task 5) so the diff stays focused.

- [ ] **Step 4: Update the single call site of `defaultKeys`**

In `internal/tui/model.go:174`, change:

```go
keys:         defaultKeys(),
```

to:

```go
keys:         pagerKeys(),
```

- [ ] **Step 5: Wire dispatch for the four new actions**

In `internal/tui/input.go`, find `handleContentKey` (the function around line 360 that already dispatches `NextLink`/`PrevLink`/`ClearLink`). At the end of its main `switch` block — *after* the existing cases but *before* the final fallthrough to viewport scrolling — add:

```go
case key.Matches(msg, m.keys.Top):
	m.content.viewport.GotoTop()
	return *m, nil
case key.Matches(msg, m.keys.Bottom):
	m.content.viewport.GotoBottom()
	return *m, nil
case key.Matches(msg, m.keys.HalfPageDown):
	m.content.viewport.HalfViewDown()
	return *m, nil
case key.Matches(msg, m.keys.HalfPageUp):
	m.content.viewport.HalfViewUp()
	return *m, nil
```

If `handleContentKey` doesn't have a single `switch` covering link cases and fallthrough — instead has multiple early-returns — append the four cases to whichever `switch` handles the link dispatch. The key thing: these cases must run *before* any default `m.content.viewport.Update(msg)` fallback so the viewport's own `g`/`G` bindings (if any) don't compete.

- [ ] **Step 6: Run the new dispatch tests**

Run: `go test ./internal/tui/ -run TestModel_GotoTop_Pager`
Expected: PASS.

Run: `go test ./internal/tui/ -run TestModel_GotoBottom_Pager`
Expected: PASS.

Run: `go test ./internal/tui/ -run TestModel_HalfPage`
Expected: both PASS.

- [ ] **Step 7: Run the full TUI suite to catch regressions**

Run: `go test ./internal/tui/...`
Expected: all PASS. Particularly verify `TestModel_ArrowKeysShadowHistoryWhileTreeModalOpen` still passes (the tree-modal shadow invariant).

- [ ] **Step 8: Commit**

```bash
git add internal/tui/keys.go internal/tui/keys_test.go internal/tui/model.go internal/tui/input.go
git commit -m "feat(tui): add g/G/^d/^u and rename defaultKeys to pagerKeys"
```

---

## Task 5: Pager binding delta — prev-link `p` → `N`

The spec moves "previous link" from `p` to `N` (Shift+n) in pager mode, aligning with vim's `n`/`N` search-next/prev idiom.

**Files:**
- Modify: `internal/tui/keys.go` (PrevLink binding)
- Modify: any test in `internal/tui/links_test.go` that presses `p` for prev-link

- [ ] **Step 1: Find tests that exercise `p` for prev-link**

Run: `grep -n "pressRune.*'p'\|KeyRunes.*'p'" internal/tui/links_test.go`
Expected: a list of lines. These tests need their key updated from `'p'` to `'N'`.

- [ ] **Step 2: Update the failing test inputs**

For each occurrence found in Step 1, replace:

```go
m = pressRune(t, m, 'p')
```

with:

```go
m = pressRune(t, m, 'N')
```

The expected *behavior* doesn't change — the test still asserts "cursor moves to previous link." Only the key changes.

If a test's name explicitly mentions the `p` key (e.g. `TestLinkCycle_P`), rename it (`TestLinkCycle_ShiftN`) and update any related comments.

- [ ] **Step 3: Run the link tests to verify they fail**

Run: `go test ./internal/tui/ -run TestLink`
Expected: FAIL — `p` is still bound, but tests now press `N` which isn't bound yet.

- [ ] **Step 4: Update the PrevLink binding**

In `internal/tui/keys.go`, replace the `PrevLink` line inside `pagerKeys()`:

```go
PrevLink: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev link")),
```

with:

```go
PrevLink: key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev link")),
```

- [ ] **Step 5: Run the link tests**

Run: `go test ./internal/tui/ -run TestLink`
Expected: PASS.

- [ ] **Step 6: Run the full TUI suite**

Run: `go test ./internal/tui/...`
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/keys.go internal/tui/links_test.go
git commit -m "feat(tui): rebind prev-link from p to N in pager dialect"
```

---

## Task 6: Add `modernKeys` factory + `keysFor` dispatcher

**Files:**
- Modify: `internal/tui/keys.go`
- Modify: `internal/tui/keys_test.go`

- [ ] **Step 1: Write failing factory invariant tests**

Append to `internal/tui/keys_test.go`:

```go
import (
	"reflect"

	"github.com/charmbracelet/bubbles/key"
)

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
					// Special case: the cursor-key duplication between
					// Up/Down and PickerCursorUp/Down in pager mode is
					// allowed since the dispatch matches both.
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

// isAllowedKeyOverlap whitelists known-good key overlaps that exist by
// design. As of v1, none — the cursor-key fields in pager mode use
// distinct keys (j/k vs ^j/^k).
func isAllowedKeyOverlap(a, b, key string) bool {
	return false
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run TestModernKeys`
Expected: build error — `modernKeys` is not defined.

Run: `go test ./internal/tui/ -run TestKeysFor`
Expected: build error — `keysFor` is not defined.

- [ ] **Step 3: Add `modernKeys` and `keysFor`**

Append to `internal/tui/keys.go`:

```go
func modernKeys() keyMap {
	return keyMap{
		Up:      key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
		Down:    key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
		Open:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:    key.NewBinding(key.WithKeys("alt+left", "backspace"), key.WithHelp("alt+←/⌫", "back")),
		Forward: key.NewBinding(key.WithKeys("alt+right"), key.WithHelp("alt+→", "forward")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+q", "ctrl+c"), key.WithHelp("q/^q", "quit")),

		NextLink:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next link")),
		PrevLink:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧⇥", "prev link")),
		ClearLink: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear link")),

		OpenBacklinksModal: key.NewBinding(key.WithKeys("alt+b"), key.WithHelp("alt+b", "backlinks")),
		OpenLogsModal:      key.NewBinding(key.WithKeys("alt+l"), key.WithHelp("alt+l", "logs")),
		OpenHelpModal:      key.NewBinding(key.WithKeys("?", "f1"), key.WithHelp("?/F1", "help")),

		ToggleTree:   key.NewBinding(key.WithKeys("ctrl+b"), key.WithHelp("^b", "open tree")),
		ToggleFolder: key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "expand/collapse")),

		OpenPicker: key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("^p", "open file…")),
		// Picker/SearchCursor fields intentionally zero-valued so the
		// dispatcher falls through to Up/Down (which are arrow-only
		// in modern). See TestModernKeys_AllActionsBound.

		OpenSearch: key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("^f", "search…")),

		Top:          key.NewBinding(key.WithKeys("ctrl+home"), key.WithHelp("^home", "top")),
		Bottom:       key.NewBinding(key.WithKeys("ctrl+end"), key.WithHelp("^end", "bottom")),
		HalfPageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
		HalfPageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
	}
}

// keysFor returns the keyMap for the named dialect. Unknown values fall
// back to pager — this is the runtime mirror of config.Load's validation
// fallback, so the binary stays usable even if a config slipped through.
func keysFor(dialect string) keyMap {
	switch dialect {
	case "modern":
		return modernKeys()
	default:
		return pagerKeys()
	}
}
```

- [ ] **Step 4: Run the factory tests**

Run: `go test ./internal/tui/ -run "TestPagerKeys|TestModernKeys|TestKeysFor|TestKeys_"`
Expected: all PASS.

- [ ] **Step 5: Run the full TUI suite**

Run: `go test ./internal/tui/...`
Expected: all PASS. The new factory isn't wired in yet (the model still uses `pagerKeys()` directly), so behavior is unchanged.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/keys.go internal/tui/keys_test.go
git commit -m "feat(tui): add modernKeys factory and keysFor dispatcher"
```

---

## Task 7: Add `tui.Options` struct + dialect-aware `tui.New`

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/helpers_test.go`
- Modify: `cmd/hypogeum/main.go` (call site update; full config wiring is Task 9)

- [ ] **Step 1: Write a failing test that uses Options**

Append to `internal/tui/keys_test.go`:

```go
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
```

Note: this test calls `m.diag.snapshot()`. If `diagnostics.go` doesn't have a public-ish snapshot method, use whatever surface the existing log modal uses to read entries — likely something like `m.diag.entries()` or direct field access since this is intra-package. Check `internal/tui/logs.go` to mirror its pattern.

Also add the `strings` import if not present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestNew_Options`
Expected: build error — `Options` struct doesn't exist, and `New` takes two args.

- [ ] **Step 3: Define `Options` and update `New`**

In `internal/tui/model.go`, near the top of the file (after the `Model` struct definition), add:

```go
// Options bundles construction-time settings. Carries forward-growable
// configuration without ballooning the New signature.
type Options struct {
	// Dialect selects the keymap factory. Empty or unknown values
	// fall back to "pager".
	Dialect string

	// StartupWarnings are non-fatal messages surfaced into the in-app
	// log modal (^l or ^Shift+l) at model construction. Typically
	// populated by main.go from config.Load's warnings slice.
	StartupWarnings []string
}
```

Change `New`'s signature from:

```go
func New(root, initialFile string) (Model, error) {
```

to:

```go
func New(root, initialFile string, opts Options) (Model, error) {
```

Inside `New`, change line ~174:

```go
keys:         pagerKeys(),
```

to:

```go
keys:         keysFor(opts.Dialect),
```

After the `diag` initialization (search for where `diag.Warn` is called for the recent-store; you want to add right after that block), add:

```go
for _, w := range opts.StartupWarnings {
	diag.Warn(w)
}
```

- [ ] **Step 4: Update the `sized` test helper**

In `internal/tui/helpers_test.go`, update `sized`:

```go
func sized(t *testing.T, root, initialFile string) Model {
	t.Helper()
	isolatedHome(t)
	m, err := New(root, initialFile, Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	renderAndScan(t, m, zoneContentPane)
	return m
}

// sizedWithOptions is the dialect-aware variant of sized, used by tests
// that need to exercise modern bindings.
func sizedWithOptions(t *testing.T, root, initialFile string, opts Options) Model {
	t.Helper()
	isolatedHome(t)
	m, err := New(root, initialFile, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	renderAndScan(t, m, zoneContentPane)
	return m
}
```

- [ ] **Step 5: Update the `cmd/hypogeum/main.go` call site**

The full config-loading wiring is in Task 9. For now, just update the single call site so the package builds:

```go
model, err := tui.New(root, initialFile, tui.Options{})
```

- [ ] **Step 6: Run the full test suite**

Run: `go test ./...`
Expected: all PASS, including the two new `TestNew_Options*` tests.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/model.go internal/tui/helpers_test.go internal/tui/keys_test.go cmd/hypogeum/main.go
git commit -m "feat(tui): add Options struct and dialect-aware New"
```

---

## Task 8: Modern-dialect dispatch tests

Reproduce a handful of existing pager tests under modern bindings to prove the dispatch is dialect-transparent.

**Files:**
- Create: `internal/tui/dialect_modern_test.go`

- [ ] **Step 1: Write the modern-dialect dispatch tests**

Create `internal/tui/dialect_modern_test.go`:

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// modernModel constructs a sized model wired with the modern keymap.
func modernModel(t *testing.T, root string) Model {
	t.Helper()
	return sizedWithOptions(t, root, "", Options{Dialect: "modern"})
}

func TestModel_BackForward_Modern(t *testing.T) {
	root := writeFixture(t)
	m := modernModel(t, root)

	// Navigate to a second file so Back has somewhere to go.
	m.navigateTo(root + "/notes/first.md")
	prevTop := m.history.Current()

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
	if got := m.history.Current(); got == prevTop {
		t.Errorf("Alt+← did not navigate back: current still %q", got)
	}
	prevTop = m.history.Current()
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight, Alt: true})
	if got := m.history.Current(); got == prevTop {
		t.Errorf("Alt+→ did not navigate forward: current still %q", got)
	}
}

func TestModel_OpenSearch_Modern(t *testing.T) {
	root := writeFixture(t)
	m := modernModel(t, root)

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlF})
	if m.modals.kind != modalSearch {
		t.Errorf("Ctrl+F did not open search modal; kind=%v", m.modals.kind)
	}
}

func TestModel_OpenBacklinks_Modern(t *testing.T) {
	root := writeFixture(t)
	m := modernModel(t, root)

	// Alt+b is encoded as a rune-bearing KeyMsg with Alt=true.
	m = pressKey(t, m, tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'b'},
		Alt:   true,
	})
	if m.modals.kind != modalBacklinks {
		t.Errorf("Alt+b did not open backlinks modal; kind=%v", m.modals.kind)
	}
}

func TestModel_OpenLogs_Modern(t *testing.T) {
	root := writeFixture(t)
	m := modernModel(t, root)

	m = pressKey(t, m, tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'l'},
		Alt:   true,
	})
	if m.modals.kind != modalLogs {
		t.Errorf("Alt+l did not open logs modal; kind=%v", m.modals.kind)
	}
}

func TestModel_LinkCycle_Modern(t *testing.T) {
	root := writeFixture(t)
	m := modernModel(t, root)

	startCursor := m.content.linkCursor
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.content.linkCursor == startCursor {
		t.Errorf("Tab did not advance link cursor (started at %d)", startCursor)
	}
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.content.linkCursor != startCursor {
		t.Errorf("Shift+Tab did not return cursor to start; got %d, want %d",
			m.content.linkCursor, startCursor)
	}
}

func TestModel_QuitBothBindings_Modern(t *testing.T) {
	root := writeFixture(t)

	// Each q variant — bare 'q' and Ctrl+Q — must yield a tea.Quit cmd.
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyCtrlQ},
	} {
		m := modernModel(t, root)
		_, cmd := m.Update(key)
		if cmd == nil {
			t.Errorf("modern quit (%v) did not return a command", key)
			continue
		}
		// tea.Quit is the singleton command from bubbletea.
		// Identity comparison is the standard way to check.
		// (If your bubbletea version doesn't expose it this way,
		// invoke cmd() and check the returned msg is tea.QuitMsg{}.)
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("modern quit (%v) did not produce QuitMsg; got %T", key, msg)
		}
	}
}

func TestModel_GotoTop_Modern(t *testing.T) {
	root := writeFixture(t)
	m := modernModel(t, root)
	m.content.viewport.SetYOffset(10)
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlHome})
	if got := m.content.viewport.YOffset; got != 0 {
		t.Errorf("Ctrl+Home did not goto top: YOffset=%d", got)
	}
}

func TestModel_PageDown_Modern(t *testing.T) {
	root := writeFixture(t)
	m := modernModel(t, root)
	startOffset := m.content.viewport.YOffset
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.content.viewport.YOffset == startOffset {
		t.Errorf("PageDown did not advance; YOffset still %d", startOffset)
	}
}
```

Note on key synthesis: bubbletea's `tea.KeyMsg` has a `Type` field, a `Runes` field, and an `Alt` modifier. There is **no `Ctrl` modifier field** — Ctrl combinations come in as dedicated `KeyType` constants (`tea.KeyCtrlHome`, `tea.KeyCtrlF`, `tea.KeyCtrlEnd`, etc.). Alt combinations use `tea.KeyMsg{Type: KeyRunes, Runes: ..., Alt: true}` for letter chords like `Alt+B`, or `tea.KeyMsg{Type: KeyLeft, Alt: true}` for arrow chords. If a test fails with a "no match" symptom, `fmt.Printf("%q\n", msg.String())` shows what string `key.Matches` is comparing against — align the binding string to that.

- [ ] **Step 2: Run the modern dispatch tests**

Run: `go test ./internal/tui/ -run "_Modern"`
Expected: all PASS.

If `TestModel_OpenSearch_Modern` fails on `Ctrl+Shift+F` and the fallback `Ctrl+F` also fails, check that the binding `"ctrl+shift+f"` matches what bubbletea emits — you may need to align the binding to `"ctrl+f"` only and document the chord limitation.

- [ ] **Step 3: Run the full suite**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/dialect_modern_test.go
git commit -m "test(tui): pin modern-dialect dispatch behavior"
```

---

## Task 9: Wire config loading in `cmd/hypogeum/main.go`

**Files:**
- Modify: `cmd/hypogeum/main.go`
- Create: `cmd/hypogeum/main_test.go` (if it doesn't already exist; otherwise modify)

- [ ] **Step 1: Inspect the current main.go run function**

Run: `cat cmd/hypogeum/main.go`
Confirm `run` looks like the Task header expects. The change is local to `run`.

- [ ] **Step 2: Wire config.Load into run**

Replace the body of `run` in `cmd/hypogeum/main.go`:

```go
func run(args []string) error {
	for _, a := range args {
		if a == "--version" || a == "-v" {
			fmt.Printf("hypogeum %s (commit %s, built %s)\n", version, commit, date)
			return nil
		}
	}
	root, initialFile, err := resolveTarget(args)
	if err != nil {
		return err
	}

	cfg, warnings := loadConfig()

	model, err := tui.New(root, initialFile, tui.Options{
		Dialect:         cfg.Dialect,
		StartupWarnings: warnings,
	})
	if err != nil {
		return err
	}

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

// loadConfig reads the user config and translates any error into a
// startup warning the TUI will surface via ^l. A parse error also goes
// to stderr (visible before the alt-screen takes over and again after
// exit). loadConfig never returns an error; hypogeum always starts.
func loadConfig() (config.Config, []string) {
	cfgPath, pathErr := config.DefaultPath()
	if pathErr != nil {
		return config.Default(), []string{"config: " + pathErr.Error() + "; using defaults"}
	}
	cfg, warnings, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hypogeum: %s: %v (using defaults)\n", cfgPath, err)
		warnings = append(warnings, fmt.Sprintf("config %s: %v; using defaults", cfgPath, err))
		return config.Default(), warnings
	}
	return cfg, warnings
}
```

Update the import block to include `"github.com/wilkes/hypogeum/internal/config"`.

- [ ] **Step 3: Add a unit test for loadConfig**

Create or append to `cmd/hypogeum/main_test.go`:

```go
package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// loadConfig is hard to test in isolation because DefaultPath uses
// os.UserConfigDir which can't be redirected by env vars on macOS.
// Instead, test the behavior via a temporary HOME and exercise the
// happy path on Linux where XDG_CONFIG_HOME is honored.
func TestLoadConfig_MissingFileDoesNotError(t *testing.T) {
	// Redirect to a temp dir where no config file exists.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))

	cfg, warnings := loadConfig()
	if cfg.Dialect != "pager" {
		t.Errorf("Dialect = %q, want %q", cfg.Dialect, "pager")
	}
	for _, w := range warnings {
		if strings.Contains(w, "using defaults") {
			t.Errorf("unexpected warning for missing file: %q", w)
		}
	}
}
```

Note: macOS uses `~/Library/Application Support` regardless of `XDG_CONFIG_HOME`, so this test exercises the missing-file path on Linux CI and the macOS path stays untested at the main level — but it's exhaustively tested in `internal/config/config_test.go`, so that's fine.

- [ ] **Step 4: Run all tests**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 5: Run a smoke test manually**

Run: `go build ./... && ./hypogeum --version`
Expected: prints version info.

Run: `mkdir -p /tmp/hypo-test && echo "# hi" > /tmp/hypo-test/index.md && ./hypogeum /tmp/hypo-test` (in a real terminal)
Expected: hypogeum launches normally with pager bindings (the default).

Run: `mkdir -p ~/Library/Application\ Support/hypogeum && echo 'dialect = "modern"' > ~/Library/Application\ Support/hypogeum/config.toml` (macOS) or equivalent path on your OS.
Expected: relaunching hypogeum now uses modern bindings — `?` shows them.

Clean up: `rm -i ~/Library/Application\ Support/hypogeum/config.toml` after smoke-testing.

- [ ] **Step 6: Commit**

```bash
git add cmd/hypogeum/main.go cmd/hypogeum/main_test.go
git commit -m "feat(cli): wire config.Load with graceful degradation"
```

---

## Task 10: Documentation updates

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Add CLAUDE.md gotcha entry**

In `CLAUDE.md`, under the `## Gotchas` section, add this entry (place it near other binding-related gotchas — likely after the `^s` paragraph or the `**Picker grabs printable keys before global modal-toggles**` paragraph):

```markdown
- **Keybindings come from a dialect factory, not a single default.** `internal/tui/keys.go` exposes `pagerKeys()` (default) and `modernKeys()`; the model picks one via `keysFor(opts.Dialect)` in `tui.New`. Users select the dialect via `~/.config/hypogeum/config.toml` (path varies per OS — see `internal/config`). Unknown dialect values fall back to pager with a `^l`-visible warning. The dispatch code in `input.go` doesn't know about dialects — it just calls `key.Matches(msg, m.keys.X)`. Adding a new action means a new `keyMap` field, a binding in both factories, and dispatch wiring; adding a new dialect means a new factory and one case in `keysFor`. See [keybinding-dialects design](docs/superpowers/specs/2026-05-31-keybinding-dialects-design.md).
```

- [ ] **Step 2: Update CLAUDE.md package layering paragraph**

Find the paragraph that lists packages (`internal/tree`, `internal/markdown`, etc. — under `## Layout`). Add a line for the new `internal/config` package:

```markdown
internal/config/         Loads ~/.config/hypogeum/config.toml (TOML). Pure, no TUI deps; degrades gracefully on missing/malformed files.
```

- [ ] **Step 3: Add README.md config section**

In `README.md`, add a `## Configuration` section near the existing usage docs:

```markdown
## Configuration

Hypogeum reads an optional config file from your platform's user-config
directory. Missing or malformed configs are not fatal — hypogeum always
starts with sensible defaults.

| OS      | Path                                                        |
| ------- | ----------------------------------------------------------- |
| Linux   | `$XDG_CONFIG_HOME/hypogeum/config.toml` (or `~/.config/...`) |
| macOS   | `~/Library/Application Support/hypogeum/config.toml`         |
| Windows | `%AppData%\hypogeum\config.toml`                            |

### Available settings

```toml
# dialect selects the keybinding preset.
#   "pager"  (default): vim/less idioms — h/l history, n/N link cycle,
#                       j/k motion, / for search, g/G top/bottom.
#   "modern":           browser/editor idioms — Alt+←/→ history,
#                       Tab/Shift+Tab link cycle, arrows for motion,
#                       Ctrl+F for search, Alt+b/Alt+l for modals.
dialect = "pager"
```

Press `?` in hypogeum to see the active dialect's full keybinding list.
Errors loading the config file appear in the `^l` log modal (or
`Ctrl+Shift+L` in modern dialect).
```

- [ ] **Step 4: Final test run**

Run: `go test -race ./...`
Expected: all PASS, race-clean.

Run: `go vet ./...`
Expected: no warnings.

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: keybinding dialects user-facing documentation"
```

---

## Final verification

After all tasks land, verify the spec's "Verification" section:

- [ ] `go test ./...` — full suite green
- [ ] `go test -race ./...` — race-clean
- [ ] `go build ./...` — compiles
- [ ] Manual: no config → pager bindings via `?`
- [ ] Manual: `dialect = "modern"` config → modern bindings via `?`
- [ ] Manual: `dialect = "garbage"` config → pager + `^l` warning
- [ ] Manual: malformed TOML → stderr line + pager + `^l` warning
- [ ] Manual: `g`/`G`/`^d`/`^u` in pager mode
- [ ] Manual: `Ctrl+Home`/`Ctrl+End`/`PageDown`/`PageUp` in modern mode

If anything fails, the per-task tests should have caught it — go back to the task whose verification was skipped or whose test was insufficient and add coverage.

Open a PR with `gh pr create` (the workflow uses merge commits, not squash, per CLAUDE.md). PR title: `feat: keybinding dialects (pager default, modern opt-in)`.

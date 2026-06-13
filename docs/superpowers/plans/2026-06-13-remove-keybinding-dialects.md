# Remove Keybinding Dialects Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse hypogeum's two keybinding dialects (`pager`/`modern`) into a single default keymap and remove the entire user-config machinery that existed only to select a dialect.

**Architecture:** This is a pure teardown — no new behavior. The surviving keymap is today's `pagerKeys()`, renamed `defaultKeys()`. The change is sequenced so the tree compiles and `go test ./...` stays green at every commit: (1) remove the modern keymap + dialect selection, (2) remove `Options`/`StartupWarnings` and simplify `New`'s signature + `main.go`, (3) delete the now-orphaned `internal/config` package, (4) docs.

**Tech Stack:** Go, Bubble Tea, `charmbracelet/bubbles/key`. No new dependencies; this removes one (`BurntSushi/toml` becomes unused — see Task 3).

**Spec:** `docs/superpowers/specs/2026-06-13-remove-keybinding-dialects-design.md`

**Conventions for every task:**
- This is a refactor: the existing test suite is the safety net. Each task makes its edits, then verifies `go build ./...` and `go test ./internal/tui/` (Task 3 also `go test ./...`). CI also runs `go vet ./...` and `go test -race ./...`.
- Do NOT run `git stash`/`checkout`/`restore`/`reset`.
- Keep gofmt tabs correct.

---

### Task 1: Collapse to a single default keymap

Remove the `modern` keymap and the dialect-selection switch. `Options` and `StartupWarnings` survive this task (removed in Task 2); `internal/config` still exists and is still imported by `main.go` (removed in Tasks 2–3). After this task the tree compiles and all tests pass.

**Files:**
- Modify: `internal/tui/keys.go`
- Modify: `internal/tui/model.go` (the `keys:` line only)
- Modify: `internal/tui/keys_test.go`
- Modify: `internal/tui/keyboard_select_test.go`
- Modify: `internal/tui/copypath_test.go`
- Delete: `internal/tui/dialect_modern_test.go`

- [ ] **Step 1: In `internal/tui/keys.go`, rename the factory and delete the dialect machinery.**

  - Rename `func pagerKeys() keyMap` → `func defaultKeys() keyMap` (body unchanged).
  - Delete the entire `func modernKeys() keyMap { ... }`.
  - Delete the entire `func keysFor(dialect string) keyMap { ... }`.
  - Remove the `"github.com/wilkes/hypogeum/internal/config"` import (only `keysFor` used it).

- [ ] **Step 2: In `internal/tui/model.go`, point the model at the single keymap.**

  Change the `Model` literal field in `New` from:
  ```go
  		keys:            keysFor(opts.Dialect),
  ```
  to:
  ```go
  		keys:            defaultKeys(),
  ```
  (Leave the `Options` struct, the `opts` parameter, and the `StartupWarnings` loop alone — Task 2 removes them.)

- [ ] **Step 3: Delete the modern-dialect test file.**

  ```bash
  git rm internal/tui/dialect_modern_test.go
  ```
  (Every test in it is modern-specific: `TestModel_BackForward_Modern`, `TestModel_OpenSearch_Modern`, `TestModel_OpenBacklinks_Modern`, `TestModel_OpenLogs_Modern`, `TestModel_LinkCycle_Modern`, `TestModel_QuitBothBindings_Modern`, `TestModel_GotoTop_Modern`, `TestModel_PageDown_Modern`, plus the `modernModel`/`modernModelTall` helpers.)

- [ ] **Step 4: In `internal/tui/copypath_test.go`, delete the modern-chord test.**

  Remove the entire `func TestModel_CopyPath_ModernChord(t *testing.T) { ... }` (the one that calls `sizedWithOptions(..., Options{Dialect: "modern"})` and presses `tea.KeyCtrlY`). Leave the pager copy-path test(s) in the file intact.

- [ ] **Step 5: In `internal/tui/keys_test.go`, prune and collapse the dialect tests.**

  - Delete `func TestModernKeys_AllActionsBound(t *testing.T) { ... }`.
  - Delete the `var modernZeroFields = map[string]bool{ ... }` declaration.
  - Delete `func TestKeysFor_Dispatch(t *testing.T) { ... }`.
  - Delete `func TestNew_OptionsSelectsDialect(t *testing.T) { ... }` (it asserts dialect-specific `Back` keys, which no longer vary).
  - Rename `func TestPagerKeys_AllActionsBound` → `func TestDefaultKeys_AllActionsBound`, and change its body's `km := pagerKeys()` to `km := defaultKeys()`.
  - In `TestKeys_NoOverlappingActions`, replace the dialect loop with a single keymap. Change:
    ```go
    	for _, dialect := range []string{"pager", "modern"} {
    		km := keysFor(dialect)
    		v := reflect.ValueOf(km)
    		...
    	}
    ```
    to operate once on `km := defaultKeys()` (drop the `for` loop and the `dialect` references in the error messages; keep the inner overlap-detection logic and the `isAllowedKeyOverlap` call exactly as-is).
  - In `TestKeys_HelpTextNonEmpty`, likewise replace the `for _, dialect := range []string{"pager", "modern"}` loop with a single `km := defaultKeys()` and drop `dialect` from the error message.
  - Leave `func isAllowedKeyOverlap(...)` unchanged — the `BeginSelect`/`ToggleFolder` Space whitelist and the `^j`/`^k` picker/search whitelist still apply to the default keymap.
  - Keep `TestNew_OptionsSurfacesStartupWarnings` for now (Task 2 deletes it).

- [ ] **Step 6: In `internal/tui/keyboard_select_test.go`, replace the modern visual test with an arrow-motion test.**

  Delete `func TestVisual_ModernDialectEntersAndYanks(t *testing.T) { ... }` (it constructs `New(root, "", Options{Dialect: "modern"})`) and add in its place:
  ```go
  func TestVisual_ArrowKeysMoveCaret(t *testing.T) {
  	root := writeFixture(t)
  	m := sized(t, root, "")
  	var copied string
  	m.copyToClipboard = func(s string) { copied = s }
  	m.setContent("hello world")

  	m = pressRune(t, m, 'v')                           // enter visual
  	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeySpace}) // anchor at {0,0}
  	for i := 0; i < 5; i++ {
  		m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRight}) // extend with plain arrows
  	}
  	m = pressRune(t, m, 'y') // yank

  	if copied != "hello" {
  		t.Errorf("arrow-key selection yank = %q, want %q", copied, "hello")
  	}
  }
  ```

- [ ] **Step 7: Verify build + tests are green.**

  Run: `go build ./... && go test ./internal/tui/`
  Expected: PASS. (`sizedWithOptions` is now unused but still defined — that's fine in Go; Task 2 deletes it. `internal/config` still compiles and `main.go` still imports it.)

- [ ] **Step 8: Commit.**

  ```bash
  git add -A
  git commit -m "refactor(tui): collapse keybindings to a single default keymap"
  ```

---

### Task 2: Remove `Options`, `StartupWarnings`, and the config wiring in `main.go`

Delete the now-vestigial `Options` struct, simplify `New`'s signature, and drop all config usage from `main.go`. After this task nothing imports `internal/config` (Task 3 deletes the package). Every `New(..., Options{})` test call site loses its argument.

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `cmd/hypogeum/main.go`
- Modify: `internal/tui/helpers_test.go`
- Modify: `internal/tui/keys_test.go`
- Modify: `internal/tui/*_test.go` (mechanical `Options{}` argument removal)

- [ ] **Step 1: In `internal/tui/model.go`, delete `Options` and simplify `New`.**

  - Delete the entire `type Options struct { ... }` (the block with `Dialect` and `StartupWarnings`).
  - Change the signature `func New(root, initialFile string, opts Options) (Model, error)` → `func New(root, initialFile string) (Model, error)`.
  - Delete the loop that consumes startup warnings:
    ```go
    	for _, w := range opts.StartupWarnings {
    		diag.Warn(w)
    	}
    ```
    (The surrounding `diag := newDiagnostics(...)` and the `vault.Build`/`recent.New` `diag.Warn` calls stay — they don't use `opts`.)

- [ ] **Step 2: In `cmd/hypogeum/main.go`, stop loading config.**

  - In `run`, replace:
    ```go
    	cfg, warnings := loadConfig()

    	model, err := tui.New(root, initialFile, tui.Options{
    		Dialect:         cfg.Dialect,
    		StartupWarnings: warnings,
    	})
    	if err != nil {
    		return err
    	}
    ```
    with:
    ```go
    	model, err := tui.New(root, initialFile)
    	if err != nil {
    		return err
    	}
    ```
  - Delete the entire `func loadConfig() (config.Config, []string) { ... }`.
  - Remove the `"github.com/wilkes/hypogeum/internal/config"` import. If `"fmt"` or `"os"` become unused after deleting `loadConfig`, remove them too; if they're still used elsewhere in the file, leave them. (Verify by building.)

- [ ] **Step 3: In `internal/tui/helpers_test.go`, simplify `sized` and delete `sizedWithOptions`.**

  - In `sized`, change `m, err := New(root, initialFile, Options{})` → `m, err := New(root, initialFile)`.
  - Delete the entire `func sizedWithOptions(t *testing.T, root, initialFile string, opts Options) Model { ... }` and its doc comment (it has no callers after Task 1).

- [ ] **Step 4: In `internal/tui/keys_test.go`, delete the StartupWarnings test.**

  Delete `func TestNew_OptionsSurfacesStartupWarnings(t *testing.T) { ... }` (it constructs `Options{Dialect: ..., StartupWarnings: ...}`, both now gone). Vault/recent warnings still reach the diagnostics ring via `diag.Warn` inside `New`; there is no longer a startup-warning injection point to test.

- [ ] **Step 5: Remove the `Options{}` argument from every remaining test call site.**

  Every remaining `New(...)` in tests passes a zero-value `Options{}` as the last argument. Replace `, Options{})` with `)` across the test files. The affected files and lines (zero-value cases only — the `Options{Dialect: ...}` cases all lived in tests deleted in Task 1 / Step 4):
  - `internal/tui/content_test.go` (5 sites)
  - `internal/tui/logs_test.go` (2)
  - `internal/tui/model_test.go` (5)
  - `internal/tui/help_test.go` (3)
  - `internal/tui/dispatch_test.go` (2: `New(dir, initFile, Options{})`, `New(dir, "", Options{})`)
  - `internal/tui/backlinks_test.go` (2)
  - `internal/tui/diagnostics_test.go` (1)
  - `internal/tui/view_test.go` (3)
  - `internal/tui/search_test.go` (6, incl. `New(dir, initial, Options{})`)
  - `internal/tui/picker_test.go` (1)

  A safe mechanical way (review the diff afterward):
  ```bash
  grep -rl ', Options{})' internal/tui/*_test.go | xargs sed -i '' 's/, Options{})/)/g'
  ```
  (On Linux `sed`, drop the `''` after `-i`.) Then confirm no stray `Options{` remains in tests: `grep -rn 'Options{' internal/tui/` should return nothing.

- [ ] **Step 6: Verify build + tests are green.**

  Run: `go build ./... && go test ./internal/tui/`
  Expected: PASS. `internal/config` still builds standalone (its own test still runs) but has no importers now.

- [ ] **Step 7: Commit.**

  ```bash
  git add -A
  git commit -m "refactor(tui): drop Options/StartupWarnings and the config wiring"
  ```

---

### Task 3: Delete the `internal/config` package

With no importers left, remove the package outright.

**Files:**
- Delete: `internal/config/config.go`
- Delete: `internal/config/config_test.go`

- [ ] **Step 1: Confirm there are no remaining importers.**

  Run: `grep -rn "internal/config" --include=*.go .`
  Expected: no output. If anything prints, STOP — Task 2 missed a reference; fix it before deleting.

- [ ] **Step 2: Remove the package.**

  ```bash
  git rm internal/config/config.go internal/config/config_test.go
  ```

- [ ] **Step 3: Check whether `BurntSushi/toml` is now unused and tidy modules.**

  Run:
  ```bash
  grep -rn "BurntSushi/toml" --include=*.go .
  go mod tidy
  ```
  If the grep returns nothing, `go mod tidy` will drop `github.com/BurntSushi/toml` from `go.mod`/`go.sum`. Stage whatever `go mod tidy` changes.

- [ ] **Step 4: Verify the whole module builds and tests pass.**

  Run: `go build ./... && go vet ./... && go test ./...`
  Expected: PASS across all packages (the `internal/config` package no longer appears).

- [ ] **Step 5: Commit.**

  ```bash
  git add -A
  git commit -m "refactor: delete internal/config (dialect was its only setting)"
  ```

---

### Task 4: Documentation

Update the prose surfaces to describe a single keymap and no config file.

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`
- Modify: `docs/index.md`
- Modify: `docs/superpowers/specs/2026-05-31-keybinding-dialects-design.md`

- [ ] **Step 1: CLAUDE.md — rewrite the dialect gotcha.**

  Replace the bullet that begins **"Keybindings come from a dialect factory, not a single default."** (the one describing `pagerKeys()`/`modernKeys()`/`keysFor`/TOML config) with:
  ```markdown
  - **Keybindings live in one place.** `internal/tui/keys.go` exposes a single `defaultKeys()` factory returning the `keyMap` the whole app uses; `tui.New` calls it directly. Adding an action means a new `keyMap` field, a binding in `defaultKeys()`, and dispatch wiring in `input.go` (which calls `key.Matches(msg, m.keys.X)`). There is no dialect system and no user config file.
  ```

- [ ] **Step 2: CLAUDE.md — drop dialect qualifiers from the summary and gotchas.**

  In the "## What this is" paragraph and the gotcha bullets, remove the `pager`/`modern` qualifiers and the config-file mention. Concretely:
  - Summary line: `` `^p` / `o` (pager; `^p`-only in modern) `` → `` `^p` / `o` ``; `` `t` (pager) / `^b` (modern) `` → `` `t` ``; `` `h`/`l` navigate browser-style history (pager dialect; modern uses `Alt+←/→`) `` → `` `h`/`l` navigate browser-style history ``; `` `y` (pager) / `^y` (modern) copies… `` → `` `y` copies… ``.
  - Keyboard-selection sentence: `` the dialect's copy key (`y`/`^y`) yanks `` → `` `y` yanks ``.
  - The "Modals swap" and "tree is a modal" gotchas: `` `t` (tree; `^b` in modern) `` → `` `t` (tree) ``; `` `t` (pager; `^b` in modern) `` → `` `t` ``.
  - The "`^p` (or `o` in pager) opens a flat recency-ranked finder" gotcha → "`^p` / `o` opens a flat recency-ranked finder".
  - Search gotcha mentioning `/` (pager) / `Ctrl+F` (modern): → just `/`. (Modern's `Ctrl+F` is gone.)

  Search CLAUDE.md for any remaining `modern`, `dialect`, or `config.toml` and remove/rephrase. (The keybinding-dialects design-doc link, if present, can point at the now-superseded spec.)

- [ ] **Step 3: README.md — fix the Keys table and delete the Configuration section.**

  - In "## Keys", delete the line `Pager dialect (default) — see [Configuration](#configuration) to switch to modern.`
  - Correct the table to the actual default keymap (it currently lists stale `^b`/`^p`). Use this table body:
    ```markdown
    | Key | Action |
    |-----|--------|
    | `↑` / `k`, `↓` / `j` | Move within the focused pane |
    | `Enter` | Open the selected file / follow selected link |
    | `h` / `←` | Back (collapse folder when tree modal is open) |
    | `l` / `→` | Forward (expand folder when tree modal is open) |
    | `n` / `N` | Cycle to next / previous link |
    | `v` | Start keyboard selection (then `Space` to anchor, motion to extend, `y` to copy) |
    | `y` | Copy current file path / yank selection (in visual mode) |
    | `Esc` | Clear link selection / cancel visual mode |
    | `b` | Open backlinks (modal) |
    | `t` | Open directory tree (modal) |
    | `^p` / `o` | Open file finder (type to fuzzy-filter; `^j`/`^k` cursor) |
    | `/` | Full-text search across vault markdown (type to search; `^j`/`^k` cursor) |
    | `^l` | Log viewer |
    | `?` | Help (cheat sheet) |
    | `q` | Quit |
    ```
  - Delete the entire "## Configuration" section: the intro paragraph, the per-OS path table, the "### Available settings" heading, the ```toml dialect example fenced block, and the trailing "Press `?` … active dialect … `Alt+l` in modern dialect" lines. (Stop at the next top-level section, "## Inspiration", which stays.)

- [ ] **Step 4: docs/index.md — supersede the dialect entry and add this teardown.**

  - Change the keybinding-dialects bullet (currently `- [Keybinding dialects](superpowers/specs/2026-05-31-keybinding-dialects-design.md) — two coherent presets…`) to begin `— superseded —` and note dialects were removed in favor of a single default keymap.
  - Add a new bullet:
    ```markdown
    - [Remove keybinding dialects](superpowers/specs/2026-06-13-remove-keybinding-dialects-design.md) — shipped — collapsed `pager`/`modern` into one `defaultKeys()` keymap and deleted the `internal/config` package and `Options`/`StartupWarnings`.
    ```

- [ ] **Step 5: Mark the dialect spec superseded.**

  At the top of `docs/superpowers/specs/2026-05-31-keybinding-dialects-design.md` (just under the H1), add:
  ```markdown
  **Status:** superseded — dialects removed 2026-06-13 in favor of a single default keymap. See [remove-keybinding-dialects](2026-06-13-remove-keybinding-dialects-design.md).
  ```

- [ ] **Step 6: Flip this teardown spec to shipped.**

  In `docs/superpowers/specs/2026-06-13-remove-keybinding-dialects-design.md`, change `**Status:** designed, not yet implemented.` → `**Status:** shipped.`

- [ ] **Step 7: Verify build/tests still green (docs-only, but confirm nothing else is dirty).**

  Run: `go build ./... && go test ./internal/tui/`
  Expected: PASS.

- [ ] **Step 8: Commit.**

  ```bash
  git add -A
  git commit -m "docs: describe the single default keymap; remove dialect/config docs"
  ```

---

## Self-review notes

- **Spec coverage:** keys.go collapse (Task 1), model.go `Options`/`New`/`StartupWarnings` (Task 2), main.go (Task 2), `internal/config` deletion (Task 3), test pruning + `sizedWithOptions` fold + arrow-motion replacement (Tasks 1–2), docs incl. README Keys-table correction and Configuration-section deletion (Task 4). Every spec section maps to a task.
- **Green at each commit:** Task 1 leaves `Options`/`config`/`main.go` intact (compiles); Task 2 removes `Options` and config *usage* but leaves the package standalone (compiles); Task 3 deletes the orphaned package; Task 4 is docs-only. No task leaves a dangling reference.
- **`go mod tidy`:** included in Task 3 to drop `BurntSushi/toml` if it becomes unused — guarded by a grep so it's only dropped when truly unreferenced.
- **No placeholders:** every code edit shows the before/after text or exact deletion target. The only non-literal step is the mechanical `, Options{})` → `)` sweep, which includes the exact command and a verifying grep.

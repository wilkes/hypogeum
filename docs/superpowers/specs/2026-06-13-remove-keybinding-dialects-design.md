# Remove keybinding dialects — single default keymap

**Status:** shipped.

## Goal

Simplify hypogeum's keybindings down to a single default set, removing the
two-dialect (`pager` / `modern`) system and the entire user-config machinery
that existed only to select a dialect. The surviving keymap is today's `pager`
keymap, renamed to the default.

This is a pure simplification — no new behavior. The default bindings are
exactly what a fresh install already used.

## Why

`internal/config` exists solely to load one TOML field (`dialect`); `modern`
is a parallel keymap that doubles the binding surface, the test matrix, and the
documentation. `Options.StartupWarnings` exists only to ferry config-load
warnings into the log modal. Removing the dialect removes all of it. Per YAGNI,
config can be reintroduced if and when a second real setting appears.

## Scope of removal (full teardown)

| Area | Change |
| --- | --- |
| `internal/tui/keys.go` | Rename `pagerKeys()` → `defaultKeys()`; delete `modernKeys()` and `keysFor()`; drop the `internal/config` import and `DialectModern`/`DialectPager` references. |
| `internal/tui/model.go` | Delete the `Options` struct; `New(root, initialFile string, opts Options)` → `New(root, initialFile string)`; remove the `StartupWarnings` loop; `keysFor(opts.Dialect)` → `defaultKeys()`. |
| `cmd/hypogeum/main.go` | Delete `loadConfig()`; call `tui.New(root, initialFile)`; drop the `config` import and the stderr/warning plumbing. |
| `internal/config/` | Delete the package (`config.go` + `config_test.go`). |
| Tests | Prune dialect tests; collapse dialect-iterating tests to one keymap; fold `sizedWithOptions` into `sized`; repurpose the modern visual test into an arrow-motion test. |
| Docs | CLAUDE.md, README.md, docs/index.md, the dialect spec. |

The `keyMap` struct and every action field are unchanged. Diagnostics from
`vault.Build` / `recent.New` already flow through `diag.Warn` *inside* `New`
(not via `StartupWarnings`), so removing `StartupWarnings` loses no warning
surface.

## Detailed changes

### keys.go

- `func defaultKeys() keyMap` returns the current `pagerKeys()` body verbatim.
- Delete `modernKeys()` and `func keysFor(dialect string) keyMap`.
- Remove `import ".../internal/config"`.

### model.go

- Remove the `Options` struct entirely.
- `func New(root, initialFile string) (Model, error)`.
- Delete the `for _, w := range opts.StartupWarnings { diag.Warn(w) }` loop.
- `keys: defaultKeys()` in the `Model` literal (was `keysFor(opts.Dialect)`).

### main.go

- Delete `loadConfig()` and the `config` import.
- The construction site becomes:
  ```go
  model, err := tui.New(root, initialFile)
  if err != nil {
      return err
  }
  ```
- No config path resolution, no stderr warning, no `StartupWarnings`.

### internal/config

- `git rm internal/config/config.go internal/config/config_test.go`. The
  package has no other consumers once main.go stops importing it.

## Tests

- **`internal/tui/keys_test.go`:**
  - Delete `TestModernKeys_AllActionsBound`, `TestKeysFor_Dispatch`,
    `TestNew_OptionsSelectsDialect`, `TestNew_OptionsSurfacesStartupWarnings`,
    and the `modernZeroFields` map.
  - Rename `TestPagerKeys_AllActionsBound` → `TestDefaultKeys_AllActionsBound`,
    calling `defaultKeys()`.
  - `TestKeys_NoOverlappingActions` and `TestKeys_HelpTextNonEmpty`: drop the
    `for _, dialect := range []string{"pager","modern"}` loop; test
    `defaultKeys()` once. Keep `isAllowedKeyOverlap` (the `Space`
    BeginSelect/ToggleFolder and `^j`/`^k` picker/search whitelists still apply).
- **`internal/tui/helpers_test.go`:** `sized()` calls `New(root, initialFile)`;
  delete `sizedWithOptions`.
- **`internal/tui/keyboard_select_test.go`:** replace
  `TestVisual_ModernDialectEntersAndYanks` with `TestVisual_ArrowKeysMoveCaret`
  — build a model via `sized(t, root, "")`, `setContent("hello world")`, then
  `v`, `Space`, five `tea.KeyRight` presses, then `y`; assert the clipboard
  holds `"hello"`. This preserves arrow-key motion coverage that the modern
  test used to provide.
- **All remaining `New(root, x, Options{...})` / `Options{}` call sites:** drop
  the `Options` argument. Most route through `sized`, so the churn is
  concentrated in that helper plus the deleted dialect tests.

## Docs

- **CLAUDE.md:**
  - Rewrite the "Keybindings come from a dialect factory" gotcha into a short
    note: a single `defaultKeys()` in `internal/tui/keys.go` is the source of
    truth; adding an action means a new `keyMap` field + a binding + dispatch
    wiring.
  - Summary paragraph and per-feature lines: drop `pager`/`modern` qualifiers
    and the `~/.config/hypogeum/config.toml` mention. Concretely: `y` (pager) /
    `^y` (modern) → `y`; `t` (pager) / `^b` (modern) tree → `t`; `^p` / `o`
    (pager; `^p`-only in modern) → `^p` / `o`; the history line `h`/`l` (pager
    dialect; modern uses `Alt+←/→`) → just `h`/`l`; the keyboard-selection
    sentence's "dialect's copy key (`y`/`^y`)" → `y`.
- **README.md:**
  - "Keys" section: remove the "Pager dialect (default) — see Configuration to
    switch to modern." line. Correct the table to the actual default keymap
    (it is currently stale — predates the `t`/`o` rebind): tree is `t` (not
    `^b`), finder is `^p` / `o`, and add the missing `y` (copy path) and
    `v` / `Space` (select mode) rows.
  - Delete the entire "## Configuration" section (the config file is no longer
    read) and the trailing "Press `?` … active dialect … `Alt+l` in modern
    dialect" lines.
- **docs/index.md:** mark the keybinding-dialects entry **superseded —
  dialects removed in favor of a single default keymap**, and add a new entry
  linking this teardown spec.
- **dialect spec** (`superpowers/specs/2026-05-31-keybinding-dialects-design.md`):
  add `**Status:** superseded — dialects removed (2026-06-13); single default
  keymap.` near the top.

## Verification

- `go build ./...`, `go vet ./...`, `go test -race ./...` all green.
- `grep -rn "dialect\|Dialect\|modernKeys\|keysFor\|internal/config\|StartupWarnings\|sizedWithOptions"` over non-superseded files returns nothing (the only `dialect` mentions left are the superseded spec and the diary, which are historical).
- `hypogeum --version` still works; the binary starts and renders with the default keymap and ignores any pre-existing `config.toml`.

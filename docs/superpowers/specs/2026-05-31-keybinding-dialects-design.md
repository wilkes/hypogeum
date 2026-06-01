# Keybinding dialects

Two coherent keybinding presets — **pager** (default, vim/less idioms) and **modern** (VS Code/browser idioms) — selectable via a forward-growable TOML config file. No per-binding overrides in v1.

## Background

Hypogeum's bindings today are a fusion of three families: VS Code (`^p` picker, `^b` tree), vim (`h`/`l` history, `n`/`p` link cycling, `j`/`k` motion), and Emacs (`^s` search, `^l` logs). The set is functional but doesn't speak one dialect — users who type fluently in one family hit friction with the parts borrowed from another. There is no configuration system today; all bindings come from `defaultKeys()` in `internal/tui/keys.go`.

The CLAUDE.md-documented invariants we will preserve:

- Single-modal-swap rule; `?` anchored; `Esc` cascade.
- Tree-modal arrow-key shadow (`←`/`→` collapse/expand inside the tree modal instead of stepping history). Both dialects keep `←`/`→` bound to Back/Forward globally, so the shadow logic stays identical.
- `formatHelp(m.keys)` auto-renders the `?` cheat sheet from the live keymap. The dialect selection gets help-text updates for free.

## Decision

Add a TOML config file at the OS-canonical user-config path with a single key, `dialect`, that selects between two keymap factories. Default is `pager`; `modern` is the opt-in alternative. The schema is designed to grow (other future settings like wrap width or theme), but per-binding overrides are explicitly out of scope for v1.

### Why config file over flag/env-var

- A flag (`--dialect modern`) is the smallest mechanism but requires retyping or aliasing for every invocation.
- An env var (`HYPOGEUM_KEYS=modern`) is session-sticky but less discoverable.
- A config file persists across invocations, is the conventional Unix surface for personal preference, and gives us a place to land non-binding settings later.

A `--dialect` CLI flag is a reasonable future addition for one-off overrides, but is not in v1.

### Why dialect-only over per-binding overrides

Per-binding overrides require a binding-name vocabulary, validation, conflict detection, and a way to express "disable this." Shipping the two presets first proves the dialect-switch mechanism with minimal new surface area. If users ask for individual remapping, the override layer can land on top of `keysFor()` without changing the existing factories.

## Binding tables

### Pager (default)

| Action | Binding |
|---|---|
| Cursor down | `j` / `↓` |
| Cursor up | `k` / `↑` |
| Open / follow link | `Enter` |
| History back | `h` / `←` |
| History forward | `l` / `→` |
| Quit | `q` / `Ctrl+c` |
| Search modal | `/` |
| Next link | `n` |
| Prev link | `N` |
| Clear link / close modal | `Esc` |
| Backlinks modal | `b` |
| Logs modal | `Ctrl+l` |
| Help modal | `?` |
| Tree modal | `Ctrl+b` |
| Folder toggle (in tree) | `Space` |
| File picker | `Ctrl+p` |
| Picker cursor down/up | `Ctrl+j` / `Ctrl+k` |
| Search-modal cursor down/up | `Ctrl+j` / `Ctrl+k` |
| Top of doc | `g` |
| Bottom of doc | `G` |
| Half page down | `Ctrl+d` |
| Half page up | `Ctrl+u` |

### Modern (opt-in)

| Action | Binding |
|---|---|
| Cursor down | `↓` |
| Cursor up | `↑` |
| Open / follow link | `Enter` |
| History back | `Alt+←` / `Backspace` |
| History forward | `Alt+→` |
| Quit | `q` / `Ctrl+q` / `Ctrl+c` |
| Search modal | `Ctrl+f` |
| Next link | `Tab` |
| Prev link | `Shift+Tab` |
| Clear link / close modal | `Esc` |
| Backlinks modal | `Alt+b` |
| Logs modal | `Alt+l` |
| Help modal | `?` / `F1` |
| Tree modal | `Ctrl+b` |
| Folder toggle (in tree) | `Space` |
| File picker | `Ctrl+p` |
| Picker cursor down/up | `↓` / `↑` |
| Search-modal cursor down/up | `↓` / `↑` |
| Top of doc | `Ctrl+Home` |
| Bottom of doc | `Ctrl+End` |
| Half page down | `PageDown` |
| Half page up | `PageUp` |

### Deltas from today's defaults (even in pager mode)

- `g` / `G` (top/bottom) and `Ctrl+u` / `Ctrl+d` (half-page) are net new. We use the `less` idiom (single `g` for top) rather than vim's `gg`, to avoid a two-key state machine.
- `N` (Shift+n) for previous link, freeing bare `p` (was previous-link). `p` is now unbound; this aligns pager mode with vim's search-next/prev convention (`n`/`N`).
- `q` and `Ctrl+c` already quit today; unchanged in pager.

### What stays identical in both dialects

`Ctrl+P` picker, `Ctrl+B` tree, `Enter`, `Esc`, `?` help, `Space` folder toggle, mouse. The `Ctrl+P` and `Ctrl+B` choices are universal enough across all editor families that switching them by dialect would only hurt.

### Picker/search arrow keys in both dialects

In pager mode, `Ctrl+j` / `Ctrl+k` provide cursor navigation inside the picker (where `j`/`k` go to the textinput). In modern mode, those bindings are left as zero-value `key.Binding{}`. The existing dispatcher in `internal/tui/input.go:210-218` matches both `m.keys.Up` and `m.keys.PickerCursorUp` with `||`, so when `PickerCursorUp` is empty the fallthrough to `Up` (`↑`) gives modern mode arrow-key picker navigation with no dispatch-code changes.

## Architecture

### New package: `internal/config/`

Pure, no TUI dependencies. Matches the layering rule (`tui` depends on lower layers; lower layers don't know about `tui`).

```go
package config

type Config struct {
    Dialect string // "pager" | "modern"
}

func Default() Config { return Config{Dialect: "pager"} }

// DefaultPath returns the per-OS expected config location via os.UserConfigDir.
// Empty path + error if the OS doesn't expose one (extremely rare).
func DefaultPath() (string, error)

// Load reads and validates a config file. Missing file is not an error.
// Returns the loaded config, a slice of non-fatal warnings the caller can
// surface (e.g. "unknown dialect, falling back to pager"), and an error
// only if the file existed but couldn't be parsed.
func Load(path string) (Config, []string, error)
```

### Keymap factories: `internal/tui/keys.go`

```go
func pagerKeys() keyMap  // renamed from defaultKeys(); same content + new actions
func modernKeys() keyMap // new

func keysFor(dialect string) keyMap {
    switch dialect {
    case "modern":
        return modernKeys()
    default:
        return pagerKeys()
    }
}
```

`defaultKeys()` is renamed to `pagerKeys()`. The rename touches two lines today (the definition in `keys.go:35` and the sole call site in `model.go:174`). New fields on `keyMap`:

```go
Top          key.Binding
Bottom       key.Binding
HalfPageDown key.Binding
HalfPageUp   key.Binding
```

These dispatch into `m.content.vp.GotoTop()`, `GotoBottom()`, `HalfViewUp()`, `HalfViewDown()` — methods already on bubbles' `viewport.Model`.

### TUI options: `internal/tui/model.go`

```go
type Options struct {
    Dialect         string
    StartupWarnings []string // surfaced into the in-app log modal at construction
}

func New(root, initialFile string, opts Options) (Model, error)
```

Inside `New`: `m.keys = keysFor(opts.Dialect)`. Warnings are pushed into the existing log buffer (`internal/tui/logs.go`) so they show up via `^l` (or `Ctrl+Shift+l` in modern).

### Main wiring: `cmd/hypogeum/main.go`

```go
cfgPath, _ := config.DefaultPath()
cfg, warnings, err := config.Load(cfgPath)
if err != nil {
    fmt.Fprintf(os.Stderr, "hypogeum: %s: %v (using defaults)\n", cfgPath, err)
    cfg = config.Default()
}
model, err := tui.New(root, initialFile, tui.Options{
    Dialect:         cfg.Dialect,
    StartupWarnings: warnings,
})
```

### Data flow

```
$XDG_CONFIG_HOME/hypogeum/config.toml
        │
        ▼
config.Load() ─────► Config{Dialect: "modern"}
        │                     │
        │                     ▼
        │            tui.New(..., tui.Options{Dialect, StartupWarnings})
        │                     │
        │                     ▼
        │            m.keys = keysFor("modern")
        │                     │
        │                     ▼
        │            input.go: key.Matches(msg, m.keys.X) — unchanged
        │
        └───────► startup warnings ─► m.logs (visible via ^l / ^Shift+l)
```

### Dependency footprint

One new module: `github.com/BurntSushi/toml`. Pure-Go, MIT, no cgo, no transitive deps, ~2k LOC. Adds approximately 60 KB to the release binary.

### Config file format

```toml
# ~/.config/hypogeum/config.toml (Linux)
# ~/Library/Application Support/hypogeum/config.toml (macOS)
# %AppData%\hypogeum\config.toml (Windows)

dialect = "pager"  # or "modern"
```

Future settings (out of scope for v1; documented to anchor the forward-growth shape):

```toml
dialect = "modern"
wrap_width = 120          # future
theme = "dark"            # future
vault_root = "~/notes"    # future
```

## Error handling

Matches the watcher/vault best-effort pattern. Hypogeum never refuses to start because of config.

| Condition | Behavior |
|---|---|
| Missing config file | Silent. Use `Default()`. |
| Unreadable file (permissions) | One-line stderr write, use defaults. |
| Malformed TOML (parse error) | Stderr line with file path + TOML decoder's line/column, use defaults. Also pushed into `m.logs` as a warning so `^l` shows it post-startup. |
| Unknown `dialect` value | Push warning into `m.logs` naming valid options. Fall back to pager. |
| `os.UserConfigDir()` returns error | Skip config load entirely, use defaults. |

Stderr writes happen *before* `tea.NewProgram` runs, so the message is visible in the parent shell window briefly before the alt-screen takes over, and again after the alt-screen restores on exit. The in-app log modal is the persistent surface during a session.

## Testing strategy

### `internal/config/config_test.go` (new)

Pure-package, table-driven:

| Test | Scenario |
|---|---|
| `TestLoad_Missing` | File path doesn't exist → `Default()`, no warnings, nil error |
| `TestLoad_ValidPager` | `dialect = "pager"` → `Config{Dialect: "pager"}`, no warnings |
| `TestLoad_ValidModern` | `dialect = "modern"` → `Config{Dialect: "modern"}`, no warnings |
| `TestLoad_DefaultDialect` | Empty file → `Default()`, no warnings |
| `TestLoad_UnknownDialect` | `dialect = "vim"` → pager fallback, one warning naming valid options |
| `TestLoad_MalformedTOML` | `dialect = =` → returns error |
| `TestLoad_UnreadablePerm` | `os.Chmod(path, 0)` then load → returns error |
| `TestDefaultPath` | Asserts non-empty path returned on the host OS |

Fixtures use `t.TempDir()`. No committed config files in the repo.

### `internal/tui/keys_test.go` (new)

| Test | Asserts |
|---|---|
| `TestPagerKeys_AllActionsBound` | Every field on pager's keyMap is non-zero |
| `TestModernKeys_AllActionsBound` | Same for modern, with explicit allowlist of fields intentionally left zero (the cursor-key fields for picker/search) |
| `TestKeysFor_Dispatch` | `keysFor("modern")` → modern; `keysFor("pager")`, `keysFor("")`, `keysFor("garbage")` → pager |
| `TestKeys_HelpTextNonEmpty` | Every bound binding has a non-empty `Help().Desc` so `?` doesn't render blank rows |
| `TestKeys_NoOverlappingActions` | Within a dialect, no two distinct actions share an identical key spec (catches accidental rebind collisions) |

### Existing tests reproduced under modern dialect

A helper `newTestModel(t, opts)` is added; existing pager tests stay unchanged, and a parallel modern-dialect test is added next to each binding-sensitive test:

- `TestModel_BackForward_Modern` — `Alt+←` / `Alt+→`
- `TestModel_OpenSearch_Modern` — `Ctrl+Shift+f`
- `TestModel_LinkCycle_Modern` — `Tab` / `Shift+Tab`
- `TestModel_QuitBothBindings_Modern` — both `q` and `Ctrl+q`

### New-action tests (both dialects)

| Test | Asserts |
|---|---|
| `TestModel_GotoTop_Pager` / `_Modern` | `g` / `Ctrl+Home` scroll viewport to top (`vp.YOffset == 0`) |
| `TestModel_GotoBottom_Pager` / `_Modern` | `G` / `Ctrl+End` scroll to bottom |
| `TestModel_HalfPage_Pager` / `_Modern` | `Ctrl+d` / `PageDown` advance roughly half the viewport (`vp.Height/2 ± 1`) |

### Regression test preserved

`TestModel_ArrowKeysShadowHistoryWhileTreeModalOpen` continues to pass untouched; both dialects keep `←`/`→` bound to Back/Forward globally, so the tree-modal shadow logic is dialect-independent.

### Race cleanliness

Config load happens once at startup, before `tea.NewProgram` runs. No goroutines, no shared state. The existing `go test -race ./...` CI step is unaffected.

## Scope

In:

- New `internal/config/` package with `Config`, `Default`, `DefaultPath`, `Load`.
- Rename `defaultKeys()` → `pagerKeys()`; add `modernKeys()`, `keysFor()`.
- New `keyMap` fields: `Top`, `Bottom`, `HalfPageDown`, `HalfPageUp`.
- Dispatch wiring for the four new actions in `internal/tui/input.go`.
- `tui.Options` struct on `tui.New`.
- Config loading in `cmd/hypogeum/main.go`.
- `github.com/BurntSushi/toml` dependency.
- Tests as enumerated above.
- `CLAUDE.md` — add a Gotcha entry for the dialect system; reference the new `internal/config` package in the package layering paragraph.
- `docs/index.md` — entry pointing at this spec.

Out:

- Per-binding overrides.
- `--dialect` CLI flag.
- `HYPOGEUM_KEYS` env var.
- Theme, wrap-width, vault-root, or any non-binding config keys (schema accommodates them but they aren't implemented).
- Two-key chord support (e.g. vim's `gg`). Single `g` is sufficient for top-of-doc.
- Runtime dialect switching (a `:dialect modern` command, etc.) — restart-only.
- Migration of any existing on-disk state; there isn't any.

## Verification

- `go test ./...` — full suite stays green; new tests pass.
- `go test -race ./...` — race-clean.
- `go build ./...` — compiles on all CI targets (darwin/linux × amd64/arm64).
- Manual: launch hypogeum with no config (assert pager bindings via `?` help). Write `dialect = "modern"` to the config path and relaunch (assert modern bindings via `?`). Write `dialect = "garbage"` and relaunch (assert pager fallback + `^l` shows a warning). Write malformed TOML (assert stderr message + pager fallback). Try `g`/`G`/`^d`/`^u` in pager mode and `Ctrl+Home`/`Ctrl+End`/`PageDown`/`PageUp` in modern.

## Risks

- **Terminal variance for chord keys.** Most terminal protocols can't transmit `Ctrl+Shift+letter` distinctly from `Ctrl+letter` — the byte streams are identical. The spec deliberately avoids `Ctrl+Shift+letter` chords and uses `Alt+letter` instead (which encodes reliably as `ESC <letter>` across emulators). `Alt+←`/`Alt+→`, `Ctrl+Home`/`Ctrl+End`, and `F1` are reliable on xterm-family terminals but vary on legacy ones; modern mode mitigates with non-chord fallbacks where it can (`Backspace` for back, `?` for help). A CLAUDE.md note documents the limitation.
- **Muscle memory for the `p` → previous-link removal.** Users currently using `p` to step backward through links will hit a no-op. Acceptable cost — `N` is the vim-idiomatic prev key, and the help cheat sheet documents it. We can add a deprecation log message ("`p` is no longer prev-link; use `N`") if the change generates feedback, but the spec doesn't ship that.
- **Config file path on macOS.** `os.UserConfigDir()` returns `~/Library/Application Support/...` on macOS, which is correct but unfamiliar — Unix users sometimes expect `~/.config/`. We document the path in the README and CLAUDE.md.
- **TOML lib pinning.** `BurntSushi/toml` is at v1.x and stable, but any new dep is a dep. Acceptable for the value delivered. We won't pin to a specific minor version; standard `go.mod` resolution is sufficient.

## Future work enabled

- Per-binding overrides under a `[bindings]` table.
- `--dialect modern` CLI flag for one-off override.
- A third dialect (e.g. `helix`, `emacs`) lands as a new factory + one case in `keysFor()`.
- Theme, wrap-width, vault-root config keys all live in the same file.

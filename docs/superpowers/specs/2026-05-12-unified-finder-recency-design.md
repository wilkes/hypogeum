# Unified finder with recency — design

**Status:** shipped on 2026-05-12.
**Scope:** replace the tree-rooted `^p` picker with a flat, recency-ranked finder over every markdown file in the vault. Introduce a new `internal/recent` package that owns the hybrid mtime + visit-history score and the persisted visits state file. First cut is foundation + recency only; name-filter and other sort modes are explicitly deferred.
**Out of scope:** full-text content search, fuzzy name-filter typing inside the picker, sort-mode cycling, updated-together view, configurable decay constants, per-vault state files, Windows support.

See also: [docs index](../../index.md), [architecture](../../architecture.md), existing picker spec [vault-rooted-picker-design](2026-05-07-vault-rooted-picker-design.md), [`internal/tui`](../../packages/tui.md).

## Motivation

The user reads their own notes vault with hypogeum. The friction they hit most often is *finding* a note — not by spatial location in the tree, but by what they were just working on. Two signals are available and cheap: filesystem mtime (what changed lately) and visit history (what I've been reading). Neither is exposed today.

The existing `^p` picker mirrors the left-pane tree shape: collapsible folders, expansion state, tree-walk rendering. It's useful for spatial browsing but it's the wrong shape for the actual question users ask. "What was I just on?" doesn't map to a folder hierarchy.

Replacing `^p` rather than adding a parallel key is a deliberate choice: it commits to the unified-finder direction. Future modes (content search, updated-together) plug into the same surface rather than each spawning its own keybinding. The flat-list shape is also a strictly simpler renderer than the tree-aware version it replaces.

## Architecture

A new `internal/recent` package becomes a peer of `tree`, `markdown`, `nav`, `watch`, `vault`. It owns the visits store, scoring function, and ranked-list query. It depends on the standard library only; no Bubble Tea, no Glamour, no `tree.Node` awareness.

```
cmd/hypogeum
        │
        ▼
internal/tui                 (still the only package that knows about the UI)
   │      │      │      │      │      │
   ▼      ▼      ▼      ▼      ▼      ▼
   tree   markdown   nav   watch   vault   recent   (lower layers, mutually independent)
```

The dependency edge is `tui → recent` only. `recent` takes `[]string` of absolute paths and returns ordered `Ranked` values; it does not know how those paths were produced or what they mean. This keeps it unit-testable and means later scoring needs (e.g. content-search ranking) can reuse the same package without coupling.

The existing tree-rooted picker (`internal/tui/picker.go`, 169 lines) is replaced. The new picker is a flat viewport over `[]recent.Ranked` — one cursor int, no expansion map, no tree walk on render.

## Components

### `internal/recent` (new)

```
recent.go    Store struct, New, Record, Rank, scoring math, persistence
```

Public surface:

```go
package recent

// Store holds the persisted visit history and exposes scoring + ranking.
type Store struct { /* private */ }

// New loads visits from the state file (if it exists) and returns a Store
// ready to use. If the state file is missing, malformed, or unreadable,
// returns a Store with empty visits and a non-nil error describing what
// went wrong — the caller decides whether to surface it as a diagnostic.
// The Store still works fine with no prior history.
func New(stateFile string) (*Store, error)

// Record marks path as visited now. Writes through to the state file
// synchronously via atomic temp-file + rename. Returns an error from
// the write; callers may surface it as a diagnostic. The in-memory
// update succeeds even if the write fails, so the current session
// keeps working.
func (s *Store) Record(path string) error

// Rank returns paths sorted by hybrid score (descending). It calls
// os.Stat on each path to read mtime; entries whose stat fails are
// dropped silently from the result. mtime is not cached across calls
// because the watcher may have updated files since the last Rank.
func (s *Store) Rank(paths []string) []Ranked

// Ranked carries one entry of the ordered result.
type Ranked struct {
    Path  string
    Score float64
    MTime time.Time   // exposed so the picker can render "2h ago"
    Visit time.Time   // zero if never visited
}

// DefaultStateFile returns the per-user state file path:
//   - Linux: ~/.config/hypogeum/visits.json
//   - macOS: ~/Library/Application Support/hypogeum/visits.json
// Implemented via os.UserConfigDir(); returns an error only if no home
// directory can be determined.
func DefaultStateFile() (string, error)
```

Internal state:

```go
type Store struct {
    stateFile string
    visits    map[string]time.Time  // abs path → last visit
    mu        sync.Mutex
    nowFunc   func() time.Time      // testNow hook, defaults to time.Now
}
```

### Scoring

Two exponential-decay terms summed:

```
score(path) = exp(-Δt_mtime / H_mtime) + W_visit · exp(-Δt_visit / H_visit)
```

where:
- `Δt_mtime` = (now − mtime) in hours.
- `Δt_visit` = (now − last visit) in hours; if never visited, the visit term is 0.
- `H_mtime = 168.0` (7-day half-life on edits).
- `H_visit = 48.0` (2-day half-life on visits).
- `W_visit = 1.5` (visits weighted slightly more than edits — they reflect deliberate attention).

These are package-level constants. Tweaking after dogfooding is a one-line change; configuration is YAGNI.

Exponential decay rather than linear because there's no cliff — no "30-day-old file suddenly disappears." The list shifts gently as time passes.

### Persistence format

`visits.json` is a single JSON object:

```json
{
  "version": 1,
  "visits": {
    "/abs/path/to/note.md": "2026-05-12T14:23:11Z",
    "/abs/path/to/other.md": "2026-05-11T09:14:02Z"
  }
}
```

- **Atomic write:** write to `visits.json.tmp` then `os.Rename` over `visits.json`. Survives crash mid-write.
- **Size:** ~80 bytes per entry. 10K-entry vault ≈ 800KB. Load and write are single ReadFile/WriteFile + Marshal/Unmarshal; sub-millisecond either way.
- **No eviction in v1.** Orphan entries (files renamed or deleted externally) stay in the map but `Rank` drops them when `os.Stat` fails — they're invisible to the user. Decay handles freshness naturally.
- **Version field** is a forward-compat hook. Unknown version → `New` returns an error; caller surfaces a diagnostic, Store starts empty.

### `internal/tui` (changes)

**Picker state, replacing the current expansion-aware struct:**

```go
type pickerState struct {
    open    bool
    ranked  []recent.Ranked   // populated on open; not refreshed mid-modal
    cursor  int               // index into ranked
    vp      viewport.Model    // scrolls the flat list
}
```

The existing `m.modals.picker.expanded` map and all tree-walk picker rendering are deleted alongside the new code.

**New Model field:**

```go
recent *recent.Store         // injected at New time
```

If `recent.New` returns an error, the Store is non-nil and usable (empty visits); the error is surfaced via `diag.Warn`. If `DefaultStateFile` itself fails (no home dir), `m.recent` is set to a no-op stub that returns empty `Rank` results and silently discards `Record` calls — same graceful-degradation pattern as `m.watcher == nil`.

**Opening the picker (`^p`):**

```go
func (m *Model) openPicker() {
    paths := m.allVaultMarkdownPaths()  // walks m.rootNode once
    m.picker.ranked = m.recent.Rank(paths)
    m.picker.cursor = 0
    m.picker.open = true
    m.refreshPickerVP()
}
```

`allVaultMarkdownPaths` is a small Model method that flattens `m.rootNode` to a `[]string`. Putting it on Model rather than in `recent` preserves `recent`'s ignorance of `*tree.Node`.

**Recording visits — one line in `openFile`, after history is updated:**

```go
if err := m.recent.Record(path); err != nil {
    m.diag.Warn(fmt.Sprintf("recent: %v", err))
}
```

**Row rendering:**

```
notes/projects/hypogeum.md          2h ago
notes/journal/2026-05-10.md         yesterday
notes/inbox/scratch.md              3d ago · edited
docs/index.md                       6d ago
```

- **Left:** path relative to the vault root.
- **Right:** human-friendly recency, right-aligned. Uses the more-recent of mtime or visit time. If mtime is the more-recent signal, append `· edited`; otherwise the recency is a visit (no suffix).
- **Truncation:** if width is tight, truncate the path with a leading ellipsis (`…ingprojects/hypogeum.md`) so the basename stays visible.
- **Selected row:** reverse-video, matching tree-pane cursor style.
- **No score displayed.** A code comment on the row formatter says so, to pre-empt the "let me show the score for debug" temptation.

### Keys inside the picker

```
j / ↓      cursor down
k / ↑      cursor up
Enter      open the selected file (closes picker, calls m.openFile)
Esc        close picker, no action
^p         close picker (toggle)
```

Everything else is dropped while the picker has focus. Modal-swap invariants from [[modal-geometry]] continue to apply — `B` or `^l` while the picker is open swap to those modals; `?` while the picker is open is a no-op (the cheat sheet doesn't steal mid-task focus).

## Data flow

**Startup:**
1. `tree.Walk(root)` produces the tree (existing).
2. `vault.Build(root)` builds the wikilink/backlink indexes (existing).
3. `stateFile, _ := recent.DefaultStateFile()`; `recent.New(stateFile)` loads the visits map. Errors → `diag.Warn`, Store starts empty.
4. `tui.New` stores the Store on the Model.

**File open (any path: Enter on tree, link follow, backlink follow, history, picker):**
1. `m.history.Visit(path)` (existing).
2. `m.recent.Record(path)` — in-memory map updated, JSON rewritten atomically. Errors → `diag.Warn`.
3. `m.refreshContent(path)` (existing).

**Picker open (`^p`):**
1. `paths := m.allVaultMarkdownPaths()` walks the in-memory `m.rootNode`.
2. `m.recent.Rank(paths)` calls `os.Stat` per file, computes the score, returns sorted `[]Ranked`. Entries whose stat fails are dropped.
3. Picker viewport renders the rows; cursor starts at index 0 (highest-scoring file).

**Picker selection:**
1. `Enter` → `m.picker.open = false`; `m.openFile(ranked[cursor].Path)`.
2. `openFile` then calls `m.recent.Record` as it does for any path open (tree, link, backlink, history). Opening from the picker bumps the file's score for next time — there's no separate code path.

## Error handling

| Failure | Behavior |
|---|---|
| `~/.config/hypogeum/` doesn't exist | `recent.New` creates it (`os.MkdirAll`). Creation failure → error returned; Store still works in-memory; subsequent `Record` calls also fail. Surfaces once via `diag.Warn`. |
| State file missing | Empty Store, no error. Picker shows files sorted by mtime alone. |
| State file unreadable or malformed JSON | Empty Store, error returned. `diag.Warn`. Next successful `Record` rewrites the file. |
| State file version unknown | Treated as malformed. Same behavior. |
| `Record` write fails (disk full, permissions) | In-memory update succeeds, error returned. `diag.Warn`. The transient footer naturally dedups visible-burst dupes. |
| `os.Stat` fails in `Rank` (file deleted externally) | Drop that path silently from the result. Vault rebuild on the next watcher event will reconcile. |
| Vault has zero markdown files | `Rank` returns empty slice; picker opens but is empty; `Enter` is a no-op. |
| Two `hypogeum` instances running | Last writer wins on `visits.json`. Atomic rename guarantees the file is never half-written. Documented in `recent` package godoc; not solved in code. |
| Vault root changes between runs | `visits.json` is keyed by absolute path; files from a different vault don't appear in the current vault's `Rank` because their paths aren't in `paths`. Orphan entries decay naturally. |

## Testing

`internal/recent/recent_test.go`:
- Round-trip: `New` → `Record(p1)` → `Record(p2)` → re-`New` from same file → `Rank` returns p1, p2 in expected order.
- Decay shape: with frozen `now`, a 1-hour-old visit outranks a 1-day-old visit; a 1-day-old visit outranks an 8-day-old edit.
- Missing-file paths drop from `Rank` output.
- Malformed JSON returns an error from `New` but Store still works.
- Unknown version field returns an error from `New`.
- Atomic write: simulate a partial write (write `.tmp` then verify the real file is unchanged from prior).
- Visit weighting: a recently-visited but old-mtime file beats a recently-edited but never-visited file at equal Δt.
- Empty path list to `Rank` returns empty slice.

`internal/tui/picker_test.go` (rewritten from current):
- Opening `^p` populates `m.picker.ranked` with all vault markdown.
- `j`/`k` move the cursor; bounds are respected; `g`/`G` jump to ends.
- `Enter` calls `openFile` with the selected path and closes the picker.
- `Esc` closes without opening anything.
- `^p` while open toggles close.
- Recently-visited file appears above never-visited file with same mtime.
- Recently-edited file appears above never-edited file with same visit time.
- Empty vault → picker opens but the list is empty; `Enter` is a no-op.
- Modal-swap: `B` while picker is open swaps to backlinks modal; `^l` swaps to logs; `?` is a no-op.

`internal/tui/dispatch_test.go`:
- `openFile` calls `recent.Record`; a record failure surfaces as a `diag.Warn` but does not block the open.

Existing tests must continue to pass — no regressions to tree-pane, history, link-following, or backlinks behavior.

## Documentation updates

- `README.md`: update the `^p` row in the keys table from "file picker (modal)" to "file finder (recent first)".
- `CLAUDE.md`: update the `^p` paragraph under "Gotchas" — the picker is no longer tree-shaped, no expansion state, flat list of all vault markdown ranked by hybrid score.
- `docs/index.md`: add an entry under "Active feature work" pointing at this spec.
- `docs/packages/tui.md`: replace the picker section with a description of the flat finder. Cross-reference `internal/recent`.
- `docs/concepts/`: no new concept doc in this phase. If content-search lands later, a `ranked-finder` concept doc may be worth extracting.
- `docs/superpowers/specs/2026-05-07-vault-rooted-picker-design.md`: add a status line at top noting the picker has been superseded by this spec, with a link.

## Phasing

**Phase 1 (this spec):**
- New `internal/recent` package: Store, scoring, persistence, tests.
- TUI picker rewrite: flat list, no tree state.
- Visit recording on `openFile`.
- Doc updates.

**Phase 2 (separate specs, if desired after dogfooding):**
- Name-filter typing (Cmd-P-style fuzzy filter on the same surface).
- Sort-mode cycling (e.g. `s` cycles recent / name / path).
- Full-text content search as another mode.
- Updated-together view (files mtime-clustered with the current file).

**Phase 3:**
- Configurable decay constants if the defaults turn out to be wrong for someone.
- Per-vault state files if cross-vault contamination becomes annoying.

## Open questions / accepted risks

- **Decay constants are guesses.** 7-day mtime half-life and 2-day visit half-life feel right for personal-vault use but haven't been validated. Tweaking is a one-line change; the design doesn't depend on the specific numbers.
- **No score displayed.** A user who wants to debug why a file is ranked where it is has no in-UI signal. Acceptable: the rules are documented, and the recency timestamp + edited marker carry most of the explanatory weight.
- **macOS uses config dir, not state dir.** `os.UserConfigDir()` returns `~/Library/Application Support` on macOS and `~/.config` on Linux. Strictly, visits are state, not config — XDG would put them under `~/.local/state` on Linux. Following `os.UserConfigDir()` matches what most Go CLIs do and avoids a dep on a dedicated XDG library. Acceptable.
- **No locking on `visits.json`.** Concurrent `hypogeum` instances will lose some visits to last-writer-wins. Documented; not engineered for. If it ever matters, switch to file-lock-on-write.
- **Visit on `^p` is per-open, not per-cursor-movement.** Opening the picker doesn't Record anything; Enter does. Hovering with `j`/`k` doesn't count. This matches the principle that Record reflects *intent*, not exploration.
- **Cross-vault state file.** Two vaults share one `visits.json`, keyed by absolute path. Files from a different vault don't appear in the current vault's `Rank` (their paths aren't in `paths`); orphan entries decay naturally. If a user actively switches between many vaults, the file grows; eviction is YAGNI until it isn't.

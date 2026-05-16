# Full-text search design

Status: approved, ready for implementation plan.

`hypogeum` already gets the user to the right file via `^p` (recency-ranked fuzzy filename finder) and `^b` (tree modal). Neither helps when the user remembers a phrase but not the file. This spec adds **`^s`** — a full-text search modal that scans every markdown file in the vault, shows matching lines with surrounding context, and on `Enter` opens the file scrolled to the chosen hit.

## Scope and decisions

- **Files**: markdown only — the set `tree.Walk` already enumerates. Code/text files reachable via inline links are excluded.
- **Ranking**: recency-weighted, reusing `recent.Store.Rank` so the search modal feels like the picker. Files the user has recently opened or that have recently changed surface first when their content matches.
- **Trigger**: live with 150ms debounce. Each keystroke cancels the prior scan; the scan that lands is the one for the most recent query.
- **Navigation**: `Enter` navigates to the file and scrolls to the matched line, using the same `pendingPreselectRange` plumbing that backlink-follow and range-link Enter already use.
- **Case**: always case-insensitive substring match. No smart-case, no regex. Matches the picker's lowercased fuzzy ethos.
- **Result cap**: 200 hits, footer reads `… results truncated at 200, refine the query`. Mirrors `pickerMaxVisible`.
- **Minimum query length**: 2 characters. Shorter queries don't fire a scan; viewport shows `(type 2+ chars to search)`.

These were locked in via brainstorming on 2026-05-14. Alternatives considered and rejected: smart-case (added power-user surface area without solving a real pain point); deduplicated one-hit-per-file ranking (loses signal when a file mentions the term canonically vs. in passing); unlimited results (one-letter queries against a large vault would lag rendering); 1-char minimum (first keystroke is wasted work) and 3-char minimum (frustrating wait for short words like "go").

## Architecture and package boundaries

Two new units, one extension.

### New: `internal/search`

Pure search logic. No Bubble Tea, no recency, no modal. Imports `context`, `os`, `strings`, `sync` — that's the whole dependency surface.

```go
package search

type Hit struct {
    Path    string // absolute path
    Line    int    // 1-indexed
    Snippet string // ~60 visible chars around the match,
                   // with \x11/\x12 markers wrapping the matched substring
}

// Search scans every path for case-insensitive substring matches of
// query. Workers fan out across paths; ctx cancellation aborts in-flight
// scans (checked between files and every ~256 lines within a file).
// Returns at most maxHits hits — once reached, scanning short-circuits.
// Cancelled scans return (whatever-we-found-so-far, nil); cancellation
// is not an error from the caller's point of view.
func Search(ctx context.Context, paths []string, query string, maxHits int) ([]Hit, error)
```

The package returns hits in `(file, line)` order. The TUI re-sorts by recency because that needs `recent.Store`, which is a TUI dependency. Keeping `internal/search` pure means tests need no recency fakes.

### New: `internal/tui/search.go`

TUI integration following the picker pattern. Holds `searchState` (textinput, hits, cursor, viewport, in-flight scan context), and the dispatch glue.

```go
type searchState struct {
    input    textinput.Model
    paths    []string       // snapshot of vault paths at modal-open time
    hits     []search.Hit   // sorted by recency, capped
    cursor   int
    vp       viewport.Model
    scanCtx  context.Context
    scanStop context.CancelFunc
}
```

### Extension: `internal/tui/content.go`

`refreshContent` currently honors `m.content.rangeHighlight` only on the code-file render path. The search Enter flow needs the *markdown* path to honor it too, so that on render-complete the model calls `m.scrollToLine(rangeHighlight.Start)`. This is additive — no behavior change for code files, and it makes a feature that's already wired for code work the same way for markdown.

### Keymap and modal-kind

`keys.go` gains `OpenSearch: ^s`. `modal.go`'s `modalKind` enum gains `modalSearch`. `input.go`'s `handleKey` gets a new case in the modal-toggle switch, parallel to `OpenPicker`. The single-modal-swap invariant applies: pressing `^s` while another modal is open swaps to search. `?` (help) is still anchored (no-op while another modal is up — same as today).

### Package layering

```
internal/search       → context, os, strings, sync  (stdlib only)
internal/tui/search.go → internal/search, internal/recent, bubble tea
```

No change to the existing layering rule. `internal/search` could be imported from a future CLI subcommand or test harness without dragging TUI dependencies along.

## Data flow

### Open (`^s` pressed)

1. `handleKey` matches `OpenSearch`, calls `m.openModalWith(modalSearch, onOpen)`.
2. `onOpen` resets `searchState`: clears the input, hits, cursor; snapshots `m.allVaultMarkdownPaths()` into `searchState.paths`. The snapshot survives the modal's lifetime so a mid-search file delete from the watcher doesn't yank paths out from under an in-flight scan.
3. textinput is focused. `(type 2+ chars to search)` placeholder shows.

### Typing (printable keystroke arrives while `modalSearch` is open)

1. `handleKey` routes `tea.KeyRunes` to `handleSearchKey` before the global modal-toggle switch, parallel to the picker's printable-key fast path.
2. textinput appends the rune.
3. If a prior `scanCtx` exists, `scanStop()` cancels it. Workers from that scan return early.
4. If `len(query) < 2`: hits cleared, viewport shows placeholder, no `tea.Tick` scheduled.
5. Otherwise: a `tea.Tick(150ms)` is scheduled carrying the current query string. The tick's payload identifies which keystroke generation it belongs to.

### Debounce fire (tick lands in `Update`)

1. Compare the tick's query string to `searchState.input.Value()`. If different, the user is still typing — drop the tick.
2. Allocate a new `scanCtx` via `context.WithCancel`; store on `searchState`.
3. Return a `tea.Cmd` that, in its own goroutine, calls `search.Search(scanCtx, searchState.paths, query, 200)` and emits a `searchResultsMsg{query, hits, err}` when it returns.

### Result arrival (`searchResultsMsg`)

1. Check that `m.modals.kind == modalSearch` and `msg.query == searchState.input.Value()`. If either fails, discard — modal closed or user moved on.
2. Re-sort hits: project to unique paths, call `recent.Store.Rank`, then re-emit hits in that path order, preserving within-file `(file, line)` order for hits in the same file.
3. Store in `searchState.hits`, cursor reset to 0, viewport refreshed.
4. Log `search "<query>" returned M hits in Tms` via `m.diag.Info`.

### Enter on a hit

1. Read the hit at `searchState.cursor`.
2. `m.closeModal()` (restores focus to pre-modal pane).
3. `m.pendingPreselectRange = &markdown.LineRange{Start: hit.Line, End: hit.Line}`.
4. `m.navigateTo(hit.Path)` runs the normal history-aware open. `refreshContent` consumes `pendingPreselectRange`, renders the file, and (with the `content.go` extension above) calls `m.scrollToLine(hit.Line)` for both markdown and code destinations.

### Esc

- Non-empty query: clear query, cancel `scanCtx`, clear hits, stay open.
- Empty query: `closeModal()`.

Two-press semantics identical to the picker.

### Why `tea.Tick` rather than raw goroutines

Bubble Tea serializes state mutation through `Update`. A raw goroutine writing to `searchState` would race the model. `tea.Tick` schedules a future message; the Cmd that performs the scan runs the I/O in its own goroutine but only mutates state via the message it returns, which `Update` delivers serially. This is the pattern the existing watcher and picker use.

### Cancellation correctness

`tea.Tick` isn't cancellable, and a Cmd already in flight can't be cancelled from the next `Update`. What we *can* cancel is the work inside the Cmd via `ctx`. Each typing keystroke calls `scanStop()` on the prior context; workers `select { case <-ctx.Done(): return ... }` between files (cheap) and every ~256 lines within a file (negligible overhead). The eventual `searchResultsMsg` from a cancelled scan is discarded by the stale-query check at the top of the handler.

## UI rendering

### Modal layout

The modal fits the existing `modalGeometry` frame (60% × 60%, clamped 40-120 wide and 12-40 tall). Inside the frame, two header rows (input + separator) sit on top of a viewport holding the hit list.

```
┌─────────────────────────────────────────────────┐
│ > search query<cursor>                          │
│ ─────────────────────────────────────────────── │
│ ▌ docs/architecture.md:42                       │
│   …surrounding text with [match] highlighted…   │
│   docs/link-following.md:107                    │
│   …the [match] in context across this line…    │
│   …                                             │
│ ─────────────────────────────────────────────── │
│ … results truncated at 200, refine the query    │
└─────────────────────────────────────────────────┘
```

Viewport height inside the frame is `h - 4` (top border + input + separator + bottom border). Width is `w - 2`.

### Per-hit row composition

Each hit takes two rows in the viewport, matching the backlinks modal's pattern (`backlinks.go:66`):

- **Row 1**: `▌ <relative-path>:<line>`. The marker (`▌`, lipgloss `Foreground(62) Bold`) appears only on the row under the cursor. Path is `relativeTo(m.root, hit.Path)` truncated with leading ellipsis if it would overflow.
- **Row 2**: `  <snippet>`. Two-space indent so the snippet aligns under the path's first character. Snippet runs through `applyHighlight` (which exists today in `backlinks.go:88`) to turn `\x11`/`\x12` markers into bold yellow SGR. Truncated to `viewport.Width - 4`.

The selected hit's two rows both get `lipgloss.Reverse(true)`. The picker uses the same selection visual.

### Snippet generation

The search layer captures each match's full source line and wraps the match with `\x11`/`\x12`. If the line is wider than ~60 chars after marker stripping, trim with leading and trailing ellipses biased so the match stays roughly centered (~25 chars on each side). The 60-char target is generous because the TUI re-trims to `viewport.Width - 4` — this gives the renderer flexibility without re-reading files.

Match at start of line: no leading `…`. Match at end of line: no trailing `…`. Both ellipses are single-character `…`, not three dots.

### Empty / loading states

| Condition | Viewport content |
|---|---|
| Modal just opened, query empty | `(type 2+ chars to search)` (faint) |
| Query 1 char | `(type 2+ chars to search)` (faint) |
| Query ≥ 2 chars, scan in flight, no prior hits | `(searching…)` after 50ms (faint) |
| Query ≥ 2 chars, scan in flight, prior hits exist | prior hits stay visible (no flicker) |
| Query ≥ 2 chars, scan complete, zero hits | `(no match for "<query>")` (faint) |
| Vault has no markdown files | `(no markdown files in vault)` (faint), same as picker |

### Footer overflow

When `len(hits) >= 200`, the footer below the viewport reads `… results truncated at 200, refine the query` (faint). When `len(hits) < 200`, no footer. The wording is deliberate — we don't know the true overflow count (we stopped at 200), so `… N more` would be misleading.

### Keys while modal is open

| Key | Action |
|---|---|
| printable | append to query, reset/schedule debounce |
| `^j` / `↓` | cursor down |
| `^k` / `↑` | cursor up |
| `Enter` | navigate + scroll to selected hit |
| `Esc` × 1 | clear non-empty query |
| `Esc` × 2 | close modal |
| `^s` | close modal (toggle off) |
| Backspace | textinput handles, debounce re-fires |

`j`/`k` are typing keys, identical to the picker's choice. `^j`/`^k` are the cursor-movement keys.

### Help integration

`OpenSearch` joins the `keyMap` and shows up in the `?` help modal automatically.

## Error handling and edge cases

| Case | Behavior |
|---|---|
| Worker can't open a file | Skip silently in worker, scan continues. One `m.diag.Info` line per failure at the end of the scan. |
| File has NUL in first 512 bytes | Skip (defensive — `tree.Walk` already filters non-markdown, so this should never fire). |
| File >1 MiB | Scan first 1 MiB, log `truncated scan of <path>`. Belt-and-suspenders for the same reason. |
| Empty vault | Empty-state message, no scan ever fires. |
| Query contains `[`, `*`, etc. | Literal substring match — no regex anywhere. |
| Unicode query | `strings.ToLower` for ASCII; Latin Extended works via the same path. Locale-aware folding is out of scope and not used anywhere else in the codebase. |
| File modified mid-scan | Worker reads once into memory; sees pre- or post-modification content. Either is fine. |
| File deleted between scan-return and Enter | `navigateTo` → existing `refreshContent` error path renders the failure in the content pane. No new handling. |
| Watcher fires during open modal | `searchState.paths` snapshot is intentionally stale for the modal's lifetime. New files appear after re-open (`^s` again). |
| Debounce tick lands after modal closed | Discarded by `modals.kind != modalSearch` check. |
| Stale `searchResultsMsg` from cancelled scan | Discarded by `msg.query != currentQuery` check. |
| Two scans race | Only one `scanCtx` lives at a time. The newer one cancels the older. Both eventually emit messages; only the matching-query one is honored. |
| Cancellation cost | Cancelled worker finishes current line and returns. <5ms wasted in the worst case. |
| Hit limit reached | `Search` short-circuits via `ctx` cancel from the dispatcher when buffered channel fills past `maxHits`. Workers return early. Footer disclosed honestly. |
| Hit file never visited | `recent.Rank` returns a `Ranked` with zero `Visit` — already handled, no special case. |

## Testing strategy

### `internal/search` — pure unit tests

Lay out real files via `t.TempDir()`; call `Search` directly with `context.TODO()` or a controlled `context.WithCancel`. No fakes needed.

- Basic single-file single-hit.
- Case-insensitive matching against `Foo`/`FOO`/`foo`.
- Multiple matches in one line: snippet wraps the first; second appears in context, not separately highlighted.
- Multiple files: fan-out preserves all hits.
- Snippet trim: ~200-char line returns ~60-char window centered on the match with `…` boundaries.
- Match-at-start: no leading `…`. Match-at-end: no trailing `…`.
- `maxHits` cap: 300-hit corpus with `maxHits=10` returns exactly 10.
- Empty query: nil hits, no panic.
- Missing file in path list: skip, no error.
- Binary-byte file: skip, no error.
- Pre-cancelled context: returns quickly with zero hits and nil error.
- Mid-scan cancellation: scan over a corpus that takes ~50ms; cancel at 5ms; assert return within another 50ms. Test pins the ctx-check frequency inside the worker loop.
- Unicode body: `résumé` in source, `RÉSUMÉ` in query — matches if `strings.EqualFold` covers Latin Extended; otherwise pin what we *do* support.

### `internal/tui` — model-level tests

Following `picker_test.go` and `backlinks_test.go`: construct a `Model` via `New`, drive with synthesized `tea.KeyMsg`, assert state.

- `^s` opens the modal; `m.modals.kind == modalSearch`; input focused; hits empty.
- `^s` while open closes it; focus restored.
- `^s` while picker is open swaps to search; picker state is wiped (existing single-modal-swap invariant).
- Typing 1 char does not fire a scan.
- Typing 2 chars fires after the debounce window.
- Stale tick from prior keystroke is discarded.
- Enter on a hit: `m.history.Current()` is the hit's file; viewport YOffset reflects `scrollToLine(hit.Line)`. This is the regression guard for the `content.go` extension.
- Enter when results empty: no-op, modal stays open.
- Esc once clears non-empty query; Esc twice closes.
- Empty vault: empty-state placeholder rendered.
- Recency rerank: visit file B via `m.openFile`, search for a term hitting both A and B, assert B sorts before A. Mirrors `TestPickerRecentVisitBoostsRank`.
- `^j`/`^k` move cursor with bounds at 0 and `len(hits)-1`.
- `j`/`k` are typing keys, not cursor keys.
- Esc cancels in-flight scan (no panic when `searchResultsMsg` lands after modal closed).

### Integration smoke

One end-to-end test in a `dispatch_test.go` neighbor: open the modal, type a query, advance ticks, press Enter, verify the content pane shows the expected file with the expected line in view. The "did we actually wire it up correctly" sanity check.

### Race safety

CI runs `go test -race`. The mid-scan cancellation test is the most likely to expose a worker/dispatcher race, so it serves double duty as the race-coverage anchor.

### Out of scope

- Stress / 10k-file corpus tests — interesting for tuning, not for correctness.
- Glamour rendering of hits — hits don't go through Glamour; they're rendered by our own `formatSearchResults`.
- Fuzz tests for snippet boundaries — unit cases cover the 5 boundary positions.

## Implementation order

The plan that follows from this spec should land in this order, each as its own commit:

1. `internal/search` package with `Hit`, `Search`, and the full unit test suite. No TUI integration yet.
2. `content.go` extension to honor `rangeHighlight` on the markdown render path (small, isolated, behavior-preserving for code files).
3. `searchState`, `modalSearch` kind, `OpenSearch` keymap (auto-shows in the `?` help modal via the existing keymap iteration), `handleSearchKey`, dispatch routing, render functions in `internal/tui/search.go`. Tests in `internal/tui/search_test.go`.
4. Integration smoke test in the dispatch test neighbor.
5. `CLAUDE.md` updated with the new gotchas: `^s` keystroke, `searchState.paths` snapshot invariant, the markdown-rangeHighlight extension.

Each commit should keep the test suite green. The plan-writing skill will turn this order into a step-by-step plan with checkboxes.

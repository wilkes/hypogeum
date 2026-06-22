# Search Modal — File Grouping — Design

**Status:** Implemented (this branch).
**Date:** 2026-06-22
**Branch:** `claude/search-file-grouping`

> **Implementation note (2026-06-22).** Shipped as designed. `search` gained
> `FileMatches`/`Line`, `SearchGrouped`, and `RenderSnippet`, with `scanFile`
> refactored into a shared `scanFileLines` + `scanFiles` so the hit-oriented and
> grouped paths share one match-finding pass (`TestRenderSnippet_MatchesEagerHitSnippet`
> guards snippet identity). The TUI modal moved to a flattened `[]searchRow` with
> a fold map, count-desc/`mtime` sort, the sqrt bar, and `Tab` fold toggle. Two
> open questions resolved to the proposed defaults: bar width **8**, expanded-match
> inline cap **50**. `Enter`-on-file = jump-to-first kept (not expand).

## Goal

Rework the `/` full-text search modal from a flat list of individual matches
into a **file-grouped** view: one row per matching file showing **how many
times** it matches, with the individual matches reachable by expanding the file
inline. The point is breadth — let the user scan a far larger number of *files*
at a glance and judge "which files is this concept concentrated in" before
drilling into any one of them.

Today (`formatSearchHits`, `internal/tui/search.go`) every match costs two rows
(`path:line` + snippet), so a file with 30 matches floods 60 rows and a 40-row
modal shows ~3–4 files. The matches are also mtime-ranked across the whole list,
so the *count* per file — the thing a user most wants when locating a concept —
is never surfaced at all.

## What the new modal looks like

Collapsed (default — one row per file):

```
> render
──────────────────────────────────────────────────────────
▌ 47 ████████  internal/markdown/render.go          ▶
  23 ████      internal/tui/content.go               ▶
   8 █▌        internal/markdown/links_render.go     ▶
   3 ▌         docs/packages/markdown.md             ▶
   1 ▏         README.md                             ▶
            … 6 more files
```

Expanded (cursor on `render.go`, `Tab` pressed):

```
> render
──────────────────────────────────────────────────────────
▌ 47 ████████  internal/markdown/render.go          ▼
        312  glamour **render**er rebuilt on resize…
        318  **render**WithLinks returns the list…
        401  per-row **render** writes a tail…
            … 44 more in this file
  23 ████      internal/tui/content.go               ▶
```

Row anatomy of a **file header**: `cursor │ count (right-aligned) │ proportional
bar │ relative-path │ fold caret (▶/▼)`. A **match row** (only when expanded):
indent │ `line` │ highlighted snippet.

### Decisions (locked in brainstorm)

- **Drill-down: inline expand/collapse.** Reuses the tree modal's mental model.
- **Count viz: number + proportional bar.** Precise *and* scannable.
- **Sort: match count descending**, tie-broken by edit recency (mtime).

## Interaction model

The modal keeps a **flattened row list** rebuilt whenever hits, fold state, or
the query change — the same flatten-once / cursor-is-an-index pattern the tree
modal uses (CLAUDE.md "pre-flatten for keystroke performance"). Each visible row
is either a `fileRow` or a `matchRow`; the cursor is an index into the visible
slice.

**Keymap.** The query `textinput` is always focused and owns every printable key
**plus `←`/`→`** (it forwards them for query editing — `search.go` today). That
makes `Space`, `→`, `h`/`l` unavailable for fold control, so:

| Key | Action |
|-----|--------|
| `↑` / `↓`, `^k` / `^j` | move cursor across visible rows (files + expanded matches) |
| `Tab` | toggle fold on the file under the cursor (if the cursor is on a match row, fold its parent file) |
| `Enter` on a file row | navigate to that file's **first** match |
| `Enter` on a match row | navigate to that match |
| `Esc` | clear a non-empty query, else close (unchanged) |
| printable / `←` `→` / `Backspace` | edit the query (unchanged) |

`Tab` is the one free, natural "expand" key that doesn't collide with query
editing. `Enter`-on-file gives instant jump without forcing an expand first.

Navigation reuses the existing plumbing unchanged: on `Enter` we set
`m.pending.preselectRange = &markdown.LineRange{Start: line, End: line}` and call
`m.navigateTo(path)` — exactly what the flat modal does today, just sourced from
the selected row's `(path, line)` instead of a `search.Hit`.

**Fold state** is a `map[string]bool` keyed by absolute path (open == true),
defaulting to collapsed — mirroring `m.tree.expanded`. It is **reset on every
new scan** (a fresh result set starts all-collapsed); it is *not* user state to
preserve across queries.

**Cursor stability across folds.** Toggling a fold rebuilds the flattened rows;
the cursor stays on the same *file header* it was on (we re-find that file's row
index after the rebuild) rather than holding a raw index that would now point at
a different row. Expanding never moves the cursor off the file you toggled.

## Data model: grouped results with lazy snippets

The count is the headline, so it must be **accurate** — which the current
200-*hit* cap can't guarantee (a file with 300 matches can't read "≤200," and an
early cap silently drops whole files). The fix is to count every match but build
snippets only when needed.

### New `search` surface

Add a grouped scan alongside the existing `Search` (keep `Search` for the
`query`/CLI path, which is pointer-per-line by design):

```go
// FileMatches is one file's full-text match summary.
type FileMatches struct {
    Path  string // absolute path
    Count int    // total matching lines in the file
    Lines []Line // every matching line: number + raw line text (no snippet yet)
}

type Line struct {
    Num  int    // 1-indexed
    Text string // the raw line; snippet/highlight built lazily by the caller
    At   int    // byte offset of the match within Text
    Len  int    // match length
}

// SearchGrouped scans paths for query and returns per-file matches, capped at
// maxFiles files (not hits). Counting is exhaustive within a scanned file;
// snippet construction is deferred to the caller.
func SearchGrouped(ctx context.Context, paths []string, query string, maxFiles int) ([]FileMatches, error)
```

Why this shape:

- **Accurate counts, bounded memory.** Every matching line in a file is counted
  and its `(Num, Text, At, Len)` retained, but the file list is capped at
  `maxFiles` (e.g. 200 files — *far* more coverage than 200 hits). `Line.Text`
  is the already-read line; no extra I/O.
- **Lazy snippets make the common case cheaper than today.** `buildSnippet`'s
  ellipsis-trimming + marker insertion runs only for the matches of an *expanded*
  file (the TUI calls `search.RenderSnippet(line, budget)` per visible match
  row). A collapsed result set builds **zero** snippets — strictly less work than
  the current modal, which snippets every hit up front.
- `buildSnippet` is refactored to take a `Line` (it already operates on
  `line[matchAt:matchAt+matchLen]`); the existing per-line scan that finds
  `(matchAt, matchLen)` feeds both the old `Hit` path and the new `Line` path, so
  match-finding logic stays single-sourced.

### TUI consumption

- `runSearchCmd` calls `SearchGrouped` and stores `[]FileMatches`.
- Sort: by `Count` desc, tie-break by mtime via the existing
  `recent.RankPathsByMTime` (reused from `rerankByMTime`). Within a file, `Lines`
  stay ascending by `Num`.
- `renderSearchRows` builds the flattened `[]searchRow` from `[]FileMatches` +
  the fold map, then renders headers (count + bar + path + caret) and, for
  expanded files, match rows (snippet built lazily via `RenderSnippet` +
  `applyHighlight`, the existing `\x11`/`\x12` → SGR converter).

## The proportional bar

A fixed gutter (proposed **8 cells**) using Unicode block partials
(`▏▎▍▌▋▊▉█`). Scale each file's `Count` against the **max count in the current
result set**. Use a **sqrt** (or log) scale with a one-eighth floor for any
nonzero count, so a 1-match file still shows `▏` instead of vanishing next to a
47-match file under a linear scale. The numeric count remains the source of
truth; the bar is a scannability aid. Bar + count occupy a fixed left gutter so
paths align.

## Truncation & footers

- Cap at `searchMaxFiles` (200) **files**. When hit, the footer reads
  `… results truncated at 200 files, refine the query` (today's message, files
  not hits).
- A collapsed file with more matches than fit isn't a concern (it's one row). An
  *expanded* file caps its inline match rows at a small N (proposed 20) with a
  faint `… N more in this file` tail, so expanding a 200-match file doesn't
  reflood the modal — read the rest by opening the file.

## Non-goals

- **No change to the `search` CLI verb / `query.Search`.** Those stay
  pointer-per-line (`{path,line,snippet}`) — scripts want every match, not a
  per-file rollup. `SearchGrouped` is additive; `Search` stays.
- **No regex / no multi-match-per-line counting.** Count is *matching lines*
  (the existing granularity), consistent with the current scanner. A line with
  two matches counts once. (Could revisit, but it'd change the scan's inner
  loop.)
- **No fuzzy ranking / relevance score.** Sort is count then recency — both
  already-available signals.
- **No persisted fold state across opens.** Reset per scan, like the tree map is
  derived per navigation.
- **No two-pane preview.** Considered and rejected in brainstorm (width-hungry in
  the modal); inline expand chosen instead.

## Testing (TDD, model-level — no TTY)

- `internal/search`: `SearchGrouped` over a temp fixture — exhaustive counts
  (incl. a file with > `maxFiles`-worth of matches to prove file-capping, and a
  file with many matching lines to prove count accuracy), stable ordering,
  ctx-cancellation parity with `Search`, and `RenderSnippet` byte-equivalence to
  the old inline snippet for the same `(line, match, budget)`.
- `internal/tui` (`model_test.go` style): grouped render shows one row per file
  with the right count; sort is count-desc then mtime; `Tab` toggles the fold map
  and re-flattens; cursor stays on the toggled file; `Enter` on a file row vs a
  match row sets the correct `preselectRange` + `navigateTo` target; the
  expanded-match cap and both truncation footers render.
- Keep the suite race-clean (`go test -race ./...`).

## Rollout

1. `internal/search`: extract match-finding from `buildSnippet`; add
   `FileMatches`/`Line`, `SearchGrouped`, `RenderSnippet`. Tests.
2. `internal/tui/search.go`: swap `[]search.Hit` → `[]search.FileMatches`, add
   the fold map + flattened `[]searchRow`, count-desc sort, bar/header/match
   renderers, lazy snippet on expand.
3. `internal/tui/input.go`: `Tab` fold toggle; `Enter` file-vs-match dispatch;
   cursor moves over the flattened rows.
4. Keys: add a `FoldToggle` (`Tab`) binding in `defaultKeys()` (CLAUDE.md
   "keybindings live in one place").
5. Docs: update the `/` search bullets in CLAUDE.md and `README`/help, and the
   `docs/index.md` hook.

## Open questions

- **Bar width & scale** (8 cells, sqrt) are proposals — easy to tune once it's on
  screen against the real `docs/` vault.
- **Expanded-match inline cap** (20) — generous enough that most files show all
  matches; revisit if it feels stingy.
- **`Enter` on a collapsed file = first match.** If usage shows people expect
  `Enter` to *expand* (and only navigate from a match row), that's a one-line
  swap with `Tab` taking the navigate role — flagged for the review.

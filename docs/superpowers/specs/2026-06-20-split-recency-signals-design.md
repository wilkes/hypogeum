# Split recency signals: edit-recency vs visit-recency

Status: design

## Problem

`internal/recent` blends two distinct signals into one number:

- **filesystem mtime** — "recently edited," a long-term relevance signal
  (7-day half-life), and
- **persisted visit history** — "recently opened in hypogeum," a short-term
  attention signal (2-day half-life, ×0.5 weight).

One `score(now, mtime, visit)` feeds every recency surface: the `^p`/`o` file
finder, the `/` search-result re-rank, and the CLI `recent` verb. Because the
two signals are mixed, no surface answers either question cleanly:

- The finder is meant for "jump to the file I'm working on," which is almost
  always the most recently *edited* file — but a stale visit can outrank a
  fresh edit, and an exponential blend makes the ordering hard to reason about.
- There is no surface that answers "what did I open recently?" — the natural
  "recently opened" list is buried inside the blend and contaminated by mtime.

## Decision

Pull the two signals apart into two independent, exact orderings, each with its
own entry point. No blending, no exponential decay, no weights — each ordering
is a plain sort on a single key.

### Two rankings in `internal/recent`

- **`RankByMTime(paths []string) []Ranked`** — stateless. `os.Stat` each path,
  drop stat failures silently, sort by mtime descending (newest edit first).
  Needs no `Store` because mtime lives on the filesystem. A
  `RankPathsByMTime(paths) []string` convenience mirrors the old `RankPaths`
  for callers that only want the ordered path slice.

- **`(s *Store) RankByVisit(paths []string) []Ranked`** — visit-aware. Keep
  only paths that have a recorded visit, sort by visit time descending
  (most-recently-opened first). Never-visited files are *excluded*, not
  ranked-last — "recently opened" is a list of things you actually opened.

The blended `score` function, the `mtimeHalfLifeHours` / `visitHalfLifeHours` /
`visitWeight` constants, and the old hybrid `Rank` / `RankPaths` are removed.
The `Ranked` struct keeps `Path`, `MTime`, and `Visit`; the `Score` field is
dropped because neither new ordering needs a numeric score (both sort on a
`time.Time` key directly).

The `Store` and all visit persistence (`New`, `Record`, `DefaultStateFile`,
the JSON save/load, the `nowFunc` clock) are unchanged — visits still get
recorded on every `openFile` and still persist across sessions. Only the
ranking surface changes.

### Three entry points

1. **Finder (`^p`/`o`) → pure mtime.** Newest-edited first, no visit term. The
   finder is "jump to what I'm working on," and the freshest edit is the best
   default. Built via the stateless `recent.RankByMTime` — the picker no longer
   needs `m.recent`.

2. **Search modal (`/`) re-rank → pure mtime.** Consistency with the finder:
   both file-locating surfaces answer "recently edited."

3. **New `r` "recently opened" modal → pure visit order.** A swappable modal
   (same `openModalWith` / single-modal-swap machinery as backlinks/tree/logs;
   `?` help stays anchored). Lists visited files, most-recent first,
   visited-only. `j`/`k` (+ arrows) move the cursor, `Enter` navigates
   (history-aware `navigateTo`), `Esc` closes. Empty state when nothing has
   been visited, mirroring the backlinks empty line.

4. **CLI `recent` verb → pure visit order.** Aligned with the new modal:
   last-visited descending, visited-only. `RecentEntry` drops its `score` JSON
   field along with `Ranked.Score`.

## Why exact sorts instead of decay

The decay blend existed to *combine* two signals on one axis. Once the signals
are split, each ordering has a single key and a single question, so a plain
descending sort is both simpler and exactly what the user asked for. Decay only
mattered for trading mtime against visit at equal age — a trade that no longer
exists.

## Gotchas preserved / introduced

- **Picker grabs printable keys before global modal-toggles.** `r` is a new
  lowercase modal-toggle. It lives in the global modal-toggle switch *after*
  the `KeyRunes`→picker routing, so typing `r` into the finder's fuzzy filter
  types `r` and does not open the recent modal. Regression-tested alongside the
  existing `b`-into-picker test.
- **Modal swap + anchored help.** `r` swaps with backlinks/tree/logs/picker
  under the single-modal-swap invariant; `?` remains a no-op while another
  modal is open. `Esc` closes `modalRecent` like the others.
- **Visit recording must stay.** `openFile`'s `m.recent.Record(path)` is the
  sole source of the visit data the new modal and CLI verb read.

## Out of scope

Configurable half-lives (removed entirely), per-surface weighting (gone with
the blend), and any change to visit persistence format.

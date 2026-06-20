# `internal/recent`

Owns hypogeum's recency ordering and its persisted visit history. Standard-library only — it knows nothing about the directory tree, the vault, or the TUI. Callers pass in a slice of absolute paths and get back a sorted slice of ranked entries.

See also: [architecture overview](../architecture.md), [`internal/tui`](tui.md) (consumer), [`internal/search`](../../internal/search) (re-rank helper), unified-finder-recency spec ([2026-05-12](../superpowers/specs/2026-05-12-unified-finder-recency-design.md)).

## Purpose

Answer two recency questions, and persist the data the second one needs:

1. **Edit-recency** — "what am I working on?" — filesystem mtime, newest first. Stateless.
2. **Visit-recency** — "what have I opened recently?" — last-opened time, from a persisted visit map. Visited-only.

## The two orderings are deliberately *not* blended

Edit-recency and visit-recency are kept as two separate orderings, each a plain descending sort on a single key — there is no combined numeric score. The package doc comment is explicit about this: `RankByMTime` answers "most recently edited first" (stateless, `os.Stat`-based) and `RankByVisit` answers "most recently opened first" (visited-only, from the persisted visit map). An earlier design blended a hybrid score; the split is intentional, so don't reintroduce a single weighted number.

## Types

```go
// Ranked carries one entry of an ordered ranking result. MTime is
// populated by RankByMTime; Visit by RankByVisit. Each ordering sorts
// on its own field — there is no combined numeric score.
type Ranked struct {
    Path  string
    MTime time.Time // file modification time (RankByMTime); zero otherwise
    Visit time.Time // last visit (RankByVisit); zero if never visited
}

// Store holds the persisted visit history (a path → last-visit map)
// behind a defensive mutex; the visits file is written atomically.
type Store struct{ /* unexported */ }
```

## Public surface

Stateless edit-recency (no `Store` needed — mtime lives on the filesystem):

- `RankByMTime(paths []string) []Ranked` — paths sorted by filesystem mtime, newest first. Paths that fail `os.Stat` (missing/unreadable) are dropped silently. mtime is re-read on every call (the watcher may have touched files since the last one).
- `RankPathsByMTime(paths []string) []string` — `RankByMTime` reduced to just the ordered path slice, for callers that don't need the `Ranked` wrapper. Same drop-missing semantics.

Persisted visit-recency (via `Store`):

- `New(stateFile string) (*Store, error)` — loads visits from `stateFile`. A missing file is not an error; a malformed file or unknown version returns a non-nil error *alongside* an empty-but-usable `Store`, so the caller can surface the failure as a diagnostic and keep running.
- `(*Store).Record(path string)` — marks `path` visited now and writes through to the state file (atomic temp-file + rename).
- `(*Store).RankByVisit(paths []string) []Ranked` — the subset of `paths` with a recorded visit, sorted by visit time descending (most recently opened first). Never-opened files are excluded entirely — this is the "recently opened" list, not a recency score over the whole vault. Does not `os.Stat`, so a visited-then-deleted file still appears (callers may filter on existence if they care).
- `DefaultStateFile() (string, error)` — `os.UserConfigDir()` + `hypogeum/visits.json`.

## Who consumes it

- **The file finder (`^p` / `o`) and the `/` full-text search re-rank** use the stateless edit-recency ordering: the finder calls `RankByMTime` on open ([`internal/tui`](tui.md)), and the CLI search verb feeds `RankPathsByMTime` into `search.RerankByRecency` (`internal/query`). "Jump to what I'm working on" is almost always the most recently edited file.
- **The recently-opened modal (`r`) and the CLI `recent` verb** use `Store.RankByVisit` — the visited-only, last-opened-first list.

Visit recording happens in the TUI's `openFile` ([content.go](../../internal/tui/content.go)), which calls `Store.Record` on every file open so the visit map stays current; the finder/search rankings never touch the `Store`.

## Concurrency and persistence

The `Store` mutex is defensive — in normal TUI use the store is touched from a single goroutine. Concurrent hypogeum instances share one state file (last writer wins on the visits map); each write goes through a per-process temp file (`os.CreateTemp`) and an atomic rename, so a reader never sees a torn file. The file format is versioned (`version: 1`); an unknown version is rejected at load.

# `internal/watch`

Filesystem watcher that surfaces markdown-relevant changes as debounced, coarse-grained events. Wraps `fsnotify` so the TUI doesn't have to deal with raw inotify/kqueue ops.

See also: [architecture overview](../architecture.md), [`internal/tui`](tui.md) (the only consumer).

## Purpose

Live updates. Without this package the tree is static after startup and file contents are only re-read on navigation. With it, edits in another window land in the rendered pane within ~100ms, and new/deleted files appear or disappear from the tree as they happen.

## Public surface

```go
type Watcher struct{ /* unexported */ }

func New(root string) (*Watcher, error)
func (w *Watcher) Events() <-chan Event
func (w *Watcher) AddPath(dir string) error
func (w *Watcher) Close() error
```

`AddPath` registers an extra directory with the underlying fsnotify watcher (no-op on a nil watcher or nil internal handle). The TUI uses it to watch the parent directories of source files pulled in by `![[file#L..]]` embeds, so live-reload fires when an embedded source changes.

Events are coarse on purpose:

```go
type EventKind int
const (
    StructureChanged EventKind = iota // re-walk the tree
    FileModified                      // re-read the open file if it matches
)

type Event struct {
    Kind  EventKind
    Paths []string // affected absolute paths
}
```

## Design choices

**Watch directories, not files.** fsnotify doesn't recurse, but per-file watches lose their target on rename-over-temp (which is how most editors save). The watcher walks the tree once at startup and `Add`s every non-hidden directory. New directories raised via `Create` get added on the fly inside `stage` — `classify` is a pure function that reports `MaybeNewDir` on Create events; `stage` does the `os.Stat` and adds the directory if needed.

**Coarse event kinds.** The TUI doesn't care whether a file was created vs. renamed — both mean "rebuild the tree." Collapsing the fsnotify op set into two intents keeps the consumer simple and means tests don't have to mirror platform-specific event shapes.

**Debounce window of 100ms.** A vim `:w` produces 2–4 raw events (write of swap, rename over original, chmod). Without debouncing the TUI would re-walk the tree four times per save. The window is short enough to feel instant but long enough to coalesce the burst.

**Hidden-path filter is path-based, not Add-based.** We never `Add` `.git`, but events for files *inside* it can still arrive via the parent. `tree.IsHiddenPath` (shared with `internal/tree`) walks the path components and rejects anything whose ancestry contains a dotted segment, so the two views stay consistent.

**Pure-classify, impure-stage.** `classify(ev)` returns a `classifyResult` from op flags + path alone — no I/O — so it can be exercised by table-driven tests in `classify_test.go`. The wrapper `(*Watcher).stage` does the impure work: stat'ing Create targets to distinguish new directory from new file, calling `addDirsRecursive`, and recording into the pending-event maps.

**Best-effort, no fatal errors.** If `fsnotify.NewWatcher` fails (e.g. inotify limits exhausted on Linux), `tui.New` includes the error in a `diag.Warn` and runs without live updates. The browser still works, just statically.

## How the TUI consumes it

`internal/tui/model.go` holds a `*watch.Watcher` on the model. `Init()` returns a `tea.Cmd` that blocks on `Events()` and emits `fsEventMsg`; `Update()` reacts and re-issues the command so the loop keeps listening.

`StructureChanged` triggers `tree.Walk(m.root)`, rebuilds `m.tree.flat`, and restores the cursor by path (so a tree change beneath your selection doesn't kick the highlight elsewhere).

`FileModified` only re-reads the file if a path in the event matches `m.history.Current()`. The viewport's `YOffset` is captured before the refresh and restored after so saving doesn't yank you back to the top of the document.

## Tests

`watch_test.go` exercises the real fsnotify path against a `t.TempDir()`. Each test waits up to 2s on the events channel — generous enough for slow CI, short enough that genuine misses fail quickly. `classify_test.go` covers the pure classifier directly with table-driven cases for create/write/remove/rename and hidden-path filtering. TUI-level tests in `internal/tui/watch_test.go` synthesize `watch.Event` values directly to keep `handleFSEvent` deterministic.

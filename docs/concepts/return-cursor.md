# return-cursor

The single-slot cursor restoration that lets the user follow a backlink with `Enter`, navigate back with `h`, and resume scanning the backlinks list at the entry they followed from.

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [`internal/tui`](../packages/tui.md) and the [backlinks-navigation design](../superpowers/specs/2026-05-07-backlinks-navigation-design.md); press `b` for the full backlinks list.

## Why it exists

The "scan backlinks one at a time" workflow is: open the modal (`b`), move cursor (`j`/`k`) to an entry, follow it (`Enter`), read the source file briefly, return (`h`), reopen the modal at the same cursor, move to the next entry (`j`), repeat. Without restoration, every return drops the user at backlink cursor 0 — they have to scroll back to where they were. With restoration, the loop is tight and the cursor matches their mental model.

The state to restore is small (which file, which cursor index), and it's only valid for *one* return — going back twice, forward, or to an unrelated file via the tree should discard it.

## How it works

Single-slot state on the model:

```go
type returnCursor struct {
    sourceFile string // the file whose backlinks were being navigated
    cursor     int    // backlinks.cursor at follow time
}

// stored on the model as m.backlinks.returnCursor (nil when no follow is pending return)
```

**Set on follow** (inside `followBacklink`, before `openFile` mutates history):

```go
m.backlinks.returnCursor = &returnCursor{
    sourceFile: m.history.Current(),
    cursor:     m.backlinks.cursor,
}
```

**Consumed on Back** (after `history.Back()` and `refreshContent`): the restore is factored into `maybeRestoreReturnCursor(path)` (`internal/tui/backlinks.go`), called with the path just navigated to. It bails unless a slot is set and `path == rc.sourceFile`, consumes the slot, reopens `modalBacklinks`, and calls `refreshBacklinksModal` **twice** — once to populate `m.backlinks.items`, then clamp the saved cursor against the (possibly shrunk) list, then a second refresh so the highlight lands on the right row:

```go
func (m *Model) maybeRestoreReturnCursor(path string) {
    if m.backlinks.returnCursor == nil || path != m.backlinks.returnCursor.sourceFile {
        return
    }
    rc := m.backlinks.returnCursor
    m.backlinks.returnCursor = nil
    // ... reopen modalBacklinks ...
    m.refreshBacklinksModal(path)                                  // populate items
    m.backlinks.cursor = clamp(rc.cursor, 0, len(m.backlinks.items)-1)
    m.refreshBacklinksModal(path)                                  // re-render at clamped cursor
}
```

The `path == rc.sourceFile` check is path-keyed, not time-keyed: if the user navigates Back twice, the second Back lands on a *different* file, the check fails, and the slot is left untouched (it'll be consumed if they ever return to the original — though in practice the user has moved on by then). The slot is cleared either way on the next successful match-and-restore.

## Invariants / gotchas

- **Single-slot.** Only the most recent follow is remembered. Following a second backlink before returning overwrites the slot. This matches the user's mental model — "the last place I came from" is what `h` should restore.
- **Path-keyed lifetime, not time-keyed.** A stale `returnCursor` is harmless: it sits there until either the user returns to its `sourceFile` (consumed) or some unrelated path eventually matches `sourceFile` (rare; restoration would still be valid because the cached cursor was at that file's backlink list).
- **Cursor is clamped on restore.** If the vault refreshed between follow and return and the selected backlink no longer exists, the cursor lands on a neighbor. Test: `TestReturnCursor_ClampsToShrunkList` in `internal/tui/backlinks_test.go`.
- **Always restores to the modal.** Earlier versions tracked which surface (pane vs modal) the user was on; the pane was later removed in favor of a modal-only backlinks surface, so the restore now unconditionally reopens the modal.
- **Vault refresh between follow and return is fine.** The restore re-queries `vault.Backlinks` for the path; the clamp handles list-shrink.

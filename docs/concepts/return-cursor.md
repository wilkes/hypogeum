# return-cursor

The single-slot cursor restoration that lets the user follow a backlink with `Enter`, navigate back with `h`, and resume scanning the backlinks list at the entry they followed from.

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [`internal/tui`](../packages/tui.md) and the [backlinks-navigation design](../superpowers/specs/2026-05-07-backlinks-navigation-design.md); press `b` for the full backlinks list.

## Why it exists

The "scan backlinks one at a time" workflow is: open the pane (`b`), move cursor (`j`/`k`) to an entry, follow it (`Enter`), read the source file briefly, return (`h`), move to the next entry (`j`), repeat. Without restoration, every return drops the user at backlink cursor 0 — they have to scroll back to where they were. With restoration, the loop is tight and the cursor matches their mental model.

The state to restore is small (which file, which cursor index, which surface — pane or modal), and it's only valid for *one* return — going back twice, forward, or to an unrelated file via the tree should discard it.

## How it works

Single-slot state on the model:

```go
type returnCursor struct {
    sourceFile string             // the file whose backlinks were being navigated
    cursor     int                // backlinks.cursor at follow time
    surface    backlinksSurface   // surfacePane | surfaceModal
}

// stored on the model as m.backlinks.returnCursor (nil when no follow is pending return)
```

**Set on follow** (inside `followBacklink`, before `openFile` mutates history):

```go
m.backlinks.returnCursor = &returnCursor{
    sourceFile: m.history.Current(),
    cursor:     m.backlinks.cursor,
    surface:    m.activeBacklinksSurface(),
}
```

**Consumed on Back** (after `history.Back()` and `refreshContent`):

```go
if rc := m.backlinks.returnCursor; rc != nil && path == rc.sourceFile {
    m.refreshBacklinks(path)
    m.backlinks.cursor = clamp(rc.cursor, 0, len(m.backlinks.items)-1)
    switch rc.surface {
    case surfacePane:
        if m.shouldShowBacklinks() {
            m.focus = focusBacklinks
        }
    case surfaceModal:
        m.modals.kind = modalBacklinks
        m.refreshBacklinksModal(path)
    }
    m.backlinks.returnCursor = nil
}
```

The `path == rc.sourceFile` check is path-keyed, not time-keyed: if the user navigates Back twice, the second Back lands on a *different* file, the check fails, and the slot is left untouched (it'll be consumed if they ever return to the original — though in practice the user has moved on by then). The slot is cleared either way on the next successful match-and-restore.

## Invariants / gotchas

- **Single-slot.** Only the most recent follow is remembered. Following a second backlink before returning overwrites the slot. This matches the user's mental model — "the last place I came from" is what `h` should restore.
- **Path-keyed lifetime, not time-keyed.** A stale `returnCursor` is harmless: it sits there until either the user returns to its `sourceFile` (consumed) or some unrelated path eventually matches `sourceFile` (rare; restoration would still be valid because the cached cursor was at that file's backlink list).
- **Cursor is clamped on restore.** If the vault refreshed between follow and return and the selected backlink no longer exists, the cursor lands on a neighbor. Test: `TestReturnCursor_ClampsToShrunkList` in `internal/tui/backlinks_test.go`.
- **Surface restoration matters.** A user who followed from a modal expects to land back in a modal, not in the pane. The slot records which surface was active at follow time; the restore branches on it.
- **The pane being closed at return time is fine.** If the user closed the pane between follow and return, `m.backlinks.open` is false; we don't reopen it. Cursor is still restored in case they reopen later.
- **Vault refresh between follow and return is also fine.** `refreshBacklinks` re-queries; the clamp handles list-shrink.

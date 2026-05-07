# Backlinks navigation — design

**Status:** spec — not yet implemented.
**Scope:** make the existing backlinks surfaces (persistent pane via `b`, modal via `B`) interactive — cursor, follow, return.
**Out of scope:** block reference (`^block-id`) position resolution, pre-selecting the matching inline link in the rendered source, broken-links tally view. These belong to the parent spec's Phase 2.

See also: [docs index](../../index.md), [architecture](../../architecture.md), parent spec [wikilinks-and-backlinks-design](2026-05-07-wikilinks-and-backlinks-design.md).

## Motivation

The parent wikilinks-and-backlinks spec describes Phase 1 as including "Following a backlink: `Enter` calls `openFile(SourceFile)`" — but only the *display* half of Phase 1 actually shipped. The persistent pane and modal render entries from `vault.Backlinks(currentPath)`, but `j`/`k` do not move a cursor through them and `Enter` does not follow.

This spec closes that gap with one small extension beyond what the parent specced: when the user follows a backlink with `Enter`, the source file's viewport scrolls near the line that contains the reference, so the user lands "on" the reference rather than at the top of an unfamiliar file. And when the user navigates back via `h`, the backlink cursor is restored so they can resume scanning the list.

The vault layer is already sufficient — `Backlink.SourceFile` and `Backlink.Line` are populated. All work is in `internal/tui`.

## Architecture

No new packages. All changes are confined to `internal/tui`. The dependency graph established by the parent spec is unchanged.

## Components

### `internal/tui` (changes)

State additions on `Model`:

```go
backlinks    []vault.Backlink  // currently displayed entries; cached so cursor moves don't re-query the vault
prevFocus    focus             // saved when opening a backlinks surface, restored on close
returnCursor *returnCursor     // set on follow, consumed on the next matching Back navigation
```

```go
// returnCursor remembers where the user was in the backlinks list
// before following a backlink. Single-slot: we only restore on the
// next Back navigation, and only if it lands on the file we recorded.
type returnCursor struct {
    sourceFile string             // the file whose backlinks were being navigated
    cursor     int                // backlinkCursor at follow time
    surface    backlinksSurface   // pane vs modal
}

type backlinksSurface int

const (
    surfacePane backlinksSurface = iota
    surfaceModal
)
```

`backlinkCursor int` already exists on the model. This work makes it active.

The existing `focus` type gains a third value:

```go
type focus int

const (
    focusTree focus = iota
    focusContent
    focusBacklinks  // new
)
```

`focusBacklinks` is only meaningful when the persistent pane is open and visible (`m.shouldShowBacklinks()` is true). The modal does not use the `focus` field — while a modal is open, `handleKey` already routes to the modal branch directly; we'll add cursor and Enter handling inside that branch.

### Input handling

**Persistent pane (`m.focus == focusBacklinks`):**

| Key | Action |
|---|---|
| `j` / `↓` | `backlinkCursor++`, clamped to `len(m.backlinks)-1` |
| `k` / `↑` | `backlinkCursor--`, clamped to 0 |
| `Enter` | follow the selected backlink |
| `Esc` | return focus to `prevFocus`; pane stays open |
| `b` | close the pane; focus restored to `prevFocus` (existing toggle) |
| `Tab` | extends to a three-way cycle: tree → content → backlinks → tree |

**`Esc` priority order** (extending the existing chain documented in CLAUDE.md):

1. If a modal is open → close it.
2. Else if `m.focus == focusBacklinks` → restore `prevFocus`.
3. Else if `m.linkCursor >= 0` → clear it.
4. Else → no-op.

**Backlinks modal (`m.modalOpen == modalBacklinks`):**

| Key | Action |
|---|---|
| `j` / `↓` | `backlinkCursor++` |
| `k` / `↑` | `backlinkCursor--` |
| `Enter` | follow the selected backlink (also closes the modal) |
| `Esc` | close modal (existing); focus restored to `prevFocus` |
| `B` | toggle modal closed (existing) |

The modal branch in `handleKey` today does `m.modalVP, cmd = m.modalVP.Update(msg)` for any non-Esc key, which gives free scroll for the logs modal. We replace that fall-through with explicit cursor handling *only* when `m.modalOpen == modalBacklinks`. The logs modal keeps the existing `modalVP.Update` fall-through. Modal scroll for long backlink lists comes from `ensureCursorVisible` (below) — same pattern Bubble Tea list components use.

**Opening either surface saves and switches focus:**

```go
case key.Matches(msg, m.keys.ToggleBacklinks):  // 'b'
    if m.backlinksOpen {
        m.backlinksOpen = false
        m.focus = m.prevFocus
    } else {
        m.backlinksOpen = true
        m.refreshBacklinks(m.history.Current())
        m.prevFocus = m.focus
        m.focus = focusBacklinks
        m.backlinkCursor = 0
    }
```

`B` follows the same pattern but for the modal — no `focus` change since the modal owns input directly while open, but `prevFocus` is still saved so Esc/B-close restores it consistently.

### Following and returning

**Follow flow (`Enter` on a selected backlink):**

```go
func (m *Model) followBacklink() {
    if m.backlinkCursor < 0 || m.backlinkCursor >= len(m.backlinks) {
        return
    }
    bl := m.backlinks[m.backlinkCursor]

    // Save return state BEFORE openFile mutates history.
    m.returnCursor = &returnCursor{
        sourceFile: m.history.Current(),
        cursor:     m.backlinkCursor,
        surface:    m.activeBacklinksSurface(),
    }

    // Close modal if active; persistent pane stays open and re-populates
    // for the new file's own backlinks.
    if m.modalOpen == modalBacklinks {
        m.modalOpen = modalNone
    }
    m.focus = focusContent

    m.openFile(bl.SourceFile)
    m.scrollToLine(bl.Line)
}
```

`scrollToLine(n int)` sets `m.viewport.YOffset` so line `n` of the rendered output is positioned about 25% from the top of the viewport (typical "show context above" placement). If `n` exceeds the rendered line count, we clamp to the last line.

**Caveat: `Backlink.Line` is a source-file line, not a rendered-output line.** For most prose markdown these are 1:1, but headings, code blocks, and front-matter can shift them. The simple count-from-top approach is approximate; the user lands "near" the reference, not exactly on it. This is acceptable for Phase 1 — the snippet shown in the backlinks pane already gives enough context to find the exact reference visually. A future improvement could plumb a "find this snippet text in the rendered output" search through `markdown.RenderWithLinks`, but that's a real coordinate-system project and belongs to its own spec.

**Return flow (`h` / Back):**

`h` already calls `m.history.Back()` and `m.refreshContent(path)`. We add a check after the refresh:

```go
if m.returnCursor != nil && path == m.returnCursor.sourceFile {
    m.refreshBacklinks(path)
    m.backlinkCursor = clamp(m.returnCursor.cursor, 0, len(m.backlinks)-1)

    switch m.returnCursor.surface {
    case surfacePane:
        if m.shouldShowBacklinks() {
            m.focus = focusBacklinks
        }
    case surfaceModal:
        m.modalOpen = modalBacklinks
        m.refreshBacklinksModal(path)
    }
    m.returnCursor = nil
}
```

The single-slot `returnCursor` is consumed on use. If the user navigates Back twice, Forward, or to an unrelated file via the tree before returning, the slot is discarded — no stale cursor restoration.

### Visual feedback for the cursor

`formatBacklinks` gains a `cursor int` parameter (`-1` for no selection). The selected entry's two rows are wrapped with a left-edge marker (`▌`) in an accent color, distinct from the existing snippet highlight (yellow) which the parent spec reserved for the matched display text within the snippet.

Selected:
```
▌ relative/path/to/source.md:42
▌   …snippet with [[Note]] highlighted in yellow…
```

Unselected:
```
  relative/path/to/source.md:42
    …snippet…
```

`ensureCursorVisible` adjusts the relevant viewport's `YOffset` so the cursor's two rows are within view. Standard pattern: if the cursor row is above viewport top, scroll up; if below bottom, scroll down. Called after every cursor mutation.

## Data flow

**`b` pressed (open):**
1. `m.backlinksOpen = true`.
2. `m.prevFocus = m.focus`; `m.focus = focusBacklinks`; `m.backlinkCursor = 0`.
3. `m.refreshBacklinks(currentPath)` — caches `m.backlinks` and re-renders the pane.

**`j`/`k` in pane:**
1. Mutate `m.backlinkCursor`, clamp.
2. Re-render pane content (cheap — `formatBacklinks` over cached `m.backlinks`).
3. `ensureCursorVisible` on `m.backlinksVP`.

**`Enter` in pane:**
1. Save `m.returnCursor` from current state.
2. Set `m.focus = focusContent`.
3. `m.openFile(bl.SourceFile)` — pushes onto history, refreshes content, repopulates persistent pane for the new file (different list).
4. `m.scrollToLine(bl.Line)`.

**`h` (Back) after follow:**
1. `m.history.Back()` and `m.refreshContent(path)` (existing).
2. If `m.returnCursor != nil && path == m.returnCursor.sourceFile`: re-populate backlinks for this file, clamp and restore `m.backlinkCursor`, restore the surface (pane focus or reopen modal), clear `m.returnCursor`.

**`Enter` in modal:** same as pane plus `m.modalOpen = modalNone` before `openFile`.

**Three-way `Tab`:** when the pane is open and visible, Tab cycles tree → content → backlinks → tree. When closed or suppressed, Tab stays two-way (tree → content → tree).

## Error handling

| Failure | Behavior |
|---|---|
| Vault refreshed between follow and return; selected backlink no longer exists | `m.backlinkCursor` clamped to `len(m.backlinks)-1` on restore. Cursor lands on a neighbor. |
| Backlinks list empty when pane opens | Pane shows "(no backlinks)" (existing). `m.backlinkCursor = 0` but `Enter` no-ops because the bounds check (`m.backlinkCursor >= len(m.backlinks)`) fails. `j`/`k` are no-ops by the clamp. |
| Pane was closed by user between follow and return | `m.backlinksOpen` is false on return; we do not reopen it. Cursor is still restored in case the user reopens later. |
| Logs modal opened (`?`) between follow and return | On return, if `returnCursor.surface == surfaceModal`, opening the backlinks modal swaps the logs modal out (existing single-modal-swap invariant). |
| Source file no longer exists at follow time | `openFile` emits a status error and aborts (existing behavior). `returnCursor` was set before `openFile`, but since no navigation happened, the slot remains; the next successful Back will check it against the unchanged current file and clear it harmlessly. |
| `Backlink.Line` exceeds rendered line count | Clamp to last rendered line. |

## Testing

`internal/tui/backlinks_test.go` extensions:

| Test | Checks |
|---|---|
| `TestBacklinksPane_OpenFocusesIt` | pressing `b` while focused on tree leaves `prevFocus = focusTree` and `focus = focusBacklinks` |
| `TestBacklinksPane_CloseRestoresFocus` | pressing `b` again restores `focus = focusTree` |
| `TestBacklinksPane_CursorMovement` | `j`/`k` move `backlinkCursor`, clamp at boundaries |
| `TestBacklinksPane_EnterFollows` | `Enter` calls `openFile(bl.SourceFile)`; history records the visit; viewport `YOffset` near `bl.Line` |
| `TestBacklinksPane_BackRestoresCursor` | follow → `h` → cursor restored, `focus = focusBacklinks`, `returnCursor == nil` |
| `TestBacklinksModal_EnterFollowsAndCloses` | modal version: `Enter` follows AND closes modal AND lands focus on content |
| `TestBacklinksModal_BackReopensModal` | follow from modal → `h` → modal reopens, cursor restored |
| `TestReturnCursor_DiscardedOnUnrelatedNav` | follow → tree-click to different file → `h` to original → cursor NOT restored |
| `TestReturnCursor_ClampsToShrunkList` | follow → vault refresh removes the selected backlink → `h` → cursor clamped to new `len-1` |
| `TestScrollToLine` | rendered output of N lines: `scrollToLine(k)` produces `YOffset` such that line k is visible roughly 25% from top |
| `TestThreeWayFocusCycle` | `Tab` cycles tree → content → backlinks → tree; skips backlinks when pane is closed/suppressed |

Existing tests in `backlinks_test.go` (toggle geometry, modal open/close, single-modal invariant) continue to pass. `helpers_test.go`'s `runKey` and existing fixture tree are reused.

## Phasing

Single-shot. Cursor, follow, scroll-to-line, return-cursor, and three-way focus model all ship together — they are tightly coupled by design. A cursor without follow is half-built; follow without return-cursor breaks the "scan through backlinks one at a time" flow.

The parent spec's Phase 2 items remain Phase 2.

## Documentation updates accompanying this work

- `CLAUDE.md` "Wikilinks and backlinks — Phase 1 shipped" line: append "and backlinks navigation (cursor, follow, scroll-to-line, return-cursor)."
- `docs/index.md`: add a link to this spec (already done at the time of writing).
- Parent spec's "Following a backlink" paragraph: replace the "we do not auto-scroll" sentence with a pointer to this doc.
- `internal/tui` changes: `refreshBacklinks` is updated to populate `m.backlinks` (the cached slice this spec adds) before formatting, so cursor moves can re-render without re-querying the vault.

## Open questions / accepted risks

- **Approximate scroll target.** As noted, source-file line numbers don't perfectly correspond to rendered-output line numbers. Acceptable; the snippet provides visual landmark.
- **Three-way Tab is one more state.** The existing tree↔content toggle is symmetric; adding a third state means users have to remember the cycle order. Mitigation: `Tab` skips the backlinks state when the pane isn't visible, so most of the time the cycle is still effectively two-way. The pane being a temporary navigation tool (open → use → close) means most Tab usage doesn't see the third state.
- **Returning to a stale modal.** If the vault rebuilt while the user was on the source page, the restored modal may show a different list than what they followed from. The clamp-on-restore behavior covers correctness but the user could be mildly surprised. Acceptable; rare in practice (vault rebuilds are rare and users typically return quickly).

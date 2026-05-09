# Pre-select inline link on navigation — design

**Status:** spec — not yet implemented.
**Scope:** when arriving at a file via backlink-follow, Back, or Forward, pre-select the inline link in the rendered content that corresponds to the file we just left, so `n`/`p` cycling resumes from a meaningful position and the user has a visible reverse-video target on the page.
**Out of scope:** block reference (`^block-id`) position resolution, broken-links tally view. These are the remaining Phase 2 items from [wikilinks-and-backlinks-design](2026-05-07-wikilinks-and-backlinks-design.md).

See also: [docs index](../../index.md), [architecture](../../architecture.md), parent spec [wikilinks-and-backlinks-design](2026-05-07-wikilinks-and-backlinks-design.md), sibling spec [backlinks-navigation-design](2026-05-07-backlinks-navigation-design.md).

## Motivation

The parent wikilinks-and-backlinks spec lists "pre-selecting the matching inline link in `m.links`" as part of Phase 2. Today, after `Enter` on a backlink, the source file opens and the viewport scrolls near `Backlink.Line`, but `m.content.linkCursor` is reset to -1. The user sees no reverse-video highlight and pressing `n`/`p` starts cycling from the top of the document instead of from the link they were trying to inspect.

The same gap shows up symmetrically on history navigation: pressing `h` to Back, the user lands on the previously-visited file with no memory of which link they followed away from. Most readers expect the link they just clicked to still be highlighted ("breadcrumbed") when they come back to it.

The vault layer is sufficient — `Backlink.SourceFile` (the file we're navigating *to*) and the previous current file (which we're navigating *from*) are both already known. All work is in `internal/tui`.

## Architecture

No new packages. All changes confined to `internal/tui`. The pure-stack `nav` package is untouched. `markdown.Link` and `vault.Backlink` are untouched.

The mechanism is one new field on `Model` — `pendingPreselectTarget string` — which is the absolute path of a file whose inline link should be pre-selected on the next `refreshContent`. Three navigation sources set it; `refreshContent` consumes it and clears it.

```
followBacklink ─┐
Back (h)        ├──> sets m.pendingPreselectTarget ──> refreshContent ──> matches and clears
Forward (^l)    ─┘
```

The single-shot, set-by-leave / consume-by-arrive shape mirrors the existing `m.backlinks.returnCursor` pattern. The two are independent: `returnCursor` restores the *backlinks list cursor*, this one restores the *inline link cursor* on the rendered page.

## Components

### `internal/tui/model.go` — one new field

```go
// pendingPreselectTarget is the absolute path of a file whose inline
// link should be pre-selected on the next refreshContent. Set by any
// navigation that has a meaningful "the link you were looking at"
// notion: backlink-follow, Back, Forward. Cleared by refreshContent
// after consumption (whether or not a match was found).
pendingPreselectTarget string
```

### `internal/tui/content.go` — `refreshContent` consumes it

After `m.content.links` is populated and replacing the existing `linkCursor = -1` reset:

```go
target := m.pendingPreselectTarget
m.pendingPreselectTarget = "" // single-shot — always clear

m.content.linkCursor = -1
if target != "" {
    for i, l := range links {
        if l.Resolved.Kind == markdown.LinkLocalFile && l.Resolved.Target == target {
            m.content.linkCursor = i
            break
        }
    }
}

if m.content.linkCursor >= 0 {
    m.scrollToLink(m.content.links[m.content.linkCursor])
    m.applyLinkHighlight()
}
```

`applyLinkHighlight()` re-renders with the reverse-video splice (existing function); `scrollToLink` ensures the link's row is in the viewport (existing function).

### `internal/tui/backlinks.go` — `followBacklink` sets the target

One new line before `openFile`:

```go
m.pendingPreselectTarget = m.history.Current() // the file we're leaving
m.openFile(bl.SourceFile)
if m.content.linkCursor < 0 {
    m.scrollToLine(bl.Line)
}
```

The new conditional gates the existing `scrollToLine`: when the pre-select succeeded, `refreshContent` already scrolled via `scrollToLink` to the inline link's exact row, which is more accurate than `scrollToLine(bl.Line)` (source line vs rendered row mismatch — see the parent spec's note on approximate scroll). When no inline link matched, `scrollToLine` runs unchanged.

### `internal/tui/dispatch.go` — Back/Forward set it

Both keys peek the current path *before* calling `nav.History.Back()`/`Forward()`, then set the field. The two snippets below are separate handler branches in `dispatch.go` (each is its own `case`), so the `leaving` name is local to its branch:

```go
// Back (h) — handler branch
leaving := m.history.Current()
if path, ok := m.history.Back(); ok {
    m.pendingPreselectTarget = leaving
    m.refreshContent(path)
    m.maybeRestoreReturnCursor(path) // existing
}

// Forward (^l) — separate handler branch
leaving := m.history.Current()
if path, ok := m.history.Forward(); ok {
    m.pendingPreselectTarget = leaving
    m.refreshContent(path)
}
```

Forward does not call `maybeRestoreReturnCursor` (existing behavior preserved — return-cursor is keyed on a single matched Back).

## Data flow

**Backlink follow (`Enter`):**
1. `followBacklink` records `m.backlinks.returnCursor` (existing).
2. `m.pendingPreselectTarget = m.history.Current()`.
3. `m.openFile(bl.SourceFile)` → `history.Visit` → `refreshContent(bl.SourceFile)`.
4. `refreshContent` populates `m.content.links`, finds the first inline link with `Resolved.Kind == LinkLocalFile && Resolved.Target == pendingPreselectTarget`, sets `linkCursor`, runs `scrollToLink` and `applyLinkHighlight` if matched.
5. Back in `followBacklink`: if `linkCursor < 0`, run `scrollToLine(bl.Line)`. Otherwise the inline scroll wins.

**Back (`h`):**
1. `leaving := m.history.Current()`.
2. `m.history.Back()`. If not ok, no-op.
3. `m.pendingPreselectTarget = leaving`.
4. `m.refreshContent(path)` consumes; matches and pre-selects, or clears silently.
5. `m.maybeRestoreReturnCursor(path)` (existing) restores backlinks list cursor independently.

**Forward (`^l`):**

Same shape as Back with `History.Forward()`. Symmetric semantic: leaving file becomes the pre-select target on arrival.

**Watcher-triggered `refreshContent` between leave and arrive:**

The watcher's `refreshContent` consumes (and clears) `pendingPreselectTarget`. The subsequent intentional navigation gets no pre-select. Accepted race — see Error handling.

**Key invariant:** `pendingPreselectTarget` is consumed by every `refreshContent` regardless of whether it matched. This makes the field eagerly self-clearing: stale values can never leak across navigations.

## Error handling

| Failure | Behavior |
|---|---|
| Field set but no inline link matches | `linkCursor` stays -1; no status message (silent fallback). `scrollToLine` runs in `followBacklink` since the gate sees `linkCursor < 0`. |
| Watcher `FileModified` fires `refreshContent` between leave and arrival | Watcher's call consumes (and clears) the field. Next intentional navigation gets no pre-select. Rare; acceptable. |
| `m.history.Current()` returns "" at follow time | Defensive: setting target to "" is harmless because the consumer's `target != ""` guard skips the loop. |
| Two inline links share a target | First in document order wins (Q1 ambiguity rule). Deterministic. |
| `Resolved.Kind != LinkLocalFile` (external URLs, anchors) | Filtered out by the `LinkLocalFile` guard. Backlinks only exist for local files. |
| Back/Forward on empty history | `History.Back()`/`Forward()` returns `ok == false`; field not set. No-op. |
| Path normalization mismatch | Vault `Backlink.SourceFile`, `m.history.Current()`, and `Resolved.Target` all flow through absolute-path-producing ingestion. Equal-by-string works. Documented; tested defensively. |

## Testing

`internal/tui/preselect_test.go` (new file):

| Test | Checks |
|---|---|
| `TestPreselect_FollowBacklink_PicksFirstMatchingLink` | Source file has two links to previous file → `Enter` on backlink → `linkCursor` is index of first one |
| `TestPreselect_FollowBacklink_NoMatchLeavesCursorUnselected` | Backlink to a file with zero matching inline links → `linkCursor == -1`, no status set |
| `TestPreselect_FollowBacklink_InlineScrollOverridesScrollToLine` | Inline match found → viewport `YOffset` reflects `scrollToLink` (link's row), not `scrollToLine(bl.Line)` |
| `TestPreselect_Back_RestoresLink` | A→B→`h` (Back to A) → A's inline link to B is pre-selected |
| `TestPreselect_Forward_RestoresLink` | A→B→`h`→`Ctrl-L` (Forward to B) → B's inline link to A is pre-selected |
| `TestPreselect_ClearedAfterConsumption` | Two consecutive Back ops; second's pre-select is independent of first (no leakage) |
| `TestPreselect_WatcherEventConsumesQuietly` | Set field, fire watcher `FileModified` for current file, then navigate — pre-select doesn't fire. (Documents the accepted race.) |
| `TestPreselect_OnlyLocalFileLinks` | Page contains a non-local link whose Href happens to equal the previous path — not matched (gated on `LinkLocalFile`) |

Existing tests in `links_test.go`, `backlinks_test.go`, and `history_test.go` must continue to pass. The new logic is gated on `target != ""` so the default path (no pending target) is identical to today.

## Phasing

Single shot. All three navigation sources (backlink-follow, Back, Forward) ship together. They share one set/consume mechanism; splitting adds churn without value.

## Documentation updates accompanying this work

- `CLAUDE.md` "Wikilinks and backlinks — Phase 2" line: note that pre-select-inline-link has shipped; remaining Phase 2 items are block reference resolution and broken-links tally.
- `docs/index.md`: add a link to this spec.
- Parent spec `2026-05-07-wikilinks-and-backlinks-design.md` Phase 2 list: cross-reference this spec next to the "pre-selecting the matching inline link" line.

## Open questions / accepted risks

- **Watcher race.** A watcher refresh between leave and arrival silently drops the pre-select. Rare and harmless; documented and tested. If it ever becomes annoying, scoping the field's lifetime to a tea.Cmd dispatched alongside the navigation would close the gap, at the cost of more complex plumbing.
- **First-match-wins is path-only.** Two links in the source file with the same target (e.g., `[A](foo.md)` and `[B](foo.md)`) both match; document order wins. The user has no way to "skip to the second one" via this mechanism, but `n` cycles forward from the selection so they can advance manually.
- **Forward symmetry is conceptually correct but rarely observed.** The Back→Forward pattern is uncommon enough in practice that the Forward case mostly exists for completeness rather than as a primary user flow. We ship it anyway because the leave/consume mechanism is already the same shape; the marginal cost is one extra block in `dispatch.go`.

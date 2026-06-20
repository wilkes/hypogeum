# link-cursor

The integer index that tracks which link in the rendered content pane is currently selected. Bound to `n`/`p`/`Enter`/`Esc` while the right pane has focus.

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [link-following](../link-following.md), [`internal/markdown`](../packages/markdown.md), and [`internal/tui`](../packages/tui.md); press `b` for the full backlinks list.

## Why it exists

The user needs to follow a `[text](path.md)` or `[[wikilink]]` from the right pane without leaving the keyboard. Three approaches were considered:

- **OSC 8 hyperlinks.** Terminal support is uneven; Glamour doesn't emit them.
- **Numbered link picker (modal).** Cheaper but unbrowserlike. Rejected once the sentinel-render trick was proven.
- **Cursor over an in-order link list.** What shipped. `n` next, `p` previous, `Enter` follows, `Esc` clears. Phase 2 added an inline reverse-video highlight around the selected link's byte range so the cursor is visible in the rendered text, not just the footer.

The cursor is a single integer because [[sentinel-render]] guarantees the link list is in document order and every link has a known row in the rendered output.

## How it works

`m.content.linkCursor int` holds the selection. `-1` means no link selected. `m.content.links []markdown.Link` is the document's link list, refreshed on every render.

**Cycling:** `n` increments, `p` decrements, both wrap at the ends. After the move, `scrollToLink` adjusts `m.content.viewport.YOffset` so the selected link's row is in view.

**Following (`Enter` when `m.content.linkCursor >= 0`):** `followLink` (`internal/tui/links.go`) branches on `Resolved.Kind`:
- `LinkLocalFile` — `navigateTo(target)` (records history, moves the tree cursor). If `l.Resolved.Range` is non-nil, `m.content.rangeHighlight` is set before navigating so `refreshContent` scrolls to and reverse-videos the gutter for that range; otherwise the stale highlight is cleared.
- `LinkExternal` — arms a one-keystroke confirm: sets `m.pending.externalURL` and footer `"press Enter again to open: <href>"`. A second `Enter` exec's the platform opener (`open`/`xdg-open`/`cmd start`); any other key cancels. The opener ships (`internal/tui/external.go`).
- `LinkAnchor` — footer `"anchor navigation not implemented: #<anchor>"`. Resolving anchors to heading rows is not built.
- default (unrecognized) — footer `"unrecognized link: <href>"`.

**Clearing (`Esc`):** sets `m.content.linkCursor = -1`. This is one step in the `Esc` priority chain; see [[modal-geometry]] for the full chain.

**Reset on refresh:** every `refreshContent` resets `m.content.linkCursor` to `-1` *unless* the navigation that triggered it set `m.pendingPreselectTarget` to the path of the file being left. In that case the consumer scans the new document's link list for the first `LinkLocalFile` whose `Resolved.Target` matches and sets `linkCursor` to that index — so following a backlink, pressing `h` (Back), or `l` (Forward) lands on a page with the corresponding inline link already selected. The pending field is cleared on every `refreshContent`, matched or not, so it can never leak across navigations. See [pre-select-inline-link-design](../superpowers/specs/2026-05-09-pre-select-inline-link-design.md) for the full rules.

## Invariants / gotchas

- **Content-pane scoped.** `n`/`p`/`Esc` and link-aware `Enter` only fire when `focus == focusContent`. Tree-pane bindings are unaffected.
- **Reset on every `refreshContent`, with a single carry-over knob.** The default is `linkCursor = -1`. The exception is `m.pendingPreselectTarget`: any caller can set it before navigation to ask the next refresh to pre-select the inline link pointing at that path. Pair `m.content.links` and `m.content.linkCursor` or accept stale UI.
- **Footer marker is `→ <target> [k/n]` when selected.** The constant `linkFooterMarker` is package-public for tests to assert on.
- **Unresolved wikilinks aren't in the cycler.** They render as plain text with a `?` suffix — visible to the user but not selectable with `n`/`p`. Intentional: a broken link can't be followed, so adding it to the cycler would be a confusing no-op.
- **Inline highlight replays the cached render — no re-render.** `applyLinkHighlight` (`internal/tui/links.go`) calls `m.content.render.WithHighlight(m.content.linkCursor)` on the cached `*markdown.RenderResult` (set in `refreshContent` via `RenderDocument`). `WithHighlight` is a single `stripSentinels` pass over the retained sentinel-intact `raw` with `HighlightMarker(i)` applied — no file read, no Glamour render. The viewport's `YOffset` is saved and restored around the swap so scroll position survives. The cached handle is `nil` for code-file and error-state documents, where `applyLinkHighlight` is a no-op (those documents have no cyclable links anyway). This is the `2026-06-20-link-cycle-render-cache` change; see [[sentinel-render]] for the render/highlight split on the markdown side.

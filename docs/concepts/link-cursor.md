# link-cursor

The integer index that tracks which link in the rendered content pane is currently selected. Bound to `n`/`p`/`Enter`/`Esc` while the right pane has focus.

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [link-following](../link-following.md), [`internal/markdown`](../packages/markdown.md), and [`internal/tui`](../packages/tui.md); press `b` for the full backlinks list.

## Why it exists

The user needs to follow a `[text](path.md)` or `[[wikilink]]` from the right pane without leaving the keyboard. Three approaches were considered:

- **OSC 8 hyperlinks.** Terminal support is uneven; Glamour doesn't emit them.
- **Numbered link picker (modal).** Cheaper but unbrowserlike. Rejected once the sentinel-render trick was proven.
- **Cursor over an in-order link list.** What shipped. `n` next, `p` previous, `Enter` follows, `Esc` clears. The cursor is footer-only in Phase 1; Phase 2 adds inline highlight by re-splicing SGR around the selected link's byte range.

The cursor is a single integer because [[sentinel-render]] guarantees the link list is in document order and every link has a known row in the rendered output.

## How it works

`m.content.linkCursor int` holds the selection. `-1` means no link selected. `m.content.links []markdown.Link` is the document's link list, refreshed on every render.

**Cycling:** `n` increments, `p` decrements, both wrap at the ends. After the move, `scrollToLink` adjusts `m.content.viewport.YOffset` so the selected link's row is in view.

**Following (`Enter` when `m.content.linkCursor >= 0`):** branches on `Resolved.Kind`:
- `LinkLocalFile` — `openFile(target)` plus `selectInTree(target)`. Records history; moves the tree cursor if the path is in the tree.
- `LinkExternal` — Status bar: `"external link not opened: <href>"`. Phase 3 will hand off to `xdg-open`/`open` after a confirm flow.
- `LinkAnchor` — Status bar: `"anchor navigation not implemented"`. Phase 2 will resolve to a heading row.
- `LinkInvalid` — Status bar: `"unrecognized link"`.

**Clearing (`Esc`):** sets `m.content.linkCursor = -1`. This is one step in the `Esc` priority chain; see [[modal-geometry]] for the full chain.

**Reset on refresh:** every call to `refreshContent` (history navigation, file open, watcher refresh, resize) resets `m.content.linkCursor` to `-1`. The link cursor is per-document; it doesn't survive a navigation. A link list from a document the user is no longer viewing would point at a dead row.

## Invariants / gotchas

- **Content-pane scoped.** `n`/`p`/`Esc` and link-aware `Enter` only fire when `focus == focusContent`. Tree-pane bindings are unaffected.
- **Reset on every `refreshContent`.** Pair `m.content.links` and `m.content.linkCursor` or accept stale UI.
- **Footer marker is `→ <target> [k/n]` when selected.** The constant `linkFooterMarker` is package-public for tests to assert on.
- **Unresolved wikilinks aren't in the cycler.** They render as plain text with a `?` suffix — visible to the user but not selectable with `n`/`p`. Intentional: a broken link can't be followed, so adding it to the cycler would be a confusing no-op.
- **Phase 1 has no inline highlight.** The cursor is footer-only. The rendered text doesn't change when the cursor moves. Phase 2 adds the highlight via SGR re-splicing.

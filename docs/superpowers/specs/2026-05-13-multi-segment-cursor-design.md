# Multi-segment cursor for word-wrapped links — design

**Status:** shipped.
**Scope:** when the selected link's visible text wraps across multiple terminal rows, every visual segment is highlighted with reverse-video SGR. Today only the first segment is highlighted; subsequent rows render unhighlighted because Glamour resets SGR at line boundaries.

See also: [link-following](../../link-following.md), [docs index](../../index.md), [[sentinel-render]], [[link-cursor]], [link-following Phase 2 design](2026-05-09-link-following-phase-2-design.md).

## What's broken today

`HighlightMarker` (shipped in Phase 2) returns `\e[7m` / `\e[27m` and `stripSentinels` splices those bytes in place of `\x1c` / `\x1e`. For a link whose visible text fits on one row this looks right.

For a link that wraps, Glamour emits a fresh per-row style prelude that ends in `\e[0m`, which cancels reverse-video. The next row's prelude does not include `\e[7m`, so only the first segment is visibly highlighted. Reproduced with a probe:

```
[row 1] ... \e[7m<first fragment> \e[0m   ← reverse opens, then line tail \e[0m kills it
[row 2] \e[0m\e[38;5;252;4m\e[0m  <second fragment> \e[0m   ← no \e[7m, no highlight
[row 3] \e[0m\e[38;5;252;4m\e[0m  <third fragment>\e[27m\e[0m   ← stray \e[27m at the very end
```

The `\e[27m` only fires when `stripSentinels` finally hits `\x1e`, long after the line tails have already cancelled the highlight.

## Fix

Treat each row inside a sentinel-bracketed span as its own SGR scope:

- When the byte before a `\n` was emitted with `openEmit=true` (i.e. the line had highlighted content), write `closeMark` *before* the `\n` so the reverse-video closes cleanly.
- Reset `openEmit` to `false` on every `\n` while `inLink`. The existing lazy `openMark` emission in the default and `\n` branches then re-opens reverse-video on the first content byte of the next row.

That gives:

```
[row 1] ... \e[7m<first fragment>\e[27m \e[0m
[row 2] \e[0m\e[38;5;252;4m\e[0m\e[7m  <second fragment>\e[27m \e[0m
[row 3] \e[0m\e[38;5;252;4m\e[0m\e[7m  <third fragment>\e[27m\e[0m
```

The reverse-video bytes now bracket each row's link-text contribution.

### Indent included on continuation rows

Glamour wraps inside a 2-column left margin. When highlight reopens at the start of row 2, the lazy emit fires on the first non-escape byte — which is the leading indent space, not the first letter. As a result the indent is covered by reverse-video on continuation rows.

This is consistent with terminal text selection (`less`, `vim` visual mode) and *not* consistent with browser selection (which clips to glyphs). The trade is simplicity: detecting "first non-space content byte after escapes" introduces a state machine that complicates a hot path already tricky enough to have a regression test (`TestRenderWithLinks_OutputIsCleanRender`). Accepted: continuation rows include indent in the highlight.

### No change to spans, links, or scroll math

`sentinelSpan.row` still records the first row of the link (where the cursor was emitted). Auto-scroll already uses this. The link list, footer indicator, and viewport scroll behaviour are unchanged.

## What's not changing

- `LinkMarker` signature — still `func(linkIndex int) (open, close string)`. The fix is in `stripSentinels`, not in markers.
- The sentinel bytes themselves.
- BubbleZone markers — they're emitted in the same place (`openMark`/`closeMark`). If we ever wire BubbleZone back in, the markers will *now* be emitted once per row, which matches BubbleZone's per-row zone model (it already records each row independently for hit-testing). So the change is also forward-compatible with the dormant mouse-hit-test path.

## Tests

New unit test in `internal/markdown/links_render_test.go`:

```go
func TestHighlightMarker_WrappedLinkHighlightsEverySegment(t *testing.T) {
    in := "a\x1cone\ntwo\nthree\x1e b"
    marker := HighlightMarker(0)
    cleaned, _ := stripSentinels(in, marker)
    // Three rows; each row's link contribution wrapped in \e[7m...\e[27m.
    want := "a\x1b[7mone\x1b[27m\n\x1b[7mtwo\x1b[27m\n\x1b[7mthree\x1b[27m b"
    if cleaned != want {
        t.Errorf("multi-segment highlight: got %q want %q", cleaned, want)
    }
}
```

Existing tests stay green:
- `TestHighlightMarker_SelectedLinkGetsReverseVideo` (single-row link, no `\n` between sentinels) still produces `\e[7mbar\e[27m`.
- `TestStripSentinels_LinkWrappingTwoLines` only asserts on `cleaned`/`spans` when `marker == nil`; the `openEmit` plumbing is unaffected by a nil marker.
- `TestRenderWithLinks_OutputIsCleanRender` runs with `marker == nil`; no behavioural change.

## Open questions / accepted risks

- **Continuation-row indent is reversed.** See above — accepted.
- **Glamour line tails contain `\e[0m`.** We rely on the lazy `openMark` re-emit, *not* on patching Glamour's tail. If Glamour ever emits a row whose link content is followed by additional same-row bytes (e.g. continuation text on the same line after `\x1e`), the existing closing path stays correct.
- **Single-line links unchanged.** A link without an embedded `\n` between sentinels takes the same code path as today.

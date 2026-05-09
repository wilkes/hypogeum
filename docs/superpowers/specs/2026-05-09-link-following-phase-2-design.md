# Link following Phase 2 — inline highlight

**Status:** spec — not yet implemented.
**Scope:** when the user cycles links with `n`/`p`, the selected link's text is highlighted inline in the rendered content pane (reverse-video SGR). Phase 1 showed selection only in the footer; Phase 2 closes that gap.

See also: [link-following](../../link-following.md), [docs index](../../index.md), [[sentinel-render]], [[link-cursor]].

## What's changing

No new packages, no new state fields, no new sentinel bytes. The sentinel infrastructure introduced in Phase 1 already accepts a `LinkMarker` callback (`func(linkIndex int) (open, close string)`) that splices arbitrary bytes around each link's visible text during `stripSentinels`. Phase 1 uses this for BubbleZone mouse zones. Phase 2 reuses the same hook to inject SGR reverse-video codes around the selected link only.

## Architecture

### `internal/markdown/links_render.go` (new helper)

Add `HighlightMarker(selected int) LinkMarker`:

```go
func HighlightMarker(selected int) LinkMarker {
    return func(i int) (string, string) {
        if i == selected {
            return "\x1b[7m", "\x1b[27m" // SGR reverse-video on/off
        }
        return "", ""
    }
}
```

Reverse video (`\x1b[7m` / `\x1b[27m`) is chosen because it works on any terminal color theme without color picking and is the standard idiom for selection highlights in terminal programs (less, vim search, etc.).

### `internal/tui/links.go` (new helper + updated cycleLink)

Add `applyLinkHighlight()`:

```go
func (m *Model) applyLinkHighlight() {
    path := m.history.Current()
    if path == "" {
        return
    }
    src, err := os.ReadFile(path)
    if err != nil {
        m.status = err.Error()
        return // leave viewport unchanged; existing plain render stays visible
    }
    m.content.renderer.SetFromFile(path)
    out, _, err := m.content.renderer.RenderWithLinks(string(src), path, markdown.HighlightMarker(m.content.linkCursor))
    if err != nil {
        m.status = err.Error()
        return
    }
    offset := m.content.viewport.YOffset
    m.content.viewport.SetContent(out)
    m.content.viewport.SetYOffset(offset) // preserve position scrollToLink just set
}
```

Update `cycleLink` to call `applyLinkHighlight()` after `scrollToLink`:

```go
func (m *Model) cycleLink(step int) {
    // ... existing cursor update ...
    m.scrollToLink(m.content.links[m.content.linkCursor])
    m.applyLinkHighlight()
}
```

### `internal/tui/input.go` — Esc handler

The existing Esc handler already calls `refreshContent(currentPath)` to clear the link cursor. Add scroll-position preservation:

```go
// before:
m.content.linkCursor = -1
m.refreshContent(m.history.Current())

// after:
offset := m.content.viewport.YOffset
m.content.linkCursor = -1
m.refreshContent(m.history.Current())
m.content.viewport.SetYOffset(offset)
```

`refreshContent` is unchanged — it always renders with `linkZoneMarker` (BubbleZone mouse hit-testing) and no highlight marker, producing a plain view. This means every code path that opens a file (follow link, tree click, history navigation) naturally clears the highlight without special cases.

## Data flow

**`n`/`p` pressed:**
1. `cycleLink(±1)` updates `m.content.linkCursor`
2. `scrollToLink` adjusts `viewport.YOffset` so the link row is visible
3. `applyLinkHighlight()` reads source, re-renders with `HighlightMarker(cursor)`, calls `viewport.SetContent`, restores `YOffset`

**`Esc` pressed:**
1. Save `offset := m.content.viewport.YOffset`
2. `m.content.linkCursor = -1`
3. `refreshContent(currentPath)` — drops highlight, internally calls `GotoTop`
4. `m.content.viewport.SetYOffset(offset)` — restore position

**File open / history navigation / tree click:**
- All call `refreshContent`, which renders plain (no highlight marker) and resets `linkCursor = -1`. No special cases needed.

## Error handling

| Failure | Behavior |
|---|---|
| `os.ReadFile` fails in `applyLinkHighlight` | Set `m.status` to error string; leave viewport content unchanged (existing plain render stays visible). Cursor is already updated — user can press Esc to clear. |
| `RenderWithLinks` fails in `applyLinkHighlight` | Same: set `m.status`, leave viewport unchanged. |

## Testing

**`internal/markdown/links_render_test.go`:**
- `TestHighlightMarker_SelectedLinkGetsReverseVideo` — `HighlightMarker(1)` applied via `stripSentinels` wraps link index 1 with `\x1b[7m`/`\x1b[27m`; link index 0 is unwrapped.

**`internal/tui/links_test.go`:**
- `TestCycleLink_HighlightsSelectedLink` — after pressing `n`, `m.View()` contains `\x1b[7m` somewhere in the content pane output.
- `TestCycleLink_ClearOnEsc` — after `n` then `Esc`, `m.View()` does not contain `\x1b[7m`.
- `TestCycleLink_PreservesScrollOnHighlight` — `YOffset` after `n` matches what `scrollToLink` set, not zero.
- `TestCycleLink_PreservesScrollOnEsc` — `YOffset` after `Esc` matches the offset before Esc was pressed.

## What's not in scope

- Multi-segment highlight for word-wrapped links (the sentinel spans the wrap; a single SGR open/close will highlight both segments naturally, so this may just work — verify during implementation).
- External URL launch (`open`/`xdg-open`) — Phase 3.
- Anchor scroll within the same document — Phase 3.

# Keyboard selection (vim visual mode) — design

**Status:** designed, not yet implemented.

## Goal

Let the user select and copy content-pane text with the keyboard, in addition
to the existing mouse drag-to-select. The interaction model is **vim visual
mode**: press `v` to drop an anchor and reveal a text caret, move the caret to
extend a character-precise selection, copy with the dialect's copy key, and
cancel with `Esc`.

Both keybinding dialects (`pager`, `modern`) get the same modal visual mode —
one interaction model, one set of code paths. `modern` moves the caret with
arrow keys (it has no `h`/`j`/`k`/`l`) and yanks with `^y`; `pager` uses
`h`/`j`/`k`/`l` and `y`.

## Why this approach

The existing mouse selection already models everything we need. The
`selection{anchor, cursor cellPos, ...}` struct in `internal/tui/content.go`
records a span; `applySelectionHighlight` and `extractSelection` are **purely
position-based** — they don't care whether the positions came from a mouse or a
keyboard. So keyboard selection is mostly a new *input source* feeding the same
span machinery, not a new selection system.

Rejected alternatives:

- **Separate caret subsystem** (a `caret cellPos` independent of `selection`,
  with its own render/movement logic): cleaner separation of "keyboard caret"
  vs "mouse drag," but duplicates the highlight/extract logic and adds a
  parallel concept to keep in sync. More code, more drift risk.
- **Modal mode** (`modalVisual` through the existing modal dispatch): visual
  mode has no overlay box and needs the content visible underneath, so it
  fights the single-modal overlay/`prevFocus` invariant. Wrong tool.

## State

One new field on the existing `selection` struct
(`internal/tui/content.go`); the caret *is* `selection.cursor`:

```go
type selection struct {
    anchored    bool    // a left-press landed in the content pane (mouse)
    moved       bool    // motion seen since press (mouse click-vs-drag)
    copied      bool    // released/yanked with text → highlight persists
    visual      bool    // NEW: keyboard visual mode is active
    anchor      cellPos // where the selection started
    cursor      cellPos // current end / caret position
    pendingLink int     // link index under a mouse press, or -1
}
```

`visual` distinguishes a keyboard selection-in-progress from a mouse drag
(`anchored`) and a finalized copy (`copied`), so dispatch and repaint logic can
tell them apart.

## Keybinding + dispatch

- **One new keyMap field:** `EnterVisual`, bound to `v` in both `pagerKeys()`
  and `modernKeys()` (`internal/tui/keys.go`). `v` is currently unbound in both
  dialects (verified against `TestKeys_NoOverlappingActions`).
- **Entering visual mode:** in the content-key handler, when
  `key.Matches(msg, m.keys.EnterVisual)` and no modal is open, place the caret
  at the top-left of the visible area (`line = viewport.YOffset`, `col = 0`),
  set `visual = true, anchored = true, anchor = cursor`, and paint the caret.
  Ignored while any modal is open (selection is content-pane-only, the same
  rule mouse selection follows).
- **While `visual` is active:** a new `handleVisualKey(msg)` runs *first* in
  `handleKey` — before the global Back/Forward switch — the same precedence
  trick the tree modal uses to shadow history keys, so `h`/`l` don't trigger
  Back/Forward and `j`/`k` don't fall through to the viewport. It reuses
  existing keyMap fields rather than adding new ones:

| Key (pager / modern) | keyMap field reused | Action                       |
| -------------------- | ------------------- | ---------------------------- |
| `h j k l` / arrows   | `Back Down Up Forward` | move caret by char / line |
| `g` / `G`            | `Top` / `Bottom`    | caret to doc top / bottom    |
| `^d` / `^u`          | `HalfPageDown/Up`   | caret ± half-page            |
| `y` / `^y`           | `CopyPath`          | **yank** selection, then exit |
| `Esc`                | `ClearLink`         | cancel, exit                 |

The only new binding is `v`. Yank reuses the dialect's copy key: in visual mode
it copies the *selection*; outside visual mode it still copies the file path.

## Caret movement + scrolling

- A `moveCaret(dline, dcol)` helper updates `selection.cursor`, clamped to valid
  cells with the same bounds logic as `screenToContent`: line to
  `[0, len(lines)-1]`, col to `[0, lineWidths[line]]`.
- Horizontal motion at a line edge stays on the line (no line-wrapping of
  `h`/`l`) — simplest and predictable. Vertical motion clamps the column to the
  target line's width.
- **Auto-scroll:** after a move, if the caret's line falls outside
  `[YOffset, YOffset+Height)`, scroll the viewport so it's visible (reusing the
  existing scroll-to-line offset math). `g`/`G`/`^d`/`^u` set the caret and
  scroll together.
- Each move calls `applySelectionHighlight()` to repaint.

## Rendering the caret + selection

- The span `anchor..cursor` is highlighted by the **existing**
  `applySelectionHighlight` (reverse-video), unchanged for the multi-cell case.
- **One tweak** for caret visibility: when the span is zero-width (`hi <= lo` —
  just-entered or collapsed), render a single reverse-video caret cell at
  `cursor`, using the character under the caret or a synthesized space at
  end-of-line. Today that branch renders nothing, which would make a freshly
  placed caret invisible.
- The caret is simply the active (moving) end of the highlighted span — we do
  not draw a separate caret glyph. Keeps rendering to one code path; a distinct
  caret style can be added later if desired.

## Yank / cancel lifecycle

- **Yank** (`y` / `^y`): `extractSelection()` → `copyToClipboard(text)` → toast
  `Copied N chars` via `m.diag.Info` → `finalizeSelection()` (sets
  `copied = true`, clears `visual`/`anchored`). The reverse-video highlight
  **persists** after yank until the next keystroke / press / navigation —
  matching mouse-drag behavior. A zero-width yank copies nothing and just exits.
- **Cancel** (`Esc`): `clearSelection()` and exit. `clearSelection`'s repaint
  condition gains `|| visual` so cancelling a never-moved caret still restores
  the clean render.
- **Implicit exit:** any navigation or `setContent` already calls
  `resetSelectionState`, so opening a file, following a link, or a resize all
  drop visual mode cleanly. No new teardown paths.

## Interactions & edge cases

- **Modals:** `v` is ignored while a modal is open; modal-opening keys aren't
  handled inside `handleVisualKey`, so there's no conflict with the
  single-modal invariant.
- **Link cycling:** `n`/`p`/`Enter` are not handled in `handleVisualKey`, so
  they're inert while selecting — visual mode and link-cycling never overlap.
- **Code files & directory listings:** selection operates on
  `content.rendered`, so visual mode works on any rendered content for free.
- **Empty / short docs:** the caret clamps to the last line; `g`/`G` on a
  one-line doc are no-ops.

## Testing

Model-level tests in `internal/tui/` (no terminal needed), mirroring the
existing selection tests:

- Enter visual → caret at top-left; highlight shows one cell.
- Movement: `l`/`j` extend; `extractSelection` returns the expected substring;
  caret clamps at edges.
- Jumps: `G` then yank copies to end; `^d` moves half-page and scrolls.
- Yank: clipboard receives the text, footer shows `Copied N chars`, highlight
  persists, `visual` is cleared.
- `Esc` cancels and restores the clean render.
- `v` is a no-op while a modal is open.
- Dialect coverage: `v` enters in both pager and modern; `^y` yanks in modern,
  `y` in pager.
- Regression: `y` outside visual still copies the path; `h`/`l` outside visual
  still navigate history.

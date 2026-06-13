# Keyboard selection (vim visual mode) — design

**Status:** shipped.

## Goal

Let the user select and copy content-pane text with the keyboard, in addition
to the existing mouse drag-to-select. The interaction model is a **two-phase
vim-style visual mode**, so the user can choose where the selection starts:

1. **Positioning phase** — press `v` to reveal a movable text caret (starting
   at the top-left of the visible area). Move it freely; nothing is selected
   yet.
2. **Select phase** — press `Space` to drop the anchor at the caret. Now moving
   the caret extends a character-precise selection from that anchor.

Copy the selection with the dialect's copy key, or cancel with `Esc` (which
exits the whole mode from either phase).

Both keybinding dialects (`pager`, `modern`) get the same modal flow — one
interaction model, one set of code paths. `modern` moves the caret with arrow
keys (it has no `h`/`j`/`k`/`l`) and yanks with `^y`; `pager` uses
`h`/`j`/`k`/`l` and `y`. `Space` and `Esc` are the same in both.

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

Two new fields on the existing `selection` struct
(`internal/tui/content.go`); the caret *is* `selection.cursor`:

```go
type selection struct {
    anchored    bool    // a left-press landed in the content pane (mouse)
    moved       bool    // motion seen since press (mouse click-vs-drag)
    copied      bool    // released/yanked with text → highlight persists
    visual      bool    // NEW: keyboard visual mode is active (either phase)
    selecting   bool    // NEW: anchor dropped (Space) → movement extends
    anchor      cellPos // where the selection started
    cursor      cellPos // current end / caret position
    pendingLink int     // link index under a mouse press, or -1
}
```

`visual` distinguishes a keyboard interaction from a mouse drag (`anchored`)
and a finalized copy (`copied`). `selecting` splits the two keyboard phases:

- `visual && !selecting` → **positioning**: moving the caret moves *both*
  `anchor` and `cursor` together, so there is no span — only the caret cell.
- `visual && selecting` → **extending**: `Space` froze `anchor`; moving the
  caret moves only `cursor`, growing the span.

## Keybinding + dispatch

- **Two new keyMap fields:**
  - `EnterVisual`, bound to `v` in both `pagerKeys()` and `modernKeys()`
    (`internal/tui/keys.go`). `v` is currently unbound in both dialects.
  - `BeginSelect`, bound to `Space` (`" "`) in both dialects. `Space` is also
    bound to `ToggleFolder`, but the two are context-multiplexed (Space means
    expand/collapse only in the tree modal, begin-select only in visual mode —
    never both at once), exactly like the existing `^j`/`^k` picker-vs-search
    overlap. Whitelist the `(BeginSelect, ToggleFolder)` pair on `" "` in
    `isAllowedKeyOverlap` so `TestKeys_NoOverlappingActions` stays green.
- **Entering visual mode (`v`):** in the content-key handler, when
  `key.Matches(msg, m.keys.EnterVisual)` and no modal is open, place the caret
  at the top-left of the visible area (`line = viewport.YOffset`, `col = 0`),
  set `visual = true, selecting = false, anchor = cursor`, and paint the caret.
  Ignored while any modal is open (selection is content-pane-only, the same
  rule mouse selection follows).
- **While `visual` is active:** a new `handleVisualKey(msg)` runs *first* in
  `handleKey` — before the global Back/Forward switch — the same precedence
  trick the tree modal uses to shadow history keys, so `h`/`l` don't trigger
  Back/Forward and `j`/`k` don't fall through to the viewport. Inside it:

| Key                  | Matched via                | Action                              |
| -------------------- | -------------------------- | ----------------------------------- |
| `h j k l` + `←↑↓→`   | raw `msg.String()`         | move caret by char / line           |
| `g` / `G`            | `Top` / `Bottom`           | caret to doc top / bottom           |
| `^d` / `^u`          | `HalfPageDown/Up`          | caret ± half-page                   |
| `Space`              | `BeginSelect`              | drop anchor → enter extend phase    |
| `y` / `^y`           | `CopyPath`                 | **yank** selection, then exit       |
| `Esc`                | `ClearLink`                | cancel, exit (from either phase)    |

Char/line motions are matched on the **raw key** (`h`/`j`/`k`/`l` plus the four
arrows) rather than the `Back`/`Forward`/`Up`/`Down` keyMap fields, because the
`modern` dialect binds `Back`/`Forward` to `alt+←`/`alt+→` — so reusing them
would leave modern users unable to move the caret horizontally with plain
arrows. Visual mode is a self-contained modal sub-language, so explicit motion
keys (vim letters + arrows, working in both dialects) are clearer and correct.
Jumps (`g`/`G`, `^d`/`^u`) *do* reuse the dialect-aware `Top`/`Bottom`/
`HalfPageDown`/`HalfPageUp` fields. The only genuinely new bindings are `v` and
`Space`. Yank reuses the dialect's copy key: in visual mode it copies the
*selection*; outside visual mode it still copies the file path. Keys not
matched (e.g. `n`/`p`/`Enter`) are inert while visual mode is active.

## Caret movement + scrolling

- A `moveCaret(dline, dcol)` helper updates `selection.cursor`, clamped to valid
  cells with the same bounds logic as `screenToContent`: line to
  `[0, len(lines)-1]`, col to `[0, lineWidths[line]]`. **In the positioning
  phase (`!selecting`) it also sets `anchor = cursor`** so the caret moves as a
  single cell with no span; in the extend phase it moves only `cursor`.
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
- **One tweak** for caret visibility: when the span is zero-width (`hi <= lo`),
  render a single reverse-video caret cell at `cursor`, using the character
  under the caret or a synthesized space at end-of-line. Today that branch
  renders nothing — which would make the caret invisible throughout the entire
  positioning phase (where `anchor == cursor` by construction) and at the
  moment the anchor is first dropped.
- The caret is simply the active (moving) end of the highlighted span — we do
  not draw a separate caret glyph. Keeps rendering to one code path; a distinct
  caret style can be added later if desired.

## Yank / cancel lifecycle

- **Begin select** (`Space`, positioning phase only): set `selecting = true`,
  leaving `anchor` frozen at the current caret. No-op if already `selecting`.
- **Yank** (`y` / `^y`): `extractSelection()` → `copyToClipboard(text)` → toast
  `Copied N chars` via `m.diag.Info` → `finalizeSelection()` (sets
  `copied = true`, clears `visual`/`selecting`/`anchored`). The reverse-video
  highlight **persists** after yank until the next keystroke / press /
  navigation — matching mouse-drag behavior. A yank during the positioning
  phase (zero-width span) copies nothing and just exits.
- **Cancel** (`Esc`): `clearSelection()` and exit, from either phase.
  `clearSelection`'s repaint condition gains `|| visual` so cancelling a
  never-moved caret still restores the clean render.
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

- Enter visual (`v`) → caret at top-left; highlight shows one cell;
  `visual && !selecting`.
- Positioning: `j`/`l` move the caret with **no span** (`anchor == cursor`,
  `extractSelection` empty); caret clamps at edges.
- Begin select (`Space`): freezes the anchor at the caret; subsequent `l`/`j`
  grow the span; `extractSelection` returns the expected substring.
- Jumps: position with `j`, `Space`, then `G` and yank copies to end; `^d`
  moves half-page and scrolls.
- Yank: clipboard receives the text, footer shows `Copied N chars`, highlight
  persists, `visual`/`selecting` cleared.
- `Esc` cancels and restores the clean render — tested from both the
  positioning and the extend phase.
- `v` is a no-op while a modal is open.
- Dialect coverage: `v` enters and `Space` anchors in both pager and modern;
  `^y` yanks in modern, `y` in pager.
- Overlap whitelist: `TestKeys_NoOverlappingActions` passes with `Space` bound
  to both `BeginSelect` and `ToggleFolder`.
- Regression: `y` outside visual still copies the path; `h`/`l` outside visual
  still navigate history; `Space` in the tree modal still toggles a folder.

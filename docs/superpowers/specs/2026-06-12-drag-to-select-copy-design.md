# Drag-to-select with auto-copy — design

Status: shipped.

## Goal

Let the user select text in the rendered content pane with a mouse
drag; on release, the selected text is copied to the clipboard
automatically — no extra keystroke. The selection stays highlighted as
confirmation, and the footer shows a transient "Copied N chars" toast.

This mirrors the "select → it's on your clipboard" behavior users expect
from GUI text and from terminal apps that own the mouse.

## The constraint that shapes everything

hypogeum launches with `tea.WithMouseCellMotion()`
(`cmd/hypogeum/main.go:55`). When a program enables mouse reporting, the
terminal stops doing its *own* click-drag selection — every mouse event
is delivered to the app. So the terminal's native selection + copy is
unavailable; the app must implement selection itself.

`MouseCellMotion` reports button **press**, button **release**, and
**motion while a button is held**. That press → motion* → release
sequence is exactly what drag-to-select needs.

## Decisions (from brainstorming)

- **Selection model:** app-drawn. hypogeum tracks the drag, highlights
  the span, and copies on release.
- **Granularity:** character/word level — an arbitrary span that can
  start and end mid-line, like GUI selection.
- **Scope:** content pane only. Modals (tree, picker, search, backlinks,
  logs) keep their current click behavior.
- **Feedback:** both — the highlight persists after copy *and* a footer
  toast ("Copied N chars") appears.

## Architecture

### Selection state

Add a `selection` value to `contentUIState` (`internal/tui/content.go`):

```go
type cellPos struct {
    line int // absolute index into the rendered output's lines
    col  int // visible column (0-based)
}

type selection struct {
    active  bool    // a drag has produced at least one motion event
    copied  bool    // released → highlight persists as confirmation
    moved   bool    // motion seen since press (click-vs-drag discriminator)
    anchor  cellPos // where the drag started
    cursor  cellPos // current / final drag point
}
```

`line` is the **absolute** rendered-line index (independent of
`viewport.YOffset`), so scrolling mid-drag does not corrupt the
selection. `anchor`/`cursor` are normalized at extraction time so the
earlier point comes first regardless of drag direction.

### Retaining the base render

`refreshContent` currently builds `out`, calls
`m.content.viewport.SetContent(out)`, and discards `out`. We store the
link-highlighted output as `content.rendered string`. The selection
highlight layers on top of that base and is recomputed cheaply on every
motion event — the same pattern as `applyLinkHighlight`, and Glamour is
never re-run for a selection change.

### Coordinate mapping

The content pane is wrapped in a rounded border (`paneStyle`,
`view.go:157`), so rendered text starts at screen cell `(x=1, y=1)`. A
mouse event maps to a document position as:

```
col  = msg.X - 1
line = m.content.viewport.YOffset + (msg.Y - 1)
```

Clamp `col`/`line` into range so drags that leave the pane or run past
end-of-line still resolve to valid positions.

## Mouse lifecycle

Extends the content-pane branch of `handleMouse`
(`internal/tui/input.go:53`). Today that branch handles only `Press`.

- **Press** inside the content pane: record `anchor = cursor =
  screen→content`, set `moved = false`, clear any prior selection
  (`copied`/`active` reset). If the press also lands on a link zone,
  remember the link index — a press-then-release with **no** motion
  still follows the link (click-vs-drag disambiguation, preserving
  today's click-to-follow behavior).
- **Motion** (button held): update `cursor`, set `moved = true` and
  `active = true`, re-apply the highlight overlay. The first motion
  event is what turns a click into a drag, so a link under the original
  press is **not** followed.
- **Release**: if `!moved` → it was a click; follow the remembered link
  or fall through to the viewport exactly as today. If `moved` →
  finalize: extract text, call `m.copyToClipboard(text)`, push the footer
  toast, set `copied = true` (`active` stays true so the highlight
  persists), clear `moved`.

Any subsequent **press**, **keystroke**, or **navigation** clears the
selection (`active`/`copied` → false). Navigation already rebuilds
content via `refreshContent`; it resets the selection there.

## Text extraction & highlight rendering

The rendered content is a string of lines carrying ANSI escapes (colors,
bold, link-highlight reverse-video). A selection is a rectangle in
*visible columns*, but bytes do not line up with columns. The tool is
already a direct dependency: `github.com/charmbracelet/x/ansi`.

- `ansi.Cut(line, leftCol, rightCol)` slices by **visible** columns
  while preserving the active style and skipping escape sequences.
- `ansi.Strip(s)` drops every escape, leaving plain text.
- `ansi.StringWidth(line)` gives a line's visible width (for the
  end-of-line bound and clamping).

### Column bounds per line

For the normalized range `[start..end]` (start ≤ end), splitting the
base render at `\n`, line `i` selects columns:

- `i == start.line == end.line` (single line): `[start.col, end.col]`
- `i == start.line` (multi-line, first): `[start.col, width(i)]`
- `start.line < i < end.line` (middle): `[0, width(i)]`
- `i == end.line` (multi-line, last): `[0, end.col]`

### Extraction (what gets copied)

```go
for i := start.line; i <= end.line; i++ {
    lo, hi := colBounds(i)
    parts = append(parts, ansi.Strip(ansi.Cut(rendered[i], lo, hi)))
}
text := strings.Join(parts, "\n")
```

This yields the plain text the user sees — no escapes, no border bytes.

### Highlight overlay (plain-overlay approach)

For each line in range, split by columns and replace the selected span
with a single uniform reverse-video block:

```go
before := ansi.Cut(line, 0, lo)
mid    := ansi.Strip(ansi.Cut(line, lo, hi)) // strip first…
after  := ansi.Cut(line, hi, width)
out    := before + reverseVideoStyle.Render(mid) + after // …then one style
```

Inside the selection the original colors are dropped and replaced with
one reverse-video block — exactly how a GUI selection looks (the
selection color overrides the text color). Crucially this **sidesteps
the inner-`\e[0m`-reset problem**: because `mid` is stripped before
styling, there are no embedded escapes that could cancel the reverse
video mid-span. (The alternative — OR-ing reverse-video over the still
styled bytes to preserve syntax colors — reintroduces the
reset-reemit complexity the existing `stripSentinels` link code had to
solve, for marginal benefit. The copied text is identical either way
because extraction always strips.)

The overlay is rebuilt and pushed via `SetContent` on every motion and
once on release; it composes over the link-highlighted base, so a
selection can sit on top of a highlighted link without conflict.

## Clipboard

> **Spec correction (2026-06-12):** the originally-named
> `tea.SetClipboard` does **not** exist in the pinned Bubble Tea
> v1.3.4. Backend chosen: **OSC 52 via `termenv.Copy`**.

`github.com/muesli/termenv` (already a *direct* dependency) exposes
`termenv.Copy(text string)` (and `Output.Copy`), which emits an **OSC
52** escape. OSC 52 copies *through the terminal*, so it works over SSH
and inside tmux with no `pbcopy`/`xclip` platform binary, and adds no
new dependency.

The write is injected behind a function seam so tests stay TTY-free,
mirroring the existing `openExternal externalOpener` pattern
(`internal/tui/external.go`):

```go
// clipboardWriter copies text to the system clipboard. Injected so
// tests can record calls instead of emitting a real OSC 52 escape.
type clipboardWriter func(text string)

// defaultClipboardWriter is termenv.Copy (OSC 52). Real terminals copy;
// tests substitute a recorder.
func defaultClipboardWriter(text string) { termenv.Copy(text) }
```

`Model` gains a `copyToClipboard clipboardWriter` field, defaulted in
`New` to `defaultClipboardWriter` and overridable in tests.

**Caveat (accepted):** `termenv.Copy` writes the escape to stdout
outside Bubble Tea's render mutex. In practice this is safe — OSC 52 is
an invisible sequence that does not move the cursor or alter cells, and
emitting it this way is a widely-used pattern. It requires terminal OSC
52 support (most modern terminals; a few need it enabled). `termenv.Copy`
returns no error, so there is nothing to surface on failure; the
persistent highlight + toast are the user-visible confirmation that a
copy was attempted.

## Footer toast

`m.diag.Info("Copied N chars")` feeds the existing footer transient
(`internal/tui/diagnostics.go:89`), rendered by `renderFooter` with a
3s expiry. No new footer plumbing — the toast reuses the diagnostics
transient channel already wired into the footer.

## Error handling / edge cases

- **Empty selection** (drag that resolves to zero characters, e.g. a
  press-drag-release within one cell): treat as a click — do not copy,
  do not toast.
- **Drag past end-of-line / out of pane:** clamp `col`/`line` into
  range; a selection can validly run to the visual end of each line.
- **Scroll during drag:** absolute-line anchors keep the selection
  anchored to the document, not the screen.
- **Content shorter than before:** selection is cleared on any
  navigation or content refresh, so stale line indices never apply.
- **Code files / directory listings:** the content pane holds rendered
  ANSI lines regardless of source type, so selection works uniformly;
  extraction is purely column-based and source-agnostic.

## Testing (model-level, no TTY — per CLAUDE.md)

Exercise via synthesized `tea.MouseMsg` sequences against the model,
following `internal/tui/model_test.go` conventions:

- Forward drag (anchor before cursor) copies the expected text.
- Backward / upward drag copies the same text (normalization).
- Multi-line drag copies first-partial + middle-full + last-partial,
  newline-joined.
- Press + Release with **no** motion still follows a link under the
  press (click-vs-drag preserved).
- Empty/zero-width drag does not copy or toast.
- Highlight overlay appears after motion and clears on the next
  press / keystroke / navigation.
- Footer shows "Copied N chars" after a real selection.
- The injected `copyToClipboard` recorder receives the expected payload
  (keeps the test TTY-free; no real OSC 52 emission).

## Out of scope

- Selection inside modals.
- Word/line double/triple-click selection granularity.
- Selecting the *source markdown* rather than the rendered visible text
  (character-level mapping back to source for an arbitrary cell span is
  intractable; we copy what is shown).
- A keyboard "visual mode" selection.

## Touched packages

- `internal/tui/content.go` — selection state, `content.rendered`,
  extraction + overlay helpers.
- `internal/tui/input.go` — mouse lifecycle (press/motion/release) in
  the content-pane branch of `handleMouse`.
- `internal/tui/model.go` — `copyToClipboard clipboardWriter` field on
  `Model`, defaulted in `New`.
- `internal/tui/external.go` or a new `internal/tui/clipboard.go` — the
  `clipboardWriter` type + `defaultClipboardWriter` (termenv.Copy).
- `internal/tui/view.go` — none expected (overlay goes through
  `SetContent`; footer reuses the diagnostics transient).
- Tests alongside in `internal/tui/`.

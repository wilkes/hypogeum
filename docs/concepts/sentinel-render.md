# sentinel-render

The trick that lets `internal/markdown` recover link byte positions from Glamour's ANSI output, even though Glamour itself emits no positional metadata.

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [`internal/markdown`](../packages/markdown.md) and [link-following](../link-following.md); press `b` for the full backlinks list.

## Why it exists

Glamour produces ANSI-styled text with theme-dependent SGR codes and no offset information. To make links followable from the right pane, the TUI needs to know where each link's visible text *ended up* in the rendered string — both the byte range (so future phases can splice highlight SGR around it) and the row (so the viewport can scroll to it). The same mechanism also powers the hidden-URL house style.

Three approaches were considered for position recovery: coordinate mapping during render (would require forking Glamour), AST traversal (loses word-wrap geometry), and instrumenting Glamour's style with sentinel byte sequences. The third is what shipped, and it now does double duty for URL suppression.

## How it works

Two sentinel pairs are grafted onto Glamour's link primitives:

- **`\x1c` (FS) / `\x1e` (RS)** wrap `link_text` (the visible text). The post-render scan records each pair as a `(byteStart, byteEnd, row)` span; the marker hook splices BubbleZone Mark/Close pairs in their place.
- **`\x1d` (GS) / `\x1f` (US)** wrap `link` (the URL portion Glamour writes after every link). The scan discards the bytes between the pair, plus the single space Glamour hardcodes immediately before. Result: rendered prose reads as `[text]` instead of `[text] /path/to/target.md`.

Both pairs survive Glamour's word-wrap pass because Glamour treats single ASCII control bytes as opaque content. The cleaned output is byte-equivalent to a plain `Render(src)` after stripping ANSI escapes — there's a regression test (`TestRenderWithLinks_OutputIsCleanRender` in `internal/markdown/render_test.go`) that catches drift.

The instrumented style is a JSON deep clone of whichever environment default Glamour's `WithAutoStyle` would resolve to (NoTTY / dark / light), with sentinels grafted onto the `LinkText` primitive. **Do not** pass a partial config to `WithStyles` — it's replace-only, not merge, and silently drops everything else (headings, code blocks, margins). The first instrumented render came out unstyled because of this; the deep-clone approach restored visual parity.

Order is preserved by AST cross-reference: the Nth sentinel pair corresponds to the Nth `ASTLink` from `markdown.ExtractLinks`. If the two diverge (e.g. Glamour stops rendering some link form), `RenderWithLinks` falls back to a `Link` with empty `Resolved` rather than failing.

## Invariants / gotchas

- **Sentinels are single-byte ASCII control characters.** Multi-byte sentinels (`\x00LS\x01` etc.) leaked an extra byte into spans during the word-wrap pass. Single bytes survive cleanly.
- **The instrumented `Renderer` is per-width.** Glamour bakes wrap width into the renderer; `WindowSizeMsg` in the TUI rebuilds both the plain and instrumented renderers. Don't cache one across width changes.
- **`\x11` and `\x12` are reserved for snippet highlight.** The vault's snippet extraction wraps the matched display text with these bytes so the formatter can colorize them. Don't reuse any of `\x1c`/`\x1d`/`\x1e`/`\x1f` for other features, and don't run snippets or rendered output through any pipeline that strips ASCII control characters.
- **URL-suppression also runs in the plain `Render` path.** The plain renderer can't bookkeep link spans, but it does honor the hidden-URL house style via a smaller `stripURLSentinels` pass. Both renderers go through `hypogeumStyle` so prose-level styling stays consistent.
- **OSC 8 hyperlinks were investigated and rejected.** Wrapping link text in `\x1b]8;;URL\x1b\\` would give clickable hyperlinks in supporting terminals, but BubbleZone's coordinate-recording scan (`muesli/ansi.PrintableRuneWidth`) terminates an escape on any ASCII letter — meaning the URL bytes after the first letter get counted as visible width, and click hit-testing breaks. External-URL clicks will need a different mechanism (Phase 3 of [link-following](../link-following.md), via `xdg-open`/`open` on Enter).
- **The clean-strip is byte-equivalent, not visually identical.** That's checked by a golden test, not a visual assertion. If you change the instrumented style, that test catches drift.

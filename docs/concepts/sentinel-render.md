# sentinel-render

The trick that lets `internal/markdown` recover link byte positions from Glamour's ANSI output, even though Glamour itself emits no positional metadata.

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [`internal/markdown`](../packages/markdown.md) and [link-following](../link-following.md); press `b` for the full backlinks list.

## Why it exists

Glamour produces ANSI-styled text with theme-dependent SGR codes, no OSC 8 hyperlinks, and no offset information. To make links followable from the right pane, the TUI needs to know where each link's visible text *ended up* in the rendered string — both the byte range (so future phases can splice highlight SGR around it) and the row (so the viewport can scroll to it).

Three approaches were considered: OSC 8 hyperlinks (terminal support is uneven and Glamour doesn't emit them), coordinate mapping during render (would require forking Glamour), and instrumenting Glamour's style with sentinel byte sequences. The third is what shipped.

## How it works

The instrumented renderer is a second `glamour.TermRenderer` whose `link_text` style has `block_prefix = "\x1c"` (ASCII FS — file separator) and `block_suffix = "\x1e"` (ASCII RS — record separator) grafted on. Glamour writes these literally around every link's visible text and they survive the word-wrap pass. After render, a single linear scan over the output records each `(byteStart, byteEnd, row)` pair and strips the sentinels. The cleaned output is byte-equivalent to a plain `Render(src)` on the same terminal — there's a regression test (`TestRenderWithLinks_OutputIsCleanRender` in `internal/markdown/render_test.go`) that catches drift.

The instrumented style is a JSON deep clone of whichever environment default Glamour's `WithAutoStyle` would resolve to (NoTTY / dark / light), with sentinels grafted onto the `LinkText` primitive. **Do not** pass a partial config to `WithStyles` — it's replace-only, not merge, and silently drops everything else (headings, code blocks, margins). The first instrumented render came out unstyled because of this; the deep-clone approach restored visual parity.

Order is preserved by AST cross-reference: the Nth sentinel pair corresponds to the Nth `ASTLink` from `markdown.ExtractLinks`. If the two diverge (e.g. Glamour stops rendering some link form), `RenderWithLinks` falls back to a `Link` with empty `Resolved` rather than failing.

## Invariants / gotchas

- **Sentinels are single-byte ASCII control characters.** Multi-byte sentinels (`\x00LS\x01` etc.) leaked an extra byte into spans during the word-wrap pass. Single bytes survive cleanly.
- **The instrumented `Renderer` is per-width.** Glamour bakes wrap width into the renderer; `WindowSizeMsg` in the TUI rebuilds both the plain and instrumented renderers. Don't cache one across width changes.
- **`\x11` and `\x12` are reserved for snippet highlight.** The vault's snippet extraction wraps the matched display text with these bytes so the formatter can colorize them. Don't reuse `\x1c`/`\x1e` for anything else, and don't run snippets through any pipeline that strips ASCII control characters.
- **The clean-strip is byte-equivalent, not visually identical.** That's checked by a golden test, not a visual assertion. If you change the instrumented style, that test catches drift.

# `internal/markdown`

Wraps Glamour for rendering, parses link targets, and produces a follow-aware render that the TUI uses to surface links to the user.

See also: [architecture overview](../architecture.md), [link-following plan](../link-following.md), [`internal/tui`](tui.md) (consumer).

## Purpose

Two related responsibilities:

1. Render markdown source to ANSI-styled terminal output, scaled to the current terminal width.
2. Tell the caller what links are in the source and where they ended up in the rendered output.

## Types

```go
type Renderer struct {
    g            *glamour.TermRenderer
    instrumented *glamour.TermRenderer
}

type Link struct {
    Text     string
    Href     string
    Resolved ResolvedLink
    Row      int
}

type ResolvedLink struct {
    Kind   LinkKind   // LinkLocalFile, LinkExternal, LinkAnchor, LinkInvalid
    Target string     // absolute path for local files; raw URL otherwise
    Anchor string     // fragment without the leading '#'
}

type ASTLink struct {
    Text string
    Href string
}
```

`Renderer` holds two underlying Glamour renderers — one plain, one with sentinel-injected styles for `RenderWithLinks`. Both must be rebuilt when the wrap width changes.

## Public surface

- `NewRenderer(width int, opts ...Option) (*Renderer, error)` — width below 20 falls back to 80. Options include `WithResolver(Resolver)` for wikilink resolution; the TUI passes its `*vault.Vault` here.
- `(*Renderer).RenderFile(path) (string, error)` — read + plain render. Used when the link list isn't needed.
- `(*Renderer).Render(src string) (string, error)` — plain render of an already-loaded string.
- `(*Renderer).RenderWithLinks(src, base string, marker LinkMarker) (string, []Link, error)` — instrumented render. The TUI uses this on every file open. `base` is the path of the file being rendered; it's needed to resolve relative link targets. `marker` is optional (may be `nil`); if non-nil, its open/close strings are spliced around each link's visible text — the TUI uses this to inject BubbleZone Mark/Close pairs for click hit-testing.
- `ResolveLink(base, href string) ResolvedLink` — pure path classification. Useful in tests.
- `ExtractLinks(src string) []ASTLink` — goldmark AST walk; returns inline links and autolinks in document order, skips images.

## Key invariants

- **The instrumented render is byte-equivalent to the plain render after sentinel-strip.** Verified by [`render_test.go`](../../internal/markdown/render_test.go) `TestRenderWithLinks_OutputIsCleanRender`. If you change the instrumented style config, that test catches drift.
- **Link order is the AST order.** `RenderWithLinks` cross-references `ExtractLinks` output with sentinel spans positionally — the Nth sentinel pair corresponds to the Nth AST link. If the two diverge (e.g. Glamour stops rendering some link form), the loop falls back to using only the visible text and Resolved is zero-valued.
- **`Renderer` is per-width.** Don't cache one across width changes; word-wrap silently breaks. The TUI rebuilds on `WindowSizeMsg`.
- **`ResolveLink` doesn't check existence.** Returns a `LinkLocalFile` even if the target file is missing. Callers (the TUI) deal with the missing-file case at navigation time.

## The sentinel trick

Two sentinel pairs are grafted onto Glamour's link primitives:

- `\x1c` / `\x1e` (FS / RS) bracket `link_text`. Post-render the strip pass records each pair as a `(row, text)` span and (in `RenderWithLinks`) splices BubbleZone Mark/Close pairs in their place.
- `\x1d` / `\x1f` (GS / US) bracket `link` (the URL Glamour writes after every link). The strip pass discards the bytes between, plus the leading space Glamour hardcodes — so rendered prose reads as `[text]` instead of `[text] /path/to/target.md`.

The cleaned output is byte-equivalent to a plain `Render` on the same terminal after stripping ANSI — verified by `TestRenderWithLinks_OutputIsCleanRender`. Full design and rationale (including the alternatives we rejected): [[sentinel-render]].

## Link styling

`LinkText` carries an underline (`Underline: &yes` on the Glamour style primitive). Glamour's dark theme puts the underline on `Link` (the URL portion), not `LinkText` — once we hide the URL the visible text loses that cue, so we move it onto `LinkText` explicitly.

OSC 8 hyperlinks were investigated and rejected: BubbleZone's `Scan` measures cell coordinates with `muesli/ansi.PrintableRuneWidth`, which terminates any escape on an ASCII letter. An OSC 8 sequence's URL contains letters, so the scanner exits escape mode mid-URL and counts the rest as visible width. Result: zone bounds shift far to the right of where the link actually rendered, and mouse-click hit-testing breaks. External-URL launching belongs in Phase 3 of [link-following](../link-following.md) via `xdg-open`/`open` on `Enter`.

## Why goldmark is a direct dependency

It comes in transitively via Glamour, but `ExtractLinks` uses it directly to walk the AST. Promoting it to a direct require makes the dependency graph honest and prevents a Glamour version bump from silently dropping it.

## Wikilink preprocessing

When a `Resolver` is set (`WithResolver`), `RenderWithLinks` runs a regex pass over the source before handing it to Glamour, rewriting every `[[Name#Heading^Block|Alias]]` into either a standard markdown link (resolved) or styled placeholder text (unresolved with `?` suffix). The body parser is shared with `internal/vault` via the [`internal/wikilink`](../../internal/wikilink/wikilink.go) package — both consumers call `wikilink.Parse(body)` and operate on the resulting `*Body`.

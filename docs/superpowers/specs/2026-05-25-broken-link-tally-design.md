# Broken-link tally — design

**Status:** shipped on 2026-05-25 (PR #40).

**Goal:** surface a count of broken links in the currently rendered document as a faint suffix in the footer's location row. Closes one of the three remaining Wikilinks Phase 2 items from [wikilinks-and-backlinks-design](2026-05-07-wikilinks-and-backlinks-design.md). Block references and configurable vault root remain.

**Out of scope:** vault-wide tally, click-to-jump, listing the broken links (no modal), validation of `#heading` anchors, validation of `#L10-L20` range line numbers, broken-link detection inside source embeds.

## What counts as broken

Two categories, both per-document:

1. **Unresolved wikilinks.** `[[Name]]` that the vault resolver returns no match for. Today these render as plain text with a `?` suffix and are intentionally absent from `m.content.links` (per [[link-cursor]] — broken links can't be followed, so they don't belong in the cycler). Counting them does not change that — they stay out of the cycler.
2. **Inline links to non-existent local paths.** `[text](path)` where, after `markdown.ResolveLink` resolves the href relative to the current file, the path doesn't exist on disk. External schemes (`http`, `https`, `mailto`, `ftp`, `file`, anything matching the existing external-URL classifier) are skipped — we can't and shouldn't stat them.

A link is counted once. Range-link variants (`path#L10-L20`) count only the path; the line range itself is not validated.

## Computation

Done once per `refreshContent`, before the footer renders. The render path is already I/O-heavy (Glamour, file read, optional source embeds), so adding `os.Stat` calls for inline local links is in the noise.

- Unresolved wikilinks: counted inside `internal/markdown`. `preprocessWikilinks` already iterates and resolves every wikilink node — it gains a counter return alongside its existing outputs (or a small struct return). `RenderWithLinks` propagates that count out to its caller. No change to the rendered output: unresolved wikilinks still get the `?` placeholder.
- Inline local links: classified in `internal/tui/content.go` by walking `m.content.links` after `RenderWithLinks` returns. For each link whose kind is local (file or directory — i.e. not external, not wikilink-resolved-then-followed), the resolver-computed absolute path is `os.Stat`'d; missing → count++. Wikilinks that *did* resolve are necessarily real files (the vault index only holds extant paths) so they cost no stat.

The sum is stashed on `m.content.brokenCount int`. Reset to zero on every `refreshContent`.

## Render

In `internal/tui/view.go`'s `renderFooter`, after all existing `loc` mutations (transient overlay, link-cursor overlay), append ` ⚠ N broken` to `loc` when `m.content.brokenCount > 0` **and** no transient is active (a transient already replaces `loc` entirely; the broken count would be visually wrong next to a warning/error transient). When a link cursor *is* active, the suffix still appears — the cursor overlay decorates `loc` rather than replacing it, and the user benefits from knowing the document has broken targets while cycling.

Style: faint warning color (lipgloss `Color("11")` — same warning yellow as the transient style — but with `Faint(true)` so it doesn't compete with the location text). Format: literal ` ⚠ N broken`, single space leading, no trailing punctuation. Singular vs plural: always `broken`, since `N broken` reads fine for any N.

When `brokenCount == 0` the suffix is absent — no `✓ 0 broken` confirmation. The whole point is to flag attention; absence is the green path.

## Surfaces touched

| File | Change |
| --- | --- |
| `internal/markdown/links_render.go` (or wherever `preprocessWikilinks` lives) | Add an unresolved-wikilink counter return. |
| `internal/markdown/render.go` | `RenderWithLinks` propagates the count out. |
| `internal/tui/content.go` | `refreshContent` records `brokenCount` from the count + per-link `os.Stat` walk. |
| `internal/tui/model.go` (or wherever `contentState` is defined) | New field `brokenCount int` on the content state struct. |
| `internal/tui/view.go` | `renderFooter` appends the suffix. |

## Tests

- `internal/markdown`: a `RenderWithLinks` test asserts that a fixture with two unresolved `[[Foo]]`s and one resolved one returns count 2.
- `internal/tui` (model-level): build a `Model` over a tmpdir containing a markdown file with one missing inline link and two unresolved wikilinks (vault built without targets); navigate to the file; assert `m.content.brokenCount == 3` and that `m.View()` output contains the substring `⚠ 3 broken`.
- `internal/tui` (model-level, zero case): no broken links → footer output does **not** contain `broken`.
- `internal/tui` (transient suppression): when a transient diagnostic is active, the broken suffix is absent.

Tests are race-clean (no goroutines added). Pure model-level — no terminal required.

## CLAUDE.md / docs/index.md updates

- `docs/index.md`: under the wikilinks Phase 2 line, mention that the broken-link tally has shipped; remaining Phase 2 items are block references and configurable vault root.
- `CLAUDE.md`: update the "What's not built yet — Wikilinks and backlinks — Phase 2 in progress" paragraph to remove the broken-link tally from the remaining list.

## Why not bigger

A click-to-jump / modal listing is appealing but premature: vaults are small enough today that a number in the footer is enough nudge to grep for `[[` and a `?`. If the footer indicator turns out to under-serve (users want to jump straight to the offenders), a modal becomes a separately scoped follow-up — the footer count is its own minimal-viable feature.

# Link following

Plan and design notes for following links inside rendered markdown documents.

## Background

The TUI walks a directory tree and renders the selected file via Glamour. Links rendered inside the markdown are decorative only — there's no way to follow `[other note](other.md)` from inside the right-hand pane. The README calls this out as the next milestone.

`internal/markdown/render.go` already has `ResolveLink(base, href) ResolvedLink`, which classifies a target as local file, external URL, or same-document anchor. Wiring is the missing half.

## Approach: instrumented render + status-line cursor

Glamour produces ANSI output with no positional metadata, no OSC 8 hyperlinks, and theme-dependent SGR codes for link styling. Three approaches were investigated; the chosen one is **instrument the renderer**: pass a custom style with sentinel byte sequences in `link_text.block_prefix` / `block_suffix`. Glamour writes them literally around every link's visible text. A post-pass over the rendered string finds each sentinel pair and records `(byteStart, byteEnd)`. Order-preserving cross-reference with goldmark's AST attaches the original `href`.

Verified working on word-wrapped multi-line links (the wrap splits the span across rows but the sentinels still bracket it).

The "cursor" itself is a single integer index into the link list. Phase 1 surfaces it only in the footer (`[3/7] notes/first.md`) — no inline highlight. Phase 2 adds the inline highlight by re-splicing SGR codes around the active link's byte range. Phase 3 handles external URLs via `os/exec`.

Alternative considered: numbered link picker (modal). Cheaper but less browser-like; rejected once the sentinel trick was proven.

## Phase 1 scope

What's in:
- `markdown.RenderWithLinks(src) (rendered string, links []Link, err error)` — single entry point that returns both the rendered string and the link list. Replaces direct `Render` use in the TUI.
- `Link` carries `Href`, the resolved `ResolvedLink`, and the rune-row in the rendered string (for auto-scroll).
- TUI keys `n` / `p` cycle the selected link; `Enter` follows it; `Esc` clears the selection.
- Selection is content-pane scoped: only active when the right pane has focus.
- Footer shows `[k/n] <target>` for the selected link.
- Auto-scroll the viewport so the selected link's row is in view.
- Local file targets call `openFile` (history records the visit, like clicking a tree entry).
- Anchors are recorded but only scroll within the document; if the anchor doesn't resolve to a known heading, it's a no-op.
- External URLs are recognized but not opened — footer shows the URL with a hint that opening external links is unimplemented.

What's out (Phase 2/3):
- Inline highlight of the active link in the rendered text.
- Multi-segment cursor visualization for word-wrapped links.
- Actually launching external URLs with `open`/`xdg-open`.

## Implementation steps (commits)

Each commit is independently testable and leaves the tree green.

1. **Plumbing: package skeleton + plan doc.** Add `docs/link-following.md`, update CLAUDE.md with the docs convention. (This commit.)

2. **`markdown.ExtractLinks`: AST walk.** New function that takes raw markdown source and returns `[]ASTLink{Text, Href}` in document order using goldmark directly. Pure function, full unit test coverage. No TUI changes yet.

3. **`markdown.RenderWithLinks`: instrumented render.** Inject sentinel `block_prefix`/`block_suffix` into a `WithStyles` config that otherwise mirrors the dark default. Render with it, scan for sentinel pairs, strip sentinels from output, build `[]Link` cross-referenced with `ExtractLinks`. Returns the cleaned rendered string and the link list. Unit tested with fixtures: single link, multiple links, link inside list, word-wrapped link, external URL, anchor link, no links.

4. **TUI integration: render + footer indicator.** Wire `RenderWithLinks` into `refreshContent`. Store the link list on the model. Footer shows `[k/n] target` when a link is selected. No keybindings yet — this commit just verifies the plumbing doesn't break existing behavior. Existing TUI tests stay green.

5. **TUI keybindings: cycle, follow, clear.** Add `n` / `p` / `Enter` (when content focused, no current selection wins over picker logic) / `Esc`. Auto-scroll so the selected link's row is visible. Add tests that exercise: cycling order, selection wraps at end, Enter on local file calls `openFile`, Enter on external URL surfaces a "not yet" status, Esc clears selection.

6. **Wire-up review and CLAUDE.md update.** Update CLAUDE.md "What's not built yet" to reflect Phase 1 done and Phase 2/3 outstanding. Final test pass + vet.

## Open questions

- Sentinel choice: `\x00LS\x01` and `\x00LE\x01` worked in the probe but Glamour leaked an extra `\x01` into spans. Use multi-byte ASCII sentinels (e.g. `\x1c\x1d` paired with `\x1e\x1f` — file/group/record/unit separator characters) that are unlikely to appear in user content and don't get treated as control chars by Glamour's word-wrap pass. Decide during commit 3 by testing both.
- Anchor resolution: defer real anchor support to Phase 2. Phase 1 records the anchor target but does nothing with it — clicking an anchor link is a no-op with a status message.
- What to do when a local link points outside the tree root: follow it (matches a real browser — you can `cd` anywhere) but don't expand the tree. The right pane updates; the tree cursor stays where it was.

## Verification

After each commit: `go test ./... && go vet ./...`. Manual smoke: build, run against `/tmp/hypogeum-fixtures` (created during initial setup), verify links cycle and follow as expected.

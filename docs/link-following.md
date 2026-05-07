# Link following

**Status:** Phase 1 shipped. Phase 2 and 3 not started.

Plan and design notes for following links inside rendered markdown documents.

See also: [docs index](index.md), [architecture overview](architecture.md), [`internal/markdown`](packages/markdown.md), [`internal/tui`](packages/tui.md).

## Background

The TUI walks a directory tree and renders the selected file via Glamour. Links rendered inside the markdown are decorative only — there's no way to follow `[other note](other.md)` from inside the right-hand pane. The README calls this out as the next milestone.

`internal/markdown/render.go` already has `ResolveLink(base, href) ResolvedLink`, which classifies a target as local file, external URL, or same-document anchor. Wiring is the missing half.

## Approach: instrumented render + status-line cursor

Two pieces, each with its own concept doc:

- **Recovering link positions** from Glamour's ANSI output: the renderer is instrumented with sentinel byte sequences that survive word-wrap. Full design and rationale: [[sentinel-render]].
- **Selecting and following a link** with `n`/`p`/`Enter`/`Esc`: a single integer cursor into the link list, footer-only in Phase 1. Full design and rationale: [[link-cursor]].

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

All Phase 1 commits landed. Each was independently testable and left the tree green.

1. ✅ **Plumbing: package skeleton + plan doc.** Added `docs/link-following.md`, updated CLAUDE.md with the docs convention.
2. ✅ **`markdown.ExtractLinks`: AST walk.** Walks goldmark AST, returns inline links and autolinks in document order; skips images.
3. ✅ **`markdown.RenderWithLinks`: instrumented render.** Sentinel-injected style is a JSON deep clone of the environment-resolved Glamour default (NoTTY/dark/light), so cleaned output is byte-equivalent to plain `Render` on the same terminal.
4. ✅ **TUI integration: render + footer indicator.** `refreshContent` now reads source, renders with links, stores the list on the model. Footer marker only shows when a link is selected.
5. ✅ **TUI keybindings: cycle, follow, clear.** `n`/`p`/`Enter`/`Esc`, content-pane scoped, with viewport auto-scroll.
6. ✅ **Wire-up review and CLAUDE.md update.** This commit.

## What changed from the original plan

- Picked a JSON deep-clone of `styles.DarkStyleConfig` / `styles.LightStyleConfig` (matched to the environment) as the base for the instrumented style, rather than building a partial `WithStyles` config. `WithStyles` is replace-only — passing a partial config silently drops headings, margins, code blocks, etc. The first instrumented render came out unstyled; this approach restored visual parity.
- Single-byte sentinels (`\x1c`, `\x1e`) instead of the multi-byte `\x00LS\x01` / `\x00LE\x01` initially considered. Single bytes don't get split or doubled by Glamour's word-wrap pass; the multi-byte form leaked an extra `\x01` byte into spans.
- Two prerequisite commits landed before commit 1 (gitignore fix, entrypoint+CLAUDE.md). The original plan assumed those were already in place; they weren't.

## Open questions

- Sentinel choice: `\x00LS\x01` and `\x00LE\x01` worked in the probe but Glamour leaked an extra `\x01` into spans. Use multi-byte ASCII sentinels (e.g. `\x1c\x1d` paired with `\x1e\x1f` — file/group/record/unit separator characters) that are unlikely to appear in user content and don't get treated as control chars by Glamour's word-wrap pass. Decide during commit 3 by testing both.
- Anchor resolution: defer real anchor support to Phase 2. Phase 1 records the anchor target but does nothing with it — clicking an anchor link is a no-op with a status message.
- What to do when a local link points outside the tree root: follow it (matches a real browser — you can `cd` anywhere) but don't expand the tree. The right pane updates; the tree cursor stays where it was.

## Verification

After each commit: `go test ./... && go vet ./...`. Manual smoke: build, run against `/tmp/hypogeum-fixtures` (created during initial setup), verify links cycle and follow as expected.

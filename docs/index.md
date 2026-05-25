# Documentation

Index of everything in `docs/`. Browse this folder by running `hypogeum docs/` from the repo root — it dogfoods the tool against itself.

## Architecture

- [Architecture overview](architecture.md) — package layering, data flow on a keystroke, where to start reading
  - [`internal/tree`](packages/tree.md) — filesystem walker that produces the left-pane tree
  - [`internal/markdown`](packages/markdown.md) — Glamour wrapper, link resolution, sentinel-instrumented render
  - [`internal/nav`](packages/nav.md) — browser-style back/forward stack, no I/O
  - [`internal/tui`](packages/tui.md) — Bubble Tea Model that wires everything together
  - [`internal/watch`](packages/watch.md) — fsnotify-backed live-update watcher, debounced and tree-aware
  - `internal/vault` — wikilink/backlink index over the markdown set; see [[vault-index]]
  - `internal/wikilink` — shared `[[Name#Heading^Block|Alias]]` body parser used by `vault` and `markdown`

## Concepts

Cross-cutting ideas that show up in multiple specs and packages. Each is its own short file so the backlinks modal (`b`) shows everywhere it's referenced. Wikilink syntax is used here (and only here) to dogfood the feature; the rest of this index uses standard markdown links so GitHub renders them.

- [[sentinel-render]] — how link positions are recovered from Glamour's ANSI output
- [[vault-index]] — forward + reverse reference index, basename resolution, proximity tiebreak
- [[diagnostics]] — the warn/error stream, footer transient, log file, `^l` modal
- [[modal-geometry]] — single-modal invariant and layout recompute on `B`/`^l`
- [[return-cursor]] — path-keyed cursor restoration on Back navigation
- [[link-cursor]] — content-pane link selection (`n`/`p`/`Enter`/`Esc`) state model

## Active feature work

- [Link following](link-following.md) — Phases 1 and 2 shipped (cursor selection + inline reverse-video highlight). Phase 3 (external URL launch) outlined.
- [Wikilinks and backlinks](superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md) — Phase 1 shipped: wikilinks resolve via vault index, backlinks modal (`b`), log viewer (`^l`). Phase 2 partially shipped (auto-scroll, inline-link pre-select, broken-link tally, block references); configurable vault root remains. The persistent backlinks pane introduced in Phase 1 was later removed in favor of the modal-only surface.
- [Block references and heading anchors](superpowers/specs/2026-05-25-block-references-design.md) — shipped — `[[Note#Heading]]`, `[[Note#^block]]`, and `[text](note.md#heading)` now scroll the destination. Broken anchors render with the `?` suffix and count in the footer tally.
- [Broken-link tally](superpowers/specs/2026-05-25-broken-link-tally-design.md) — shipped — footer shows ` ⚠ N broken` when the current document has unresolved wikilinks or inline links to missing local paths.
- [Backlinks navigation](superpowers/specs/2026-05-07-backlinks-navigation-design.md) — shipped — cursor, `Enter`-to-follow, scroll-to-reference, and back-restores-cursor on top of the backlinks display.
- [Pre-select inline link](superpowers/specs/2026-05-09-pre-select-inline-link-design.md) — shipped — when arriving via backlink-follow / Back / Forward, pre-select the inline link that points back to where you came from, so `n`/`p` resumes from a meaningful position.
- [Narrow-terminal layout](superpowers/specs/2026-05-07-narrow-terminal-layout-design.md) — superseded — auto-hid the tree pane below an 80-col terminal width. The two-pane layout itself was later replaced by a tree-modal (`^b`), which clamps to a percentage of terminal size rather than gating on width; the narrow-terminal threshold no longer exists.
- [Vault-rooted picker](superpowers/specs/2026-05-07-vault-rooted-picker-design.md) — shipped — `^p` modal over the pruned `*tree.Node`, so only directories containing markdown appear.
- [Unified finder with recency](superpowers/specs/2026-05-12-unified-finder-recency-design.md) — shipped — `^p` opens a flat list of every vault markdown file, ranked by a hybrid mtime + persisted-visit score.
- [finder-fuzzy-filter](superpowers/specs/2026-05-12-finder-fuzzy-filter-design.md) — type-to-filter on top of the recency-ranked picker. Shipped 2026-05-12.
- [Code-file syntax highlighting](superpowers/specs/2026-05-12-code-file-rendering-design.md) — design approved 2026-05-12 — Chroma-driven highlighting + line-number gutter for non-md files opened by CLI arg or inline link. Tree, picker, and vault stay markdown-only.
- [Source embeds and line-range links](superpowers/specs/2026-05-13-source-embeds-design.md) — `![[file.go#L10-L20]]` transclusion + `[t](file.go#L10-L20)` navigation. [Plan](superpowers/plans/2026-05-13-source-embeds.md).
- [Source embeds — follow-up fixes](superpowers/specs/2026-05-13-source-embeds-followups-design.md) — four narrow fixes surfaced by post-merge review of #30: fence-aware embed regex, embed-link no-scroll on `Row=-1`, Esc preserves scroll on range-highlight clear, `followBacklink` captures pre-select range. [Plan](superpowers/plans/2026-05-13-source-embeds-followups.md).
- [Directory listing](superpowers/specs/2026-05-13-directory-listing-design.md) — shipped — directory link targets now render an in-pane listing (header + absolute path + bullet list of every non-hidden entry, dirs first with trailing `/`). Previously surfaced `Error: read /path: is a directory`. [Plan](superpowers/plans/2026-05-13-directory-listing.md).
- [Multi-segment cursor for wrapped links](superpowers/specs/2026-05-13-multi-segment-cursor-design.md) — shipped — `stripSentinels` closes/reopens the reverse-video marker on every wrapped row so the link cursor stays visible across the whole link. [Plan](superpowers/plans/2026-05-13-multi-segment-cursor.md).
- [Full-text search](superpowers/specs/2026-05-14-full-text-search-design.md) — shipped — `^s` opens a modal that scans every vault markdown file for the query, renders hits as `path:line` + highlighted snippet, recency-ranks the result list, and on `Enter` opens the file scrolled to the matched line. [Plan](superpowers/plans/2026-05-14-full-text-search.md).

## Conventions for adding to this folder

One file per topic. Kebab-case filenames, no date prefix. Update plans in place — strike-through, "Status:" lines, or check-marked steps beat parallel files. Index entries here are one line plus a short hook; the detail lives in the linked file.

Doc files cross-link with relative paths so they survive being moved between checkouts and so they navigate with the same key bindings users have for any other markdown.

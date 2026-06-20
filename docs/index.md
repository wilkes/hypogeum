# Documentation

Index of everything in `docs/`. Browse this folder by running `hypogeum docs/` from the repo root — it dogfoods the tool against itself.

## Architecture

- [Architecture overview](architecture.md) — package layering, data flow on a keystroke, where to start reading
  - [`internal/tree`](packages/tree.md) — filesystem walker that produces the directory tree shown in the `t` modal
  - [`internal/markdown`](packages/markdown.md) — Glamour wrapper, link resolution, sentinel-instrumented render
  - [`internal/nav`](packages/nav.md) — browser-style back/forward stack, no I/O
  - [`internal/tui`](packages/tui.md) — Bubble Tea Model that wires everything together
  - [`internal/watch`](packages/watch.md) — fsnotify-backed live-update watcher, debounced and tree-aware
  - [`internal/recent`](packages/recent.md) — recency ranking: `RankByMTime` (edit recency, finder + `/` search) and `RankByVisit` (visit recency, `r` modal + `recent` verb)
  - `internal/search` — pure case-insensitive full-text substring scanner with worker fan-out; backs the `/` search modal and the `search` query verb
  - `internal/query` — non-interactive JSON query mode (`search`/`links`/`recent`/`neighbors`), no TUI deps
  - `internal/vault` — wikilink/backlink index over the markdown set; see [[vault-index]]
  - `internal/wikilink` — shared `[[Name#Heading^Block|Alias]]` body parser used by `vault` and `markdown`
- [Architecture review (DDD lens)](architecture-review.md) — historical record: findings 1–4 (path-resolution triplication, `Model` god-object, duplicated highlight markers, splitting `links_render.go`) have all shipped; only the deferred `Backlinks` reverse-index (Finding #5) remains open

## Concepts

Cross-cutting ideas that show up in multiple specs and packages. Each is its own short file so the backlinks modal (`b`) shows everywhere it's referenced. Wikilink syntax is used here (and only here) to dogfood the feature; the rest of this index uses standard markdown links so GitHub renders them.

- [[sentinel-render]] — how link positions are recovered from Glamour's ANSI output
- [[vault-index]] — forward + reverse reference index, basename resolution, proximity tiebreak
- [[diagnostics]] — the warn/error stream, footer transient, log file, `^l` modal
- [[modal-geometry]] — single-modal invariant and layout recompute on `B`/`^l`
- [[return-cursor]] — path-keyed cursor restoration on Back navigation
- [[link-cursor]] — content-pane link selection (`n`/`p`/`Enter`/`Esc`) state model

## Diary

- [Development diary](diary/index.md) — chronological log of how the project was built, reconstructed from commits and merged PRs (2026-05-05 → 2026-06-13).
- [The Big Render](diary/noir.md) — the same history retold in Raymond Chandler's hardboiled voice, for fun.

## Active feature work

- [Link following](link-following.md) — Phases 1 and 2 shipped (cursor selection + inline reverse-video highlight). Phase 3 (external URL launch) outlined.
- [Wikilinks and backlinks](superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md) — Phase 1 shipped: wikilinks resolve via vault index, backlinks modal (`b`), log viewer (`^l`). Phase 2 partially shipped (auto-scroll, inline-link pre-select, broken-link tally); block references and configurable vault root remain. The persistent backlinks pane introduced in Phase 1 was later removed in favor of the modal-only surface.
- [Broken-link tally](superpowers/specs/2026-05-25-broken-link-tally-design.md) — shipped — footer shows ` ⚠ N broken` when the current document has unresolved wikilinks or inline links to missing local paths.
- [Backlinks navigation](superpowers/specs/2026-05-07-backlinks-navigation-design.md) — shipped — cursor, `Enter`-to-follow, scroll-to-reference, and back-restores-cursor on top of the backlinks display.
- [Pre-select inline link](superpowers/specs/2026-05-09-pre-select-inline-link-design.md) — shipped — when arriving via backlink-follow / Back / Forward, pre-select the inline link that points back to where you came from, so `n`/`p` resumes from a meaningful position.
- [Narrow-terminal layout](superpowers/specs/2026-05-07-narrow-terminal-layout-design.md) — superseded — auto-hid the tree pane below an 80-col terminal width. The two-pane layout itself was later replaced by a tree-modal (`t`), which clamps to a percentage of terminal size rather than gating on width; the narrow-terminal threshold no longer exists.
- [Vault-rooted picker](superpowers/specs/2026-05-07-vault-rooted-picker-design.md) — shipped — `^p` modal over the pruned `*tree.Node`, so only directories containing markdown appear.
- [Unified finder with recency](superpowers/specs/2026-05-12-unified-finder-recency-design.md) — shipped — `^p` opens a flat list of every vault markdown file, ranked by edit recency (mtime). (The original blended mtime + persisted-visit score was later removed by [Split recency signals](superpowers/specs/2026-06-20-split-recency-signals-design.md); the finder is now pure `recent.RankByMTime`.)
- [finder-fuzzy-filter](superpowers/specs/2026-05-12-finder-fuzzy-filter-design.md) — type-to-filter on top of the recency-ranked picker. Shipped 2026-05-12.
- [Finder mtime weighting](superpowers/specs/2026-05-31-finder-mtime-weighting-design.md) — demoted `visitWeight` from 1.5 to 0.5 so recently modified files outrank recently visited ones at equal age. Superseded — the whole blend (and `visitWeight`) was removed by [Split recency signals](superpowers/specs/2026-06-20-split-recency-signals-design.md); ranking is no longer a weighted combination. [Plan](superpowers/plans/2026-05-31-finder-mtime-weighting.md).
- [Split recency signals](superpowers/specs/2026-06-20-split-recency-signals-design.md) — shipped — pulled the blended mtime+visit score apart: finder (`^p`/`o`) and `/` search re-rank are now pure edit-recency (`recent.RankByMTime`), and a new `r` "recently opened" modal + the CLI `recent` verb are pure visit-recency (`recent.RankByVisit` — visited-only, last-visited first). Removed the decay blend, weights, and `Ranked.Score`.
- [Default index/readme landing file](superpowers/specs/2026-06-20-default-index-readme-design.md) — shipped — when opening a directory with no explicit file, prefer a top-level `index.*` then `readme.*` (case-insensitive stem) before falling back to the first file (`firstTopLevelFile`, `internal/tui/tree.go`). Top-level/markdown-only.
- [Code-file syntax highlighting](superpowers/specs/2026-05-12-code-file-rendering-design.md) — design approved 2026-05-12 — Chroma-driven highlighting + line-number gutter for non-md files opened by CLI arg or inline link. Tree, picker, and vault stay markdown-only.
- [Source embeds and line-range links](superpowers/specs/2026-05-13-source-embeds-design.md) — `![[file.go#L10-L20]]` transclusion + `[t](file.go#L10-L20)` navigation. [Plan](superpowers/plans/2026-05-13-source-embeds.md).
- [Source embeds — follow-up fixes](superpowers/specs/2026-05-13-source-embeds-followups-design.md) — four narrow fixes surfaced by post-merge review of #30: fence-aware embed regex, embed-link no-scroll on `Row=-1`, Esc preserves scroll on range-highlight clear, `followBacklink` captures pre-select range. [Plan](superpowers/plans/2026-05-13-source-embeds-followups.md).
- [Directory listing](superpowers/specs/2026-05-13-directory-listing-design.md) — shipped — directory link targets now render an in-pane listing (header + absolute path + bullet list of every non-hidden entry, dirs first with trailing `/`). Previously surfaced `Error: read /path: is a directory`. [Plan](superpowers/plans/2026-05-13-directory-listing.md).
- [Multi-segment cursor for wrapped links](superpowers/specs/2026-05-13-multi-segment-cursor-design.md) — shipped — `stripSentinels` closes/reopens the reverse-video marker on every wrapped row so the link cursor stays visible across the whole link. [Plan](superpowers/plans/2026-05-13-multi-segment-cursor.md).
- [Full-text search](superpowers/specs/2026-05-14-full-text-search-design.md) — shipped — `/` opens a modal that scans every vault markdown file for the query, renders hits as `path:line` + highlighted snippet, recency-ranks the result list, and on `Enter` opens the file scrolled to the matched line. [Plan](superpowers/plans/2026-05-14-full-text-search.md).
- [Keybinding dialects](superpowers/specs/2026-05-31-keybinding-dialects-design.md) — superseded — two coherent presets (pager/modern) removed in favor of a single default keymap; `internal/config` package deleted.
- [Remove keybinding dialects](superpowers/specs/2026-06-13-remove-keybinding-dialects-design.md) — shipped — collapsed `pager`/`modern` into one `defaultKeys()` keymap and deleted the `internal/config` package and `Options`/`StartupWarnings`.
- [Glamour table wrap](superpowers/plans/2026-06-01-glamour-table-wrap.md) — upgrade Glamour 0.8.0 → 0.10.0 so long table cells wrap to multiple lines instead of being character-truncated mid-word. Pins `│`/`─` separators and disables Glamour's new in-table-links footer to preserve the existing URL-hiding + alignment invariants.
- [Drag-to-select with auto-copy](superpowers/specs/2026-06-12-drag-to-select-copy-design.md) — shipped — app-drawn character-level mouse selection in the content pane; copies to the clipboard on mouse-release (OS clipboard via `atotto` + OSC 52 via `termenv.Copy` — covers local terminals incl. Terminal.app *and* SSH), persists the highlight, and shows a "Copied N chars" footer toast. Uses `charmbracelet/x/ansi` `Cut`/`Strip` for column-accurate extraction over ANSI-styled lines.
- [Copy current file path](superpowers/specs/2026-06-12-copy-current-path-design.md) — `y` copies the absolute path of the current view (`m.history.Current()`) via the existing `copyToClipboard`, toasting `Copied path: <path>`. No-op when nothing is open.
- [Keyboard selection (vim visual mode)](superpowers/specs/2026-06-13-keyboard-selection-design.md) — shipped — two-phase vim-style visual mode reusing the mouse selection's `selection{anchor,cursor}` span machinery: `v` shows a movable caret, `Space` drops the anchor to start extending, `h/j/k/l`/arrows + `g`/`G` + `^d`/`^u` move it, `y` yanks, `Esc` cancels.
- [Scriptable query mode](superpowers/specs/2026-06-20-scriptable-query-mode-design.md) — shipped — non-interactive JSON-to-stdout verbs (`search`, `links`, `recent`, `neighbors`) for agent/script consumption; git-style dispatch in `cmd/hypogeum`, new pure `internal/query` package, plus a `Vault.Outbound` accessor. Pointers (path/line/snippet), not content. [Plan](superpowers/plans/2026-06-20-scriptable-query-mode.md).
- [Benchmarking foundation](superpowers/specs/2026-06-20-benchmarking-foundation-design.md) — approved — measure-only sweep of the five hot paths (`tree.Walk`, `vault.Build`, `markdown.Render`, `search.Search`, `recent.RankByMTime`) over a deterministic scale-parameterized corpus generator (`internal/benchcorpus`). Additive test + doc code only; findings land in `docs/benchmarking.md`.
- [hypogeum-vault skill](superpowers/specs/2026-06-20-hypogeum-vault-skill-design.md) — design — an agent-facing Claude skill (`.claude/skills/hypogeum-vault/`) that teaches exploring and auditing a markdown vault via the query CLI (`neighbors`/`links`/`search`/`recent`) instead of grep, including whole-vault broken-link and orphan sweeps. Lives in-repo; symlink into `~/.claude/skills/` for global use.

## Tooling

- [Benchmarking](benchmarking.md) — how to run the hot-path benchmarks (`internal/benchcorpus` corpus + per-package `*_bench_test.go`) and the latest findings, including the 100k–1M extreme-scale sweep and the macOS vnode-cache cliff behind `recent.RankByMTime`'s super-linear scaling.
- [Link-cycle render reuse](superpowers/specs/2026-06-20-link-cycle-render-cache-design.md) — approved — split the Glamour render from the highlight pass so `n`/`p` link cycling re-applies the reverse-video via a cheap `stripSentinels` instead of a full re-render (~12.3k allocs → small constant). Adds `markdown.RenderResult` + `WithHighlight`; first follow-up from the benchmarking findings. [Plan](superpowers/plans/2026-06-20-link-cycle-render-cache.md).

## Conventions for adding to this folder

One file per topic. Kebab-case filenames, no date prefix. Update plans in place — strike-through, "Status:" lines, or check-marked steps beat parallel files. Index entries here are one line plus a short hook; the detail lives in the linked file.

Doc files cross-link with relative paths so they survive being moved between checkouts and so they navigate with the same key bindings users have for any other markdown.

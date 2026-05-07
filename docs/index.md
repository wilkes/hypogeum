# Documentation

Index of everything in `docs/`. Browse this folder by running `hypogeum docs/` from the repo root — it dogfoods the tool against itself.

## Architecture

- [Architecture overview](architecture.md) — package layering, data flow on a keystroke, where to start reading
  - [`internal/tree`](packages/tree.md) — filesystem walker that produces the left-pane tree
  - [`internal/markdown`](packages/markdown.md) — Glamour wrapper, link resolution, sentinel-instrumented render
  - [`internal/nav`](packages/nav.md) — browser-style back/forward stack, no I/O
  - [`internal/tui`](packages/tui.md) — Bubble Tea Model that wires everything together
  - [`internal/watch`](packages/watch.md) — fsnotify-backed live-update watcher, debounced and tree-aware

## Concepts

Cross-cutting ideas that show up in multiple specs and packages. Each is its own short file so the backlinks pane (`b`) shows everywhere it's referenced. Wikilink syntax is used here (and only here) to dogfood the feature; the rest of this index uses standard markdown links so GitHub renders them.

- [[sentinel-render]] — how link positions are recovered from Glamour's ANSI output
- [[vault-index]] — forward + reverse reference index, basename resolution, proximity tiebreak
- [[diagnostics]] — the warn/error stream, footer transient, log file, `?` modal
- [[modal-geometry]] — single-modal invariant and layout recompute on `B`/`?`
- [[return-cursor]] — path-keyed cursor restoration on Back navigation
- [[link-cursor]] — content-pane link selection (`n`/`p`/`Enter`/`Esc`) state model

## Active feature work

- [Link following](link-following.md) — Phase 1 shipped (cursor-based selection). Phase 2 (inline highlight) and Phase 3 (external URL launch) outlined.
- [Wikilinks and backlinks](superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md) — Phase 1 shipped: wikilinks resolve via vault index, backlinks pane (`b`), backlinks modal (`B`), log viewer (`?`). Phase 2 outlined in the spec.
- [Backlinks navigation](superpowers/specs/2026-05-07-backlinks-navigation-design.md) — spec — adds cursor, `Enter`-to-follow, scroll-to-reference, and back-restores-cursor on top of the existing backlinks display.
- [Narrow-terminal layout](superpowers/specs/2026-05-07-narrow-terminal-layout-design.md) — spec — auto-hide the tree pane below an 80-col terminal width so the two-pane layout degrades gracefully on narrow terminals.

## Conventions for adding to this folder

One file per topic. Kebab-case filenames, no date prefix. Update plans in place — strike-through, "Status:" lines, or check-marked steps beat parallel files. Index entries here are one line plus a short hook; the detail lives in the linked file.

Doc files cross-link with relative paths so they survive being moved between checkouts and so they navigate with the same key bindings users have for any other markdown.

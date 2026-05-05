# Documentation

Index of everything in `docs/`. Browse this folder by running `hypogeum docs/` from the repo root — it dogfoods the tool against itself.

## Architecture

- [Architecture overview](architecture.md) — package layering, data flow on a keystroke, where to start reading
  - [`internal/tree`](packages/tree.md) — filesystem walker that produces the left-pane tree
  - [`internal/markdown`](packages/markdown.md) — Glamour wrapper, link resolution, sentinel-instrumented render
  - [`internal/nav`](packages/nav.md) — browser-style back/forward stack, no I/O
  - [`internal/tui`](packages/tui.md) — Bubble Tea Model that wires everything together
  - [`internal/watch`](packages/watch.md) — fsnotify-backed live-update watcher, debounced and tree-aware

## Active feature work

- [Link following](link-following.md) — Phase 1 shipped (cursor-based selection). Phase 2 (inline highlight) and Phase 3 (external URL launch) outlined.

## Conventions for adding to this folder

One file per topic. Kebab-case filenames, no date prefix. Update plans in place — strike-through, "Status:" lines, or check-marked steps beat parallel files. Index entries here are one line plus a short hook; the detail lives in the linked file.

Doc files cross-link with relative paths so they survive being moved between checkouts and so they navigate with the same key bindings users have for any other markdown.

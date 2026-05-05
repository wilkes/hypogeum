# `internal/tree`

Filesystem walker that builds the `*Node` tree the TUI displays in its left pane. Knows about files; knows nothing about Bubble Tea.

See also: [architecture overview](../architecture.md), [`internal/tui`](tui.md) (consumer), [`internal/markdown`](markdown.md) (renders the file the cursor lands on).

## Purpose

Given a root directory, return a tree containing every markdown file plus the directories needed to reach them. Filter aggressively: hidden entries skipped, non-markdown files ignored, empty subtrees pruned.

## Types

```go
type Node struct {
    Path     string  // absolute
    Name     string  // basename, used for display
    IsDir    bool
    Children []*Node // nil for files; possibly empty for empty-after-pruning dirs (rare)
}
```

The root is always a directory. Children are sorted: directories first, then files, alphabetically within each group.

## Public surface

- `Walk(root string) (*Node, error)` — the only entry point.

## Key invariants

- **Empty directories are pruned.** A directory with zero markdown files anywhere underneath is dropped. Prevents the tree pane from filling with empty folders when a user points at e.g. a Documents/ folder full of PDFs.
- **Hidden entries are skipped.** Anything starting with `.` is ignored — `.git`, `.obsidian`, dotfile note dirs, etc. If you ever want to expose this, add a flag *here*, not in `tui`.
- **Walk never returns nil.** When nothing matches, it synthesizes an empty root node so callers don't have to special-case `nil`. ([tree.go:43](../../internal/tree/tree.go))
- **Recognized extensions:** `.md`, `.markdown`, `.mdown`, `.mkd`. Adding more is fine if Glamour can render them.

## Why a `*Node` tree instead of a flat list

The walker returns the natural parent/child structure. The TUI then flattens that into `[]treeRow` for keystroke speed (see [`tui`](tui.md)). Keeping the structured form here means future features that need depth or hierarchy (collapsible folders, breadcrumbs, anything tree-shaped) don't have to reconstruct it from a flat list.

## Testability

`readDir` is split into a thin wrapper (`osreaddir.go`) so the walker can be driven by a fake filesystem in tests. Currently every test hits the real filesystem via `t.TempDir()` — fast enough that the seam hasn't been used yet, but it's there if you want it.

# `internal/tree`

Filesystem walker that builds the `*Node` tree the TUI displays in its tree modal (`t`). Knows about files; knows nothing about Bubble Tea.

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

- `Walk(root string) (*Node, error)` — the main entry point.
- `MarkdownFiles(root string) ([]string, error)` — walks `root` and returns every markdown file as an absolute path in depth-first tree order (flattening the leaves of `Walk`). Returns an empty slice (never nil-with-nil-error) when nothing matches. The TUI uses it to build the file finder / search corpus.
- `IsMarkdown(name string) bool` — reports whether a filename has a recognized markdown extension. Used by `internal/watch` to filter events.
- `IsHidden(name string) bool` — reports whether a single name is hidden (starts with `.`).
- `IsHiddenPath(p string) bool` — reports whether any component of a path is hidden. Used by `internal/watch` so its filter rule matches the walker's.
- `MarkdownExts []string` — the recognized extensions, exposed for callers that need the raw slice.

## Key invariants

- **Empty directories are pruned.** A directory with zero markdown files anywhere underneath is dropped. Prevents the tree pane from filling with empty folders when a user points at e.g. a Documents/ folder full of PDFs.
- **Hidden entries are skipped.** Anything starting with `.` is ignored — `.git`, `.obsidian`, dotfile note dirs, etc. If you ever want to expose this, add a flag *here*, not in `tui`.
- **Walk never returns nil.** When nothing matches, it synthesizes an empty root node so callers don't have to special-case `nil`. ([tree.go:52](../../internal/tree/tree.go))
- **Recognized extensions:** `.md`, `.markdown`, `.mdown`, `.mkd`. Adding more is fine if Glamour can render them.

## Why a `*Node` tree instead of a flat list

The walker returns the natural parent/child structure. The TUI then flattens that into `[]treeRow` for keystroke speed (see [`tui`](tui.md)). Keeping the structured form here means future features that need depth or hierarchy (collapsible folders, breadcrumbs, anything tree-shaped) don't have to reconstruct it from a flat list.

## Testability

Every test hits the real filesystem via `t.TempDir()` — fast enough that an `os.ReadDir` seam hasn't been needed. If a future test needs a fake filesystem, the natural place to introduce one is the `os.ReadDir` call in `walk`.

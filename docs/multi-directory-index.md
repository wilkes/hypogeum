# Multi-directory index (overlay roots)

Status: shipped.

## What

Point hypogeum at **two or more** directories and have it index them as if
they were a single, superimposed directory tree — overlaying one repository
of notes over another. Wikilinks, backlinks, full-text search (`^s`) and the
`^p` finder all treat the union as one corpus; the `^b` tree shows the merged
structure.

```sh
hypogeum ~/notes ~/work/docs          # overlay two roots
hypogeum ~/a ~/b ~/c                   # three roots, all merged
hypogeum ~/notes                       # single root — unchanged behavior
```

## Semantics

The merge follows overlay-filesystem intuition:

- **Directories at the same relative path merge.** `a/sub/` and `b/sub/`
  appear as one `sub/` folder whose children are the union of both.
- **Files at the same relative path are *both* kept** ("union, keep both").
  When `a/index.md` and `b/index.md` collide, the tree shows two rows, each
  disambiguated by its source root: `index.md (a)` and `index.md (b)`.
  Nothing is hidden — there is no "last layer wins" shadowing.
- **The index is fully unified.** Wikilink resolution, backlinks, `^s` and
  `^p` see one flat set of files regardless of which root each came from. A
  `[[note]]` in root `a` can resolve to a file in root `b`; proximity scoring
  (closest file to the source) breaks ties exactly as it does within one root.

## How it works

### `tree.Merge`

`internal/tree/merge.go` adds `Merge(roots []string) (*Node, error)`:

- With **one** root, `Merge` is identical to `Walk` — single-directory
  behavior (including the absolute-path root node) is unchanged.
- With **two or more**, each root is `Walk`-ed (pruned, absolute paths) and
  the per-root subtrees are overlaid into a synthesized **virtual root** whose
  `Path` is empty and whose `Name` is the roots' base names joined with `+`.

In a merged (≥2-root) tree, **directory nodes are keyed by their slash-joined
relative path** (`"sub/deep"`) rather than an absolute path, because a merged
directory backs onto several real directories and has no single absolute path.
**File nodes keep their real absolute `Path`** so they can be opened, watched,
and resolved. Colliding files are deduped by absolute path first (so an
overlapping/repeated root doesn't double-list a file), then disambiguated by
source-root base name.

### TUI plumbing

`Model.root string` became `Model.roots []string`. Two pieces of navigation
logic that keyed off the single root were made structural so they work for
both absolute (single-root) and relative (merged) directory keys:

- **`isExpanded`** compares against the root *node pointer* (`n == m.rootNode`)
  instead of `n.Path == m.root`.
- **`expandAncestorsOf`** walks the actual node tree (`nodeChain`) to find the
  target file and expand its ancestor directory nodes, instead of doing
  `filepath.Dir` string math up to `m.root`. This is correct even when a file
  comes from a different root than the one that first named its merged parent
  directory.

Path display (footer, picker, search, backlinks, link label) goes through
`relPathForRoots(roots, abs)`, which renders the path relative to the
best-matching (shortest-relative) root, falling back to the absolute path.

### Sibling packages

- **`internal/vault`** — `BuildRoots([]string, Diagnostics)` walks and indexes
  every root into the single shared name index, so wikilinks resolve across
  roots out of the box. `Build(root, diag)` remains as a single-root wrapper.
- **`internal/watch`** — `New(roots ...string)` is variadic; it adds every
  root's directory subtree to the fsnotify watcher. Existing single-arg
  `New(dir)` calls are unaffected.
- **`internal/search`, `internal/recent`** — unchanged. Search walks
  `m.rootNode` (which already contains every file across all roots) and
  recency is keyed by absolute path.

### CLI

`resolveTarget` returns `roots []string`:

- 0 args → `[cwd]`
- 1 arg → a directory (browse it) or a file (open it, root at its parent)
- 2+ args → every argument must be a directory; all become overlay roots.
  A non-directory among multiple args is a usage error.

`tui.NewMulti(roots, initialFile)` is the multi-root entry point;
`tui.New(root, initialFile)` is a single-root wrapper kept for existing
callers and tests.

## Not in scope

- Per-root precedence / "last wins" shadowing (we keep both, by design).
- Showing the source root in the `^p` finder or `^s` results (index is
  fully unified; only the tree disambiguates collisions visually).
- Adding/removing whole roots at runtime — the root set is fixed at launch.

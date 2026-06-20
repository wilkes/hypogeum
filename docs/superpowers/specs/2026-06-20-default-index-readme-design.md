# Default landing file prefers index then readme

## Problem

When hypogeum opens a directory with no explicit initial file, it auto-opens
the *first* top-level non-directory child (alphabetical, from the tree walk).
For vaults that follow the common `index.md` / `README.md` convention, the user
would rather land on that overview file than on whatever happens to sort first
alphabetically.

## Behavior

`firstTopLevelFile` (`internal/tui/tree.go`) now resolves the landing file in
three passes over the root's direct children:

1. The first top-level non-directory child whose basename **stem** (filename
   minus its extension), compared **case-insensitively**, equals `index`
   (e.g. `index.md`, `INDEX.md`, `Index.markdown`).
2. Else the first whose stem equals `readme` (e.g. `README.md`,
   `readme.markdown`).
3. Else the first non-directory child — the prior behavior, unchanged.

The stem is computed as
`strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))`.

## Preserved scope (unchanged invariants)

- **Top-level only.** No descent into subdirectories. (An earlier version
  descended and landed on the deepest alphabetical leaf — a known regression we
  must not reintroduce.)
- **Markdown-only.** The tree walker already filters to markdown files, so this
  function never sees non-markdown entries and does not filter by extension
  itself — it only matches the stem.
- **Tie-breaking is the existing tree-walk order.** Among multiple matches
  (e.g. both `index.md` and `index.markdown`), the alphabetical walk order
  decides; no special tie-breaking is added.

# Directory listing in the content pane — design

**Status:** spec — not yet implemented.
**Scope:** when a link or a CLI argument points at a directory, render a synthesized listing of its non-hidden entries in the content pane. Today `refreshContent` calls `os.ReadFile` blindly and surfaces `Error: read /path: is a directory` in the viewport.

See also: [link-following](../../link-following.md), [docs index](../../index.md), [`internal/tree`](../../packages/tree.md), [`internal/tui`](../../packages/tui.md).

## What's broken today

`refreshContent(path)` does:

```go
src, err := os.ReadFile(path)
if err != nil {
    m.status = err.Error()
    m.content.viewport.SetContent(fmt.Sprintf("Error: %v", err))
    ...
}
```

A directory path hits `os.ReadFile`'s "is a directory" branch and the user sees the raw error. The tree pane filters non-markdown out (and prunes empty directories), so the only directories likely to be linked are the ones that contain real content — which is exactly when surfacing the listing helps.

## Fix

`refreshContent` and `applyLinkHighlight` consult `os.Stat(path)` before reading. If the entry is a directory, they synthesize a markdown listing and feed it through the existing markdown renderer.

### The synthesizer

`func renderDirListing(dir string) (markdown string, err error)` (new helper in `internal/tui/dir.go`):

```
# <dir name>

`<absolute path>`

- [..](<parent absolute path>)
- [subdir-a/](<abs path>/subdir-a)
- [subdir-b/](<abs path>/subdir-b)
- [file-1.md](<abs path>/file-1.md)
- [other-file.go](<abs path>/other-file.go)
```

Rules:
- Header: the basename of the directory (or "/" for filesystem root).
- Then the absolute path on its own line, in inline code, so the user can see context that the breadcrumb-style header omits.
- `..` parent entry, unless `dir` is the filesystem root or equals `m.root` (the tree-root anchor). Including `..` even when it exits the tree-root is fine because resolved links still work — they just don't appear in the tree pane.
- Listed entries: every `os.ReadDir` entry whose name doesn't start with `.`.
- Directories first, then files (matches tree pane sort).
- Within each group, case-insensitive alphabetical sort (matches `tree.sortChildren`).
- Subdirectories render with a trailing `/` in the visible text, no trailing slash in the href (so resolved Target is the directory path itself).
- All hrefs are absolute. This sidesteps the `ResolveLink` quirk where `filepath.Dir(base)` would strip the last segment of `base` when `base` is the directory itself — going absolute avoids ever needing to think about that.

### `refreshContent` change

Before the `os.ReadFile`:

```go
info, statErr := os.Stat(path)
if statErr == nil && info.IsDir() {
    listing, listErr := renderDirListing(path)
    if listErr != nil {
        // surface the error like any other read failure
    }
    // run listing through the markdown renderer path
    src = []byte(listing)
    // fall through to the existing markdown path
} else if statErr != nil {
    // same error handling as today
}
```

The non-markdown code-renderer dispatch is *not* taken for a directory — directory listings are always rendered through `markdown.RenderWithLinks` so the link-following plumbing applies to them. (`tree.IsMarkdown(path)` returns false for a bare directory name without an extension, so the existing branch would have wrongly tried Chroma otherwise.)

### `applyLinkHighlight` change

Same os.Stat probe, same dispatch into the synthesizer. Today this function reads the file and re-renders; for a directory it re-synthesizes the listing and re-renders. The listing is deterministic so re-synthesis is safe.

A future optimization could cache the synthesized listing on the model and skip re-synthesis when only the link cursor changed, but the cost (one `os.ReadDir`) is negligible compared to a Glamour render and the bookkeeping isn't worth it.

### Watch package

`internal/watch` already coarsens fsnotify events to `StructureChanged` (re-walk tree) and `FileModified` (re-render current file). A directory listing wants both: a file added/removed in the dir is a `StructureChanged`, but `refreshContent` for the current dir handles it the same way as any path. **No watcher changes needed** — the existing rebuild-on-StructureChanged path already triggers `refreshContent(currentPath)`, which now picks up the new entry list.

## What's not changing

- Tree pane filter: the pruned `*tree.Node` still only includes directories transitively containing markdown. We're not retrofitting "show all files" into the tree.
- `internal/tree` package: untouched. The synthesizer reads directly with `os.ReadDir` since we want all files, not the pruned set.
- Link resolution: `ResolveLink` is unchanged. Absolute hrefs in the synthesized markdown bypass the base-relative resolution path entirely.
- History: a directory visit goes through `openFile` → `m.history.Visit(path)` like a regular file. Back from a file lands you on the listing; Back from the listing pops to whatever was before.
- The recent-files store: today `recent.Record` is called on every `openFile`. A directory visit will record the directory there too. Acceptable: the picker filters its own results so directories won't show up wrongly; recent-store consumers all treat the recorded path as an opaque string. Open question: do we want to suppress recording for directories? Probably yes — they're transient navigation aids, not "files I worked on." See open questions.

## Tests

`internal/tui/dir_test.go` (new file) covers the synthesizer:

- Empty directory: returns header + `..` link + nothing else.
- Mixed entries: directories sort before files, both groups alphabetical, dir entries have trailing `/` in display text.
- Hidden filtering: `.git`, `.DS_Store` skipped.
- Path: the listing puts the directory's absolute path in the document.

`internal/tui/content_test.go` (extended) covers the dispatch:

- `refreshContent` on a directory path produces a non-empty viewport whose plain-text form contains the basename and at least one entry name.
- No `Error:` prefix in the viewport for a real directory.

## Open questions / accepted risks

- **Recent-files records directories.** Acceptable for v1; we can filter them out of recording in a follow-up if it becomes noise. The picker already filters to markdown.
- **Listing of a huge directory.** No truncation; `node_modules/` with 5000 entries will render slowly. Mitigation: hypogeum is a *vault* viewer, not a file manager — vaults aren't this shape. If it becomes a problem, cap at ~500 entries with a warning footer.
- **Symlink loops via `..`.** Not possible — `..` resolves to the real parent. The user can `..` past the tree root; that's a feature (browser-like). The tree cursor stays put when the path isn't in the pruned tree.
- **Tree-root anchor not enforced.** The `..` link from a tree-rooted listing happily takes you out of the tree. Matches link-following's general rule ("you can `cd` anywhere"). Documented in the Open Questions of `link-following.md`.
- **The link-cursor pre-select machinery.** `pendingPreselectTarget` is consumed by `refreshContent` for normal markdown; a synthesized listing has the same `RenderWithLinks` shape so it falls through cleanly. The pre-select will match a listed entry by absolute path if Back lands on the directory — which is the right behavior (the cursor lands on the file you just left).

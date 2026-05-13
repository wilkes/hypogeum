# Directory listing — implementation plan

Design: [2026-05-13-directory-listing-design.md](../specs/2026-05-13-directory-listing-design.md).
Status: shipped.

## Commit 1: synthesizer + tests

Add `internal/tui/dir.go` with `renderDirListing(dir string) (string, error)`:

- Header: `# <basename or "/">`.
- Body: absolute path in inline code, blank line, then bullet list.
- `..` first item, with `[..](absolute-parent-path)`, unless `dir` is the filesystem root.
- Then non-hidden directory entries, alphabetical, display text has trailing `/`, href is absolute path.
- Then non-hidden file entries, alphabetical, href is absolute path.

Add `internal/tui/dir_test.go` covering: empty dir, mixed entries with sort order, hidden-entry skipping, trailing slash on dir display text, absolute hrefs. No filesystem-root cases (would need `t.TempDir` adjacent to `/`).

Run: `go test ./internal/tui/ -run Dir`.

## Commit 2: dispatch in refreshContent + applyLinkHighlight

`internal/tui/content.go`: in `refreshContent`, before `os.ReadFile`, run `os.Stat(path)`. If `info.IsDir()`, call the synthesizer, treat its output as the source, and skip both the read-file error path *and* the non-markdown branch. The synthesized listing always renders through `RenderWithLinks`.

`internal/tui/links.go`: same probe in `applyLinkHighlight`.

Add a test in `internal/tui/content_test.go`:

- `refreshContent` on a directory containing `a.md` and `b.txt` produces a viewport that contains both basenames and does *not* contain "Error:".

Run: `go test ./internal/tui/`.

## Commit 3: docs

- `docs/index.md`: add an entry under "Active feature work".
- `docs/link-following.md`: note that directory targets render as a listing rather than erroring.
- `CLAUDE.md`: short gotcha — directories dispatch through the synthesizer in `refreshContent`, not through the code or markdown read-from-disk paths.
- Mark this plan and the design as **shipped**.

## Verification at end

- `go test ./... && go vet ./...` green.
- Manual smoke: `go run ./cmd/hypogeum docs/`, click the `docs/concepts/` link from `docs/architecture.md` (or wherever it appears), confirm the listing renders, `n`/`p` cycles entries, `Enter` opens a file, Back returns to the listing.
- Manual smoke at filesystem boundary: open the tree root listing via the `..` link from a top-level subdir; confirm the rendered output is sensible.

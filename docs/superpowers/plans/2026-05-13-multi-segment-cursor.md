# Multi-segment cursor — implementation plan

Design: [2026-05-13-multi-segment-cursor-design.md](../specs/2026-05-13-multi-segment-cursor-design.md).
Status: shipped.

Two commits — one for the failing test, one for the fix — kept separate so the bug shape is recorded in git history.

## Commit 1: failing test that pins the multi-segment shape

Add `TestHighlightMarker_WrappedLinkHighlightsEverySegment` in `internal/markdown/links_render_test.go`. Test must fail with the current `stripSentinels`. Verification: `go test ./internal/markdown/ -run WrappedLinkHighlightsEverySegment` reports a failure showing only the first segment wrapped.

## Commit 2: per-row open/close in `stripSentinels`

Edit `internal/markdown/links_render.go`:

- In the `case '\n':` branch: before writing the `\n`, if `inLink && openEmit`, write `closeMark` and set `openEmit = false`. The existing line-counter increment and `linkText.WriteByte('\n')` remain.
- No other changes. The default branch's existing `if inLink && !openEmit { write openMark; openEmit = true }` re-opens reverse-video on the first content byte of the next row.

Run the full markdown package tests, then the full repo: `go test ./... && go vet ./...`.

## Commit 3: docs

- Mark the link-following "Multi-segment cursor visualization" line in `docs/link-following.md` as shipped, with a pointer to the design.
- Update CLAUDE.md's link-following leftovers note (one line near the bottom of the External URL handoff section).
- Add the design entry to `docs/index.md` under "Active feature work".
- Mark this plan as **Shipped**.

## Verification at end

- `go test ./... && go vet ./...` green.
- Manual smoke: build, open a markdown file containing a long-text link (e.g. one of the `docs/superpowers/specs/*.md` files), narrow the terminal until the link wraps, cycle to it with `n`, confirm every visible row of the link is reverse-highlighted.

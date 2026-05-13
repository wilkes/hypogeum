# Source embeds — follow-up fixes

Four narrow follow-ups to [source-embeds](2026-05-13-source-embeds-design.md), surfaced by post-merge review of PR #30. None are blockers; all are confirmed real bugs or invariant gaps that the test suite doesn't catch.

## Motivation

Post-merge code review of #30 flagged four findings that each scored 75/100 on the review rubric — real, narrow, not currently frequent. Reviewing them in aggregate, three are user-visible regressions in specific sequences and one is an invariant asymmetry worth tightening. Bundling into one PR keeps churn low while making the four-fix change set independently reviewable as separate commits.

## Scope

In: the four fixes below.

Out: any redesign of embed rendering, link cycling, or the Esc cascade beyond what each fix needs. No new dependencies. No goldmark.

## Fix 1 — `preprocessEmbeds` skips fenced code blocks

### Problem

`embedTokenRegex` in `internal/markdown/links_render.go` matches `![[…]]` anywhere in the source string, including inside triple-backtick or tilde fenced code blocks. Documentation that *demonstrates* the embed syntax inside a fence (the way the design spec for #30 does) would render with that demonstration replaced by a `> ⚠ … file not found` warning blockquote — corrupting the demo. The comment above the regex falsely claims the regex matches "outside of inline code spans."

### Fix

Before the regex replacement runs, split the input into alternating non-fence and fence segments via a line-based scan. Run the regex replace only on non-fence segments; concatenate the result with the fence segments untouched.

Fence detection:
- A line opens a fence when the first non-whitespace run is 3+ backticks or 3+ tildes (after up to 3 leading spaces of indent).
- The fence closes on a line whose first non-whitespace run is the *same marker character* with *length ≥ the opening run's length*, after up to 3 leading spaces of indent.
- No other content on the closing line beyond optional trailing whitespace.

This is a deliberate subset of CommonMark — enough to cover the 99% case (well-formed docs) without pulling in a markdown parser. Inline `` `code` `` spans remain unhandled; embeds inside them will still process. The comment is updated to say so explicitly.

### Tests

In `internal/markdown/embed_render_test.go`:
- Embed token inside a triple-backtick fence renders literally (no warning, no slicing).
- Embed token inside a tilde fence renders literally.
- Embed tokens before, after, and between two fences all render normally.
- A nested-looking case: an outer ```` ```` ```` fence containing a shorter ` ``` ` line stays a single fence; the inner line is literal text.

### Comment change

Replace the misleading "outside of inline code spans" comment with text that names the actual contract: scans for `![[…]]` outside of fenced code blocks; inline code spans are not detected.

## Fix 2 — Embed link `Row=-1` no longer scroll-jumps

### Problem

`preprocessEmbeds` synthesizes one `Link` per embed with `Row: -1` (the embed has no single representative line in the rendered output). When `n`/`p` lands the cursor on such a link, `scrollToLink` in `internal/tui/links.go` evaluates `l.Row < top` (always true for `Row = -1`), computes `SetYOffset(max(0, -2)) = 0`, and silently jumps the viewport to the document top. The plan's stated intent — "the cycler treats `Row=-1` as no-scroll-on-focus" — was never implemented.

### Fix

At the top of `scrollToLink`, add `if l.Row < 0 { return }`. Cursor state still updates — `cycleLink` advances `m.content.linkCursor` *before* calling `scrollToLink`, and `applyLinkHighlight` (the next step in the cycle) re-renders the content with the new selection. Only the viewport scroll-jump is suppressed.

### Tests

In `internal/tui/model_test.go` (or the closest cycle-test neighbor):
- Render a markdown doc with enough content before an embed that the viewport scrolls past the embed. Set `YOffset` to a non-zero value. Press `n` until the cursor lands on the embed link. Assert `YOffset` is unchanged.
- Existing range-link cycling tests stay green as a sanity check.

### Comment change

Add a one-line comment next to the `Row: -1` literal in `links_render.go` naming the sentinel contract, since the contract now lives in two places (the link site and the cycler).

## Fix 3 — Esc clearing `rangeHighlight` preserves scroll position

### Problem

The Esc cascade in `internal/tui/input.go` has a new branch (added in commit `75c813e`) that clears `m.content.rangeHighlight` and calls `m.refreshContent(cur)` when the open file is non-markdown. `refreshContent` calls `viewport.GotoTop()`, snapping the viewport to line 1. The link-cursor branch immediately below it already saves and restores `YOffset` around its own `refreshContent` — the new branch did not. Pressing Esc on a code file to dismiss the gutter highlight therefore loses the user's scroll position.

### Fix

Mirror the existing save/restore pattern from the link-cursor branch:

```go
if m.content.rangeHighlight != nil && !tree.IsMarkdown(cur) {
    offset := m.content.viewport.YOffset
    m.content.rangeHighlight = nil
    if err := m.refreshContent(cur); err == nil {
        m.content.viewport.SetYOffset(offset)
    }
    return m, nil
}
```

The `err == nil` guard matches the existing pattern: if the refresh fails the viewport now shows an error message, and restoring an offset into that would be wrong.

### Tests

Extend `TestModel_EscClearsRangeHighlight` (or its nearest neighbor) in `internal/tui/model_test.go`:
- Navigate into a code file via a range link so `rangeHighlight` is set.
- Set `m.content.viewport.YOffset` to a non-zero value.
- Press Esc.
- Assert `rangeHighlight == nil` AND `YOffset` is unchanged.

## Fix 4 — `followBacklink` captures `pendingPreselectRange`

### Problem

Back/Forward (`internal/tui/input.go`) capture `m.content.rangeHighlight` into `m.pendingPreselectRange` before navigating, so the highlight is reapplied after the move. `followBacklink` (`internal/tui/backlinks.go`) sets `m.pendingPreselectTarget` but not `m.pendingPreselectRange`. The path is currently unreachable through `vault.Build` (vault is markdown-only, and only code files have `rangeHighlight` set) but the asymmetry is a latent gap.

### Fix

In `followBacklink`, alongside `m.pendingPreselectTarget = m.history.Current()`, add `m.pendingPreselectRange = m.content.rangeHighlight`. One line, same shape as Back/Forward.

### Tests

Because the precondition is unreachable through normal vault construction, the test exercises the field assignment directly:

- Construct a model with `m.content.rangeHighlight` set to a non-nil `LineRange` and a hand-built backlink list.
- Invoke `followBacklink` directly.
- Assert `m.pendingPreselectRange == m.content.rangeHighlight` (the value captured before refresh clears the live field).

The point is to lock the invariant against future regressions, not to exercise an end-to-end user path.

### CLAUDE.md addendum

Add a single sentence to the existing "Range-link Enter sets `m.content.rangeHighlight`" gotcha noting that every navigation-out path (Back, Forward, followBacklink) captures `rangeHighlight` into `pendingPreselectRange`. The intent is to keep the next contributor from adding a fifth navigation path that forgets the capture.

## Out of scope

- Inline `` `code` `` span handling in `preprocessEmbeds`. Not requested, not in the design spec, and the surrounding parser logic doesn't handle it for wikilinks either — out of scope keeps the fix narrow.
- Walking the rendered output to give embed links a real `Row` value. The sentinel-return fix is sufficient and matches stated intent.
- Pushing YOffset save/restore into `refreshContent` itself. Wider blast radius and not necessary to fix this bug.
- Documenting backlinks for non-markdown files as a future feature. The fix lands the invariant; if future work adds the path, no extra change is needed.

## Branch and PR shape

- Branch `source-embeds-followups` off `main`.
- One commit per fix (4 fix commits) plus this spec commit at the start of the branch.
- Merge via `gh pr merge --merge` (squashing disabled by repo policy).

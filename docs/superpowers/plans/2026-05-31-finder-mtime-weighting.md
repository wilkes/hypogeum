# Finder mtime weighting — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Demote the visit-history weight in `internal/recent` from `1.5` to `0.5` so the `^p` finder ranks recently *modified* files above recently *visited* files at equal age.

**Architecture:** One package-level constant change in `internal/recent/recent.go`, two test edits, and one docstring update in `CLAUDE.md`. No API change, no migration, no callers touched. The `recent` package's existing layering (math + persistence; UI-unaware) means the picker, vault, and `^s` search pick up the new ranking with no code change.

**Tech Stack:** Go 1.x, stdlib only for the recent package. Tests run via `go test`.

**Spec:** [docs/superpowers/specs/2026-05-31-finder-mtime-weighting-design.md](../specs/2026-05-31-finder-mtime-weighting-design.md)

**Branch:** `finder-mtime-weighting` (already checked out off `main`; spec already committed).

---

## Task 1: Add tests that pin the new invariant (red)

Codify both the new `TestScoreOnlyVisit` expected range and the new `TestScoreRecentEditBeatsRecentVisit` invariant. Both should *fail* while `visitWeight` is still `1.5` — that's the TDD red.

**Files:**
- Modify: `internal/recent/recent_test.go:54-65` (update one existing test's expected range)
- Modify: `internal/recent/recent_test.go` (append one new test before the closing of the file's test set; place after `TestScoreRecentVisitBeatsRecentEdit`, ~line 91)

- [ ] **Step 1: Update `TestScoreOnlyVisit` expected range**

Find the existing test at `internal/recent/recent_test.go:54-65`. The current body is:

```go
func TestScoreOnlyVisit(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	// Visited 1 hour ago, file very old (mtime contribution near zero).
	var mtime time.Time = now.Add(-10000 * time.Hour) // way more than 7-day half life
	visit := now.Add(-1 * time.Hour)
	got := score(now, mtime, visit)

	// mtime term ≈ 0, visit term ≈ 1.5 · exp(-1/48) ≈ 1.469
	if got < 1.46 || got > 1.5 {
		t.Errorf("very-old mtime, 1h visit: got %v, want in [1.46, 1.5]", got)
	}
}
```

Change the comment and the range to reflect the new weight (`0.5 · exp(-1/48) ≈ 0.490`):

```go
func TestScoreOnlyVisit(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	// Visited 1 hour ago, file very old (mtime contribution near zero).
	var mtime time.Time = now.Add(-10000 * time.Hour) // way more than 7-day half life
	visit := now.Add(-1 * time.Hour)
	got := score(now, mtime, visit)

	// mtime term ≈ 0, visit term ≈ 0.5 · exp(-1/48) ≈ 0.490
	if got < 0.48 || got > 0.50 {
		t.Errorf("very-old mtime, 1h visit: got %v, want in [0.48, 0.50]", got)
	}
}
```

- [ ] **Step 2: Add `TestScoreRecentEditBeatsRecentVisit` after the existing recent-visit tests**

Append this new test directly after `TestScoreRecentVisitBeatsRecentEdit` (line ~91), before the `TestStoreRecordAndRankBasic` block:

```go
func TestScoreRecentEditBeatsRecentVisit(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)

	// File A: edited 1 hour ago, never visited.
	scoreA := score(now, now.Add(-1*time.Hour), time.Time{})
	// File B: never edited (very old mtime), visited 1 hour ago.
	scoreB := score(now, now.Add(-10000*time.Hour), now.Add(-1*time.Hour))

	if scoreA <= scoreB {
		t.Errorf("equal-age: recent edit should outrank recent visit: edit=%v visit=%v", scoreA, scoreB)
	}
}
```

- [ ] **Step 3: Run the recent tests to verify the new pair fails on the current constant**

Run: `go test ./internal/recent/...`
Expected: two failures.
- `TestScoreOnlyVisit` fails because the unchanged code returns ~`1.469`, outside the new `[0.48, 0.50]` range.
- `TestScoreRecentEditBeatsRecentVisit` fails because `scoreA ≈ 0.994` and `scoreB ≈ 1.469` (visit term × 1.5 > mtime term).

If either test passes, stop and re-read the test — the assertion is mis-stated.

- [ ] **Step 4: Commit the red tests**

```bash
git add internal/recent/recent_test.go
git commit -m "$(cat <<'EOF'
test(recent): pin new mtime-dominates-visits invariant (red)

Update TestScoreOnlyVisit's expected range for visitWeight=0.5 and
add TestScoreRecentEditBeatsRecentVisit. Both fail until the next
commit lowers visitWeight; committing red so the regression test is
auditable.
EOF
)"
```

---

## Task 2: Lower `visitWeight` from 1.5 to 0.5 (green)

**Files:**
- Modify: `internal/recent/recent.go:32-35` (the `visitWeight` constant and its doc comment)

- [ ] **Step 1: Change the constant and its comment**

Find lines 32-35 in `internal/recent/recent.go`:

```go
	// visitWeight scales the visit-history term relative to the mtime
	// term. >1 means an equally-aged visit outranks an equally-aged edit.
	visitWeight = 1.5
)
```

Replace with:

```go
	// visitWeight scales the visit-history term relative to the mtime
	// term. <1 means an equally-aged edit outranks an equally-aged
	// visit; visits still contribute positively, just at a reduced
	// weight so they nudge ranking rather than dominate it.
	visitWeight = 0.5
)
```

- [ ] **Step 2: Run the recent tests to verify everything passes**

Run: `go test ./internal/recent/...`
Expected: all tests pass, including the two from Task 1.

Sanity-check the math while you're here:
- `TestScoreOnlyVisit`: `0.5 · exp(-1/48) = 0.5 × 0.9794 ≈ 0.490` → inside `[0.48, 0.50]` ✓.
- `TestScoreRecentEditBeatsRecentVisit`: edit ≈ `0.994`, visit ≈ `0.490`. Edit wins ✓.
- `TestScoreRecentVisitBeatsRecentEdit` (the existing pre-image): equal-mtime + visit (`0.994 + 0.490 = 1.484`) still beats equal-mtime no-visit (`0.994`) ✓.

- [ ] **Step 3: Run the whole suite race-clean**

Run: `go test -race ./...`
Expected: PASS across all packages. CI runs the same command, so this guards against surprise.

- [ ] **Step 4: Commit the green code**

```bash
git add internal/recent/recent.go
git commit -m "$(cat <<'EOF'
feat(recent): demote visitWeight from 1.5 to 0.5

Recently modified files now outrank recently visited files at equal
age in the ^p finder. Visits still contribute positively (so they
remain a tiebreaker for equal-mtime files) but no longer dominate
the score.

At age 1h: mtime ≈ 0.994 vs visit ≈ 0.490 — mtime wins by ~2x.

See docs/superpowers/specs/2026-05-31-finder-mtime-weighting-design.md
for design rationale and the rejected alternatives.
EOF
)"
```

---

## Task 3: Update CLAUDE.md prose

The `^p` description in `CLAUDE.md` says "× 1.5" explicitly. Flip the number and tweak the qualifier so future readers see the new regime.

**Files:**
- Modify: `CLAUDE.md:71` (the `^p` bullet's parenthetical)

- [ ] **Step 1: Edit the parenthetical**

Find this fragment within the `^p` bullet (line 71):

```
The hybrid recency score (filesystem mtime, 7-day half-life + persisted visits, 2-day half-life × 1.5) lives in `internal/recent`.
```

Replace with:

```
The hybrid recency score (filesystem mtime, 7-day half-life + persisted visits, 2-day half-life × 0.5 — mtime dominates at equal age) lives in `internal/recent`.
```

- [ ] **Step 2: Commit the doc change**

```bash
git add CLAUDE.md
git commit -m "$(cat <<'EOF'
docs(claude.md): note visit weight is now 0.5, mtime-dominant
EOF
)"
```

---

## Task 4: Verify, push, open PR

- [ ] **Step 1: Final verification**

Run from repo root, in this order:

```bash
go build ./...
go vet ./...
go test -race ./...
```

Expected: all three succeed silently (or with PASS output for the test command). Any failure here means stop — do not push.

- [ ] **Step 2: Push the branch**

```bash
git push -u origin finder-mtime-weighting
```

- [ ] **Step 3: Open the PR**

```bash
gh pr create --title "Demote finder visit-weight: mtime wins at equal age" --body "$(cat <<'EOF'
## Summary

- Lowers `visitWeight` in `internal/recent` from `1.5` to `0.5` so the `^p` finder ranks recently *modified* files above recently *visited* files at equal age.
- Visits still contribute positively (tiebreaker role preserved), just at one-third their old weight.
- No API change, no migration, no callers touched.

## Spec

[docs/superpowers/specs/2026-05-31-finder-mtime-weighting-design.md](docs/superpowers/specs/2026-05-31-finder-mtime-weighting-design.md)

## Test plan

- [ ] `go test -race ./...` passes (covers updated `TestScoreOnlyVisit` and new `TestScoreRecentEditBeatsRecentVisit`)
- [ ] Manual: open `^p` against a vault with a mix of recently-edited-but-unopened files and recently-opened-but-stale files; confirm edits float to the top
- [ ] Manual: typed-query `^p` ordering still feels stable (match score dominates; recency is only a tiebreaker)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: `gh` prints the PR URL. Capture it and return it to the user.

- [ ] **Step 4: Confirm CI**

Watch the PR's first check. The workflow is `.github/workflows/ci.yml` (`go build`, `go vet`, `go test -race`). If any check fails, investigate before merging.

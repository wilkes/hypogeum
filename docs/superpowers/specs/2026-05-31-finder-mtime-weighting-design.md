# Finder mtime weighting

Rebalance the `^p` finder's recency score so a recently *modified* file outranks a recently *visited* file at equal age. One-constant tweak in `internal/recent/recent.go`; no API change, no migration.

## Background

`internal/recent` computes a hybrid exponential-decay score from two signals:

- **mtime term** — `exp(-hours_since_mtime / 168)` (7-day half-life).
- **visit term** — `1.5 · exp(-hours_since_visit / 48)` (2-day half-life, weighted ×1.5).

The two terms are summed; higher is better. Today's `visitWeight = 1.5` plus the faster decay of visits combine so that, at equal age, the visit term outranks the mtime term by a large margin. Example at age = 24h: mtime contributes `0.905`; visits contribute `1.060`. So a file opened yesterday but never edited ranks above a file edited yesterday but never opened.

The intended behavior is the inverse: edits should be the dominant freshness signal, and visit history should nudge ranking, not steer it.

## Decision

Set `visitWeight = 0.5` (down from `1.5`). Leave both half-lives unchanged.

At age = 24h, the new contributions are mtime `0.905` vs visit `0.353` — mtime wins by ~2.5×. Visits are still positive, so the existing test that asserts "equal mtime + a visit beats equal mtime + no visit" still passes; visits keep their tiebreaker role but no longer dominate.

### Why not other approaches

- **Pure mtime sort** would simplify the implementation but loses the legitimate signal that "I keep returning to this file" carries about attention. Visit history still helps when several files share similar mtimes.
- **Add `mtimeWeight = 2.0`** introduces a second knob without buying anything that lowering `visitWeight` doesn't already buy. The two terms are summed, so only their ratio matters; the simpler edit is to move one constant.
- **Shorten the mtime half-life** would change *how sharply* mtime freshness decays, which is a separate design question from "edits vs visits at equal age." Out of scope for this tweak.

## Scope

In:

- `internal/recent/recent.go` — change the `visitWeight` constant from `1.5` to `0.5`. Update the doc comment on the constant to describe the new regime (`<1` means visits rank below mtime at equal age).
- `internal/recent/recent_test.go` — update `TestScoreOnlyVisit`'s expected range from `[1.46, 1.5]` to `[0.48, 0.50]`. Add `TestScoreRecentEditBeatsRecentVisit` codifying the new invariant.
- `CLAUDE.md` — the `^p` paragraph mentions the visit weighting as "× 1.5"; flip the prose and the number.

Out:

- Exposing weights as flags or config. The constants' own comments call out that tuning is meant to be a one-line code change.
- Changing half-lives.
- Touching `^s` ranking, the picker UI, or the visits state file format.

## Behavior changes

- Empty-query `^p` ordering: shifts to favor recently modified files. A file edited an hour ago will be at or near the top even if you haven't opened it; a file you opened an hour ago but is years old drops below recent edits.
- Typed-query `^p` ordering: unchanged in practice. Fuzzy-match score dominates; recency is a stable tiebreaker. Tiebreak ordering shifts in the same direction as the empty-query case but the user-visible effect is minimal.
- `^s` full-text search: same shift in tiebreaker behavior.
- The persisted visits file (`~/.config/hypogeum/visits.json`) is unchanged. No migration.

## Verification

- `go test ./internal/recent/...` — updated test passes; new test pins the invariant.
- `go test ./...` — full suite stays green.
- Manual: open `^p` against a vault with a mix of recently-edited-but-unopened files and recently-opened-but-stale files; confirm edits float to the top.

## Risks

- Anyone whose mental model of `^p` is "things I've been working on" rather than "things that have changed" will feel the difference. The user explicitly asked for this; documenting in `CLAUDE.md` is sufficient.
- No data loss or backwards compat concern: the constant is read at runtime, the visits file remains identical.

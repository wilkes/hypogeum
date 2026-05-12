package recent

import (
	"testing"
	"time"
)

func TestRankedZeroValue(t *testing.T) {
	r := Ranked{}
	if r.Path != "" {
		t.Errorf("zero Ranked.Path: got %q want \"\"", r.Path)
	}
	if r.Score != 0 {
		t.Errorf("zero Ranked.Score: got %v want 0", r.Score)
	}
	if !r.MTime.IsZero() {
		t.Errorf("zero Ranked.MTime: got %v want zero", r.MTime)
	}
	if !r.Visit.IsZero() {
		t.Errorf("zero Ranked.Visit: got %v want zero", r.Visit)
	}
}

func TestConstants(t *testing.T) {
	// Sanity check that the published constants are positive and finite.
	if mtimeHalfLifeHours <= 0 {
		t.Errorf("mtimeHalfLifeHours must be > 0, got %v", mtimeHalfLifeHours)
	}
	if visitHalfLifeHours <= 0 {
		t.Errorf("visitHalfLifeHours must be > 0, got %v", visitHalfLifeHours)
	}
	if visitWeight <= 0 {
		t.Errorf("visitWeight must be > 0, got %v", visitWeight)
	}
	_ = time.Now() // keeps the time import used in this file
}

func TestScoreOnlyMTime(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	// File edited 1 hour ago, never visited.
	mtime := now.Add(-1 * time.Hour)
	var visit time.Time // zero
	got := score(now, mtime, visit)

	// score = exp(-1/168) + 0  ≈  0.9941
	if got < 0.99 || got > 1.0 {
		t.Errorf("1h-old mtime, no visit: got %v, want in [0.99, 1.0]", got)
	}
}

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

func TestScoreRecentVisitBeatsOldEdit(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)

	// File A: edited 8 days ago, never visited.
	scoreA := score(now, now.Add(-8*24*time.Hour), time.Time{})
	// File B: edited 8 days ago, visited 1 day ago.
	scoreB := score(now, now.Add(-8*24*time.Hour), now.Add(-1*24*time.Hour))

	if scoreB <= scoreA {
		t.Errorf("recent visit should outrank no-visit: A=%v B=%v", scoreA, scoreB)
	}
}

func TestScoreRecentVisitBeatsRecentEdit(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)

	// File A: edited 1 hour ago, never visited.
	scoreA := score(now, now.Add(-1*time.Hour), time.Time{})
	// File B: edited 1 hour ago, visited 1 hour ago.
	scoreB := score(now, now.Add(-1*time.Hour), now.Add(-1*time.Hour))

	if scoreB <= scoreA {
		t.Errorf("equal-mtime: visited should outrank not-visited: A=%v B=%v", scoreA, scoreB)
	}
}

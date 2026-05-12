package recent

import (
	"os"
	"path/filepath"
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

func TestStoreRecordAndRankBasic(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.md")
	p2 := filepath.Join(dir, "b.md")
	if err := os.WriteFile(p1, []byte("# A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, []byte("# B"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Store{
		visits:  map[string]time.Time{},
		nowFunc: time.Now,
	}

	// Visit p1 first, then p2 — p2 is more recent, should rank higher
	// (assuming both files have similar mtimes).
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	s.nowFunc = func() time.Time { return now }
	s.visits[p1] = now.Add(-2 * time.Hour)
	s.visits[p2] = now.Add(-1 * time.Hour)

	ranked := s.Rank([]string{p1, p2})
	if len(ranked) != 2 {
		t.Fatalf("Rank returned %d entries, want 2", len(ranked))
	}
	if ranked[0].Path != p2 {
		t.Errorf("Rank order: first entry %q, want %q (more recent visit)", ranked[0].Path, p2)
	}
}

func TestStoreRankDropsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.md")
	pMissing := filepath.Join(dir, "missing.md")
	if err := os.WriteFile(p1, []byte("# A"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Store{visits: map[string]time.Time{}, nowFunc: time.Now}
	ranked := s.Rank([]string{p1, pMissing})

	if len(ranked) != 1 {
		t.Fatalf("Rank returned %d entries, want 1 (missing file should drop)", len(ranked))
	}
	if ranked[0].Path != p1 {
		t.Errorf("only entry: got %q want %q", ranked[0].Path, p1)
	}
}

func TestStoreRecordUpdatesVisitTime(t *testing.T) {
	s := &Store{visits: map[string]time.Time{}, nowFunc: time.Now}
	fixed := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	s.nowFunc = func() time.Time { return fixed }

	// Record into a Store with no stateFile — write should be a no-op
	// (covered in Task 4); for now we just check the in-memory map.
	s.stateFile = "" // explicit: persistence disabled
	if err := s.Record("/abs/path/x.md"); err != nil {
		t.Fatalf("Record with empty stateFile: %v", err)
	}
	got, ok := s.visits["/abs/path/x.md"]
	if !ok {
		t.Fatal("Record did not add to visits map")
	}
	if !got.Equal(fixed) {
		t.Errorf("Record stored %v, want %v", got, fixed)
	}
}

func TestStoreRankEmpty(t *testing.T) {
	s := &Store{visits: map[string]time.Time{}, nowFunc: time.Now}
	ranked := s.Rank(nil)
	if len(ranked) != 0 {
		t.Errorf("Rank(nil) returned %d entries, want 0", len(ranked))
	}
}

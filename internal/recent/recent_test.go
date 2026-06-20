package recent

import (
	"encoding/json"
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

	// mtime term ≈ 0, visit term ≈ 0.5 · exp(-1/48) ≈ 0.490
	if got < 0.48 || got > 0.50 {
		t.Errorf("very-old mtime, 1h visit: got %v, want in [0.48, 0.50]", got)
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

	// Persistence checks land in Task 4; for now we only verify the
	// in-memory map is updated.
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

func TestRankPaths(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.md")
	p2 := filepath.Join(dir, "b.md")
	if err := os.WriteFile(p1, []byte("# A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, []byte("# B"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Store{visits: map[string]time.Time{}, nowFunc: time.Now}
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	s.nowFunc = func() time.Time { return now }
	s.visits[p1] = now.Add(-2 * time.Hour)
	s.visits[p2] = now.Add(-1 * time.Hour)

	paths := []string{p1, p2}

	// RankPaths must return exactly Rank mapped to .Path, in the same order.
	ranked := s.Rank(paths)
	want := make([]string, len(ranked))
	for i, r := range ranked {
		want[i] = r.Path
	}

	got := s.RankPaths(paths)
	if len(got) != len(want) {
		t.Fatalf("RankPaths len: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("RankPaths[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestRankPathsDropsMissing(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.md")
	pMissing := filepath.Join(dir, "missing.md")
	if err := os.WriteFile(p1, []byte("# A"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Store{visits: map[string]time.Time{}, nowFunc: time.Now}
	got := s.RankPaths([]string{p1, pMissing})
	if len(got) != 1 || got[0] != p1 {
		t.Errorf("RankPaths dropping missing: got %v want [%q]", got, p1)
	}
}

func TestStoreRankEmpty(t *testing.T) {
	s := &Store{visits: map[string]time.Time{}, nowFunc: time.Now}
	ranked := s.Rank(nil)
	if len(ranked) != 0 {
		t.Errorf("Rank(nil) returned %d entries, want 0", len(ranked))
	}
}

func TestNewWithMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "visits.json")

	s, err := New(path)
	if err != nil {
		t.Fatalf("New on missing file: got error %v, want nil", err)
	}
	if s == nil {
		t.Fatal("New returned nil Store")
	}
	if len(s.visits) != 0 {
		t.Errorf("Store visits: got %d entries, want 0", len(s.visits))
	}
	if s.stateFile != path {
		t.Errorf("Store.stateFile: got %q want %q", s.stateFile, path)
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "visits.json")
	// Create a "vault" file so Rank has something stat-able.
	vaultFile := filepath.Join(dir, "note.md")
	if err := os.WriteFile(vaultFile, []byte("# N"), 0o644); err != nil {
		t.Fatal(err)
	}

	s1, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Record(vaultFile); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// New Store loaded from same path should see the visit.
	s2, err := New(path)
	if err != nil {
		t.Fatalf("re-load: %v", err)
	}
	if _, ok := s2.visits[vaultFile]; !ok {
		t.Errorf("re-loaded visits missing %q; have %v", vaultFile, s2.visits)
	}
}

func TestNewWithMalformedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "visits.json")
	if err := os.WriteFile(path, []byte("not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := New(path)
	if err == nil {
		t.Error("New on malformed file: got nil error, want one")
	}
	if s == nil {
		t.Fatal("New returned nil Store even on error")
	}
	if len(s.visits) != 0 {
		t.Errorf("Store visits on malformed: got %d entries, want 0", len(s.visits))
	}
}

func TestNewWithUnknownVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "visits.json")
	bad := `{"version":99,"visits":{}}`
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := New(path)
	if err == nil {
		t.Error("New on unknown version: got nil error, want one")
	}
	if s == nil || len(s.visits) != 0 {
		t.Errorf("Store on unknown version: want non-nil empty Store")
	}
}

func TestRecordWritesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "visits.json")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Record("/abs/x.md"); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Real file exists, .tmp does not.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("real visits.json missing: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file should have been renamed away; stat err=%v", err)
	}

	// Content is well-formed JSON with our format.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fileShape struct {
		Version int                  `json:"version"`
		Visits  map[string]time.Time `json:"visits"`
	}
	if err := json.Unmarshal(data, &fileShape); err != nil {
		t.Fatalf("parse written file: %v", err)
	}
	if fileShape.Version != 1 {
		t.Errorf("file version: got %d want 1", fileShape.Version)
	}
	if _, ok := fileShape.Visits["/abs/x.md"]; !ok {
		t.Errorf("written visits missing /abs/x.md; got %v", fileShape.Visits)
	}
}

func TestDefaultStateFile(t *testing.T) {
	p, err := DefaultStateFile()
	if err != nil {
		t.Fatalf("DefaultStateFile: %v", err)
	}
	if !filepath.IsAbs(p) {
		t.Errorf("DefaultStateFile returned non-absolute %q", p)
	}
	if filepath.Base(p) != "visits.json" {
		t.Errorf("DefaultStateFile basename: got %q want visits.json", filepath.Base(p))
	}
}

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
	if !r.MTime.IsZero() {
		t.Errorf("zero Ranked.MTime: got %v want zero", r.MTime)
	}
	if !r.Visit.IsZero() {
		t.Errorf("zero Ranked.Visit: got %v want zero", r.Visit)
	}
}

// chtime sets a deterministic mtime on path so RankByMTime ordering is
// independent of test execution timing.
func chtime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestRankByMTimeSortsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "older.md")
	newer := filepath.Join(dir, "newer.md")
	if err := os.WriteFile(older, []byte("# old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("# new"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	chtime(t, older, base.Add(-2*time.Hour))
	chtime(t, newer, base.Add(-1*time.Hour))

	// Pass in reverse order to prove the function sorts rather than echoes.
	ranked := RankByMTime([]string{older, newer})
	if len(ranked) != 2 {
		t.Fatalf("RankByMTime returned %d entries, want 2", len(ranked))
	}
	if ranked[0].Path != newer {
		t.Errorf("first entry %q, want %q (newest mtime)", ranked[0].Path, newer)
	}
	if ranked[1].Path != older {
		t.Errorf("second entry %q, want %q", ranked[1].Path, older)
	}
	// MTime is carried through.
	if ranked[0].MTime.IsZero() {
		t.Error("RankByMTime should populate MTime")
	}
}

func TestRankByMTimeDropsStatFailures(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.md")
	missing := filepath.Join(dir, "missing.md")
	if err := os.WriteFile(good, []byte("# good"), 0o644); err != nil {
		t.Fatal(err)
	}
	ranked := RankByMTime([]string{good, missing})
	if len(ranked) != 1 {
		t.Fatalf("RankByMTime returned %d entries, want 1 (missing dropped)", len(ranked))
	}
	if ranked[0].Path != good {
		t.Errorf("surviving entry %q, want %q", ranked[0].Path, good)
	}
}

func TestRankByMTimeEmpty(t *testing.T) {
	if got := RankByMTime(nil); len(got) != 0 {
		t.Errorf("RankByMTime(nil) returned %d entries, want 0", len(got))
	}
}

func TestRankPathsByMTime(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "older.md")
	newer := filepath.Join(dir, "newer.md")
	if err := os.WriteFile(older, []byte("# old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("# new"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	chtime(t, older, base.Add(-2*time.Hour))
	chtime(t, newer, base.Add(-1*time.Hour))

	got := RankPathsByMTime([]string{older, newer})
	want := []string{newer, older}
	if len(got) != len(want) {
		t.Fatalf("RankPathsByMTime len: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("RankPathsByMTime[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestRankByVisitVisitedOnlyMostRecentFirst(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	unvisited := filepath.Join(dir, "c.md")
	for _, p := range []string{a, b, unvisited} {
		if err := os.WriteFile(p, []byte("# x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	s := &Store{visits: map[string]time.Time{}, nowFunc: func() time.Time { return now }}
	s.visits[a] = now.Add(-2 * time.Hour)
	s.visits[b] = now.Add(-1 * time.Hour) // more recent

	ranked := s.RankByVisit([]string{a, b, unvisited})
	if len(ranked) != 2 {
		t.Fatalf("RankByVisit returned %d entries, want 2 (unvisited excluded)", len(ranked))
	}
	if ranked[0].Path != b {
		t.Errorf("first entry %q, want %q (most recent visit)", ranked[0].Path, b)
	}
	if ranked[1].Path != a {
		t.Errorf("second entry %q, want %q", ranked[1].Path, a)
	}
	for _, r := range ranked {
		if r.Path == unvisited {
			t.Errorf("unvisited file %q should be excluded", unvisited)
		}
		if r.Visit.IsZero() {
			t.Errorf("ranked entry %q has zero Visit", r.Path)
		}
	}
}

func TestRankByVisitExcludesNeverVisited(t *testing.T) {
	s := &Store{visits: map[string]time.Time{}, nowFunc: time.Now}
	ranked := s.RankByVisit([]string{"/abs/never.md", "/abs/also.md"})
	if len(ranked) != 0 {
		t.Errorf("RankByVisit with no visits: got %d entries, want 0", len(ranked))
	}
}

func TestRankByVisitEmpty(t *testing.T) {
	s := &Store{visits: map[string]time.Time{}, nowFunc: time.Now}
	if got := s.RankByVisit(nil); len(got) != 0 {
		t.Errorf("RankByVisit(nil) returned %d entries, want 0", len(got))
	}
}

// TestRankByVisitDoesNotRequireFileOnDisk documents that visit ordering is
// purely about the recorded visit timestamp — it does not os.Stat the paths,
// so a file that was visited and later deleted still appears (callers decide
// whether to filter). This keeps visit-recency independent of the filesystem.
func TestRankByVisitIncludesPathsWithoutStat(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	s := &Store{visits: map[string]time.Time{}, nowFunc: func() time.Time { return now }}
	gone := "/nonexistent/gone.md"
	s.visits[gone] = now.Add(-1 * time.Hour)

	ranked := s.RankByVisit([]string{gone})
	if len(ranked) != 1 || ranked[0].Path != gone {
		t.Errorf("RankByVisit should include visited-but-missing path: got %v", ranked)
	}
}

func TestStoreRecordUpdatesVisitTime(t *testing.T) {
	s := &Store{visits: map[string]time.Time{}, nowFunc: time.Now}
	fixed := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	s.nowFunc = func() time.Time { return fixed }
	s.stateFile = "" // persistence disabled
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

	if _, err := os.Stat(path); err != nil {
		t.Errorf("real visits.json missing: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file should have been renamed away; stat err=%v", err)
	}

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

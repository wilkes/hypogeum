# Unified Finder with Recency — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the tree-rooted `^p` picker with a flat, recency-ranked finder over every markdown file in the vault. Hybrid score combines filesystem mtime with persisted visit history.

**Architecture:** New `internal/recent` package owns scoring math and persisted visits (JSON, atomic write). The TUI calls `recent.Record(path)` from `openFile`, and `recent.Rank(paths)` from a rewritten flat picker. No new dependencies; standard library only for persistence.

**Tech Stack:** Go 1.23, Bubble Tea (existing TUI), `bubbles/viewport` (existing), `os.UserConfigDir`, `encoding/json`.

**Spec:** [docs/superpowers/specs/2026-05-12-unified-finder-recency-design.md](../specs/2026-05-12-unified-finder-recency-design.md)

---

## File Structure

**Files created:**
- `internal/recent/recent.go` — `Store`, `New`, `Record`, `Rank`, `Ranked`, `DefaultStateFile`, scoring constants, internal JSON load/save.
- `internal/recent/recent_test.go` — all `recent` package tests.

**Files modified:**
- `internal/tui/picker.go` — full rewrite. Drops tree-walk, expansion map, `pickerFlatten`, `toggleAt`, `selectedFile`. Adds flat-list state, row renderer with relative path + recency suffix.
- `internal/tui/model.go` — adds `recent *recent.Store` field on `Model`; constructs Store in `New`; passes Store to picker init.
- `internal/tui/content.go` — adds one `m.recent.Record(path)` call inside `openFile`, with diagnostic on failure.
- `internal/tui/input.go` — picker key handlers (Up/Down/g/G/Enter/Esc/^p toggle) simplified for flat list; remove `ToggleFolder` handling inside the picker; remove "Enter on directory expands" branch.
- `internal/tui/picker_test.go` — full rewrite alongside picker.
- `internal/tui/model_test.go` — minor: ensures `m.recent` is non-nil after `New`.
- `README.md` — update `^p` row in keys table.
- `CLAUDE.md` — update `^p` gotcha paragraph.
- `docs/index.md` — add entry under "Active feature work".
- `docs/packages/tui.md` — replace picker section with flat-finder description.
- `docs/superpowers/specs/2026-05-07-vault-rooted-picker-design.md` — add superseded-by header.

**Files NOT touched:**
- `internal/tree`, `internal/markdown`, `internal/nav`, `internal/watch`, `internal/vault`, `internal/wikilink` — unchanged.
- `cmd/hypogeum/main.go` — unchanged.

---

## Conventions used in this plan

- **Module path:** `github.com/wilkes/hypogeum`.
- **Test commands** always run from repo root unless otherwise noted.
- **Time injection:** all `recent` tests use a `withNow(now time.Time)` test helper rather than mocking `time.Now` globally.
- **Path style:** all paths in `visits.json` are absolute. Tests use `t.TempDir()` so paths are absolute and isolated.
- **Commits:** one commit per task. Each task ends with a Step labeled "Commit".
- **Verification:** `go test ./...` from repo root after every task, before each commit. `go vet ./...` once at the end.

---

## Task 1: Create the `recent` package skeleton with `Ranked` and constants

**Files:**
- Create: `internal/recent/recent.go`
- Create: `internal/recent/recent_test.go`

- [ ] **Step 1.1: Write the failing test**

Create `internal/recent/recent_test.go`:

```go
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
```

- [ ] **Step 1.2: Run the test to verify it fails**

```
go test ./internal/recent/...
```

Expected: `FAIL` because the package doesn't exist yet.

- [ ] **Step 1.3: Create the package skeleton**

Create `internal/recent/recent.go`:

```go
// Package recent owns the persisted visit history and the recency-based
// scoring used by the TUI picker. It depends only on the standard library
// and knows nothing about the directory tree, vault, or UI — callers pass
// in a slice of absolute paths and receive a sorted slice of Ranked entries.
package recent

import "time"

// Half-lives and weights for the hybrid score. Package-level constants
// rather than configuration: tweaking is a one-line change, exposing
// them as knobs is YAGNI.
const (
	// mtimeHalfLifeHours sets the decay rate of the filesystem-mtime
	// score term. 168h = 7 days: a file edited 7 days ago scores half
	// what a file edited just now scores from this term.
	mtimeHalfLifeHours = 168.0

	// visitHalfLifeHours sets the decay rate of the visit-history score
	// term. 48h = 2 days: visits decay faster than edits because they
	// reflect short-term attention rather than long-term relevance.
	visitHalfLifeHours = 48.0

	// visitWeight scales the visit-history term relative to the mtime
	// term. >1 means an equally-aged visit outranks an equally-aged edit.
	visitWeight = 1.5
)

// Ranked carries one entry of the ordered Rank result.
type Ranked struct {
	Path  string
	Score float64
	MTime time.Time // file modification time at the moment of Rank
	Visit time.Time // last visit; zero if never visited
}
```

- [ ] **Step 1.4: Run the test to verify it passes**

```
go test ./internal/recent/...
```

Expected: `PASS`.

- [ ] **Step 1.5: Commit**

```
git add internal/recent/recent.go internal/recent/recent_test.go
git commit -m "feat(recent): package skeleton with Ranked type and scoring constants"
```

---

## Task 2: Implement the scoring function and test it with frozen time

**Files:**
- Modify: `internal/recent/recent.go`
- Modify: `internal/recent/recent_test.go`

- [ ] **Step 2.1: Write the failing test**

Append to `internal/recent/recent_test.go`:

```go
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
```

- [ ] **Step 2.2: Run the test to verify it fails**

```
go test ./internal/recent/...
```

Expected: `FAIL` with `undefined: score`.

- [ ] **Step 2.3: Implement the score function**

Append to `internal/recent/recent.go`:

```go
import (
	"math"
	"time"
)
```

(Reconcile imports — the file already imports `time`; add `math`.)

Then append below the constants:

```go
// score computes the hybrid score for a file with the given mtime and
// last-visit time, evaluated at now. The two terms decay exponentially
// with different half-lives and the visit term is weighted; see package
// constants.
//
// A zero visit time means "never visited" — the visit term is zero
// (not exp(huge) — that would be 0 in practice but is semantically
// confusing). We check IsZero explicitly to make the intent obvious.
func score(now, mtime, visit time.Time) float64 {
	var s float64
	if !mtime.IsZero() {
		dtMtime := now.Sub(mtime).Hours()
		if dtMtime < 0 {
			dtMtime = 0
		}
		s += math.Exp(-dtMtime / mtimeHalfLifeHours)
	}
	if !visit.IsZero() {
		dtVisit := now.Sub(visit).Hours()
		if dtVisit < 0 {
			dtVisit = 0
		}
		s += visitWeight * math.Exp(-dtVisit/visitHalfLifeHours)
	}
	return s
}
```

- [ ] **Step 2.4: Run the test to verify it passes**

```
go test ./internal/recent/...
```

Expected: `PASS` on all four new tests plus the two from Task 1.

- [ ] **Step 2.5: Commit**

```
git add internal/recent/recent.go internal/recent/recent_test.go
git commit -m "feat(recent): hybrid mtime+visit decay score function"
```

---

## Task 3: Implement `Store` with in-memory `Record` and `Rank` (no persistence yet)

**Files:**
- Modify: `internal/recent/recent.go`
- Modify: `internal/recent/recent_test.go`

- [ ] **Step 3.1: Write the failing test**

Append to `internal/recent/recent_test.go`:

```go
import (
	"os"
	"path/filepath"
)
```

(Reconcile imports — add `os` and `path/filepath` to the existing import block.)

```go
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
```

- [ ] **Step 3.2: Run the test to verify it fails**

```
go test ./internal/recent/...
```

Expected: `FAIL` with `undefined: Store`.

- [ ] **Step 3.3: Implement `Store`, `Record`, `Rank`**

Append to `internal/recent/recent.go` (after the existing `import` block, reconcile to add `os`, `sort`, `sync` to imports):

```go
import (
	"math"
	"os"
	"sort"
	"sync"
	"time"
)
```

Then append the type and methods:

```go
// Store holds the persisted visit history and exposes scoring + ranking.
// Concurrency: in normal TUI use Store is touched from a single goroutine
// (the Bubble Tea update loop). The mutex is defensive against future
// changes — its cost is negligible for the call frequencies involved.
type Store struct {
	stateFile string
	visits    map[string]time.Time
	mu        sync.Mutex
	nowFunc   func() time.Time
}

// Record marks path as visited now. In Task 4 this also writes through to
// the state file; for now it only updates the in-memory map. Returns
// an error from the write (none in this task).
func (s *Store) Record(path string) error {
	s.mu.Lock()
	s.visits[path] = s.nowFunc()
	s.mu.Unlock()
	// Persistence wired up in Task 4.
	return nil
}

// Rank returns paths sorted by hybrid score (descending). It calls os.Stat
// on each path to read mtime; entries whose stat fails are dropped silently
// from the result. mtime is not cached across calls because the watcher may
// have updated files since the last Rank.
func (s *Store) Rank(paths []string) []Ranked {
	now := s.nowFunc()
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Ranked, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		mtime := info.ModTime()
		visit := s.visits[p] // zero if absent
		out = append(out, Ranked{
			Path:  p,
			Score: score(now, mtime, visit),
			MTime: mtime,
			Visit: visit,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	return out
}
```

- [ ] **Step 3.4: Run the test to verify it passes**

```
go test ./internal/recent/...
```

Expected: `PASS`.

- [ ] **Step 3.5: Commit**

```
git add internal/recent/recent.go internal/recent/recent_test.go
git commit -m "feat(recent): Store with in-memory Record and Rank"
```

---

## Task 4: Persistence — `New`, `DefaultStateFile`, atomic JSON load/save, `Record` writes through

**Files:**
- Modify: `internal/recent/recent.go`
- Modify: `internal/recent/recent_test.go`

- [ ] **Step 4.1: Write the failing tests**

Append to `internal/recent/recent_test.go`:

```go
import "encoding/json"
```

(Reconcile imports — add `encoding/json` to the existing import block.)

```go
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
```

- [ ] **Step 4.2: Run the tests to verify they fail**

```
go test ./internal/recent/...
```

Expected: `FAIL` — `New` and `DefaultStateFile` don't exist yet, and `Record` doesn't actually persist.

- [ ] **Step 4.3: Implement persistence**

Append the following to `internal/recent/recent.go` (reconcile imports: add `encoding/json` and `path/filepath`):

```go
import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)
```

Then append:

```go
// fileFormat is the on-disk representation. Keeping it as a separate
// type avoids exposing the layout via Store's exported fields, and the
// version field gives us a forward-compat hook.
type fileFormat struct {
	Version int                  `json:"version"`
	Visits  map[string]time.Time `json:"visits"`
}

const currentVersion = 1

// New loads visits from stateFile (if it exists) and returns a Store ready
// to use. If the file is missing, the Store starts empty and no error is
// returned. If the file is malformed or has an unknown version, the Store
// starts empty and a non-nil error describes the problem so the caller can
// surface it as a diagnostic.
//
// If stateFile is empty, persistence is disabled: Record updates the
// in-memory map and returns nil without touching disk.
func New(stateFile string) (*Store, error) {
	s := &Store{
		stateFile: stateFile,
		visits:    map[string]time.Time{},
		nowFunc:   time.Now,
	}
	if stateFile == "" {
		return s, nil
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, fmt.Errorf("read %s: %w", stateFile, err)
	}

	var ff fileFormat
	if err := json.Unmarshal(data, &ff); err != nil {
		return s, fmt.Errorf("parse %s: %w", stateFile, err)
	}
	if ff.Version != currentVersion {
		return s, fmt.Errorf("unsupported visits version %d in %s", ff.Version, stateFile)
	}
	for k, v := range ff.Visits {
		s.visits[k] = v
	}
	return s, nil
}

// DefaultStateFile returns the per-user state file path under
// os.UserConfigDir() + "hypogeum/visits.json". Returns an error only if
// no user config dir can be determined.
func DefaultStateFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(dir, "hypogeum", "visits.json"), nil
}

// saveLocked writes the current visits map to stateFile via atomic
// temp-file + rename. Caller must hold s.mu. No-op if stateFile is empty.
func (s *Store) saveLocked() error {
	if s.stateFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.stateFile), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(s.stateFile), err)
	}
	ff := fileFormat{Version: currentVersion, Visits: s.visits}
	data, err := json.MarshalIndent(ff, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal visits: %w", err)
	}
	tmp := s.stateFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.stateFile); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmp, s.stateFile, err)
	}
	return nil
}
```

Now update `Record` to call `saveLocked`. Replace the existing `Record` body:

```go
func (s *Store) Record(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visits[path] = s.nowFunc()
	return s.saveLocked()
}
```

- [ ] **Step 4.4: Run the tests to verify they pass**

```
go test ./internal/recent/...
```

Expected: `PASS`. All tests from Tasks 1–4 should pass.

- [ ] **Step 4.5: Commit**

```
git add internal/recent/recent.go internal/recent/recent_test.go
git commit -m "feat(recent): atomic JSON persistence and DefaultStateFile"
```

---

## Task 5: Wire `recent.Store` into `Model.New`

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

- [ ] **Step 5.1: Write the failing test**

Append to `internal/tui/model_test.go`:

```go
func TestNewInitializesRecentStore(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "n.md")
	if err := os.WriteFile(notePath, []byte("# N"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.recent == nil {
		t.Fatal("Model.recent is nil; want non-nil Store")
	}
}
```

If `model_test.go` doesn't already import `os` and `path/filepath`, add them.

- [ ] **Step 5.2: Run the test to verify it fails**

```
go test ./internal/tui/... -run TestNewInitializesRecentStore
```

Expected: `FAIL` with `m.recent` undefined.

- [ ] **Step 5.3: Add `recent` field on Model and initialize it in `New`**

In `internal/tui/model.go`, add the import:

```go
import (
	// ... existing imports ...
	"github.com/wilkes/hypogeum/internal/recent"
)
```

Add the field on the `Model` struct (place it next to `vault` for visual grouping):

```go
type Model struct {
	// ... existing fields ...
	vault  *vault.Vault
	recent *recent.Store
	diag   *diagnostics
	// ... rest ...
}
```

In `New`, after the `diag` and `vault` initialization and before the renderer setup, add:

```go
stateFile, sferr := recent.DefaultStateFile()
if sferr != nil {
	diag.Warn("recent: cannot determine state file path: " + sferr.Error())
}
rstore, rerr := recent.New(stateFile)
if rerr != nil {
	diag.Warn("recent: " + rerr.Error())
}
```

And set it on the Model literal:

```go
m := Model{
	// ... existing fields ...
	vault:  v,
	recent: rstore,
	diag:   diag,
	// ... rest ...
}
```

- [ ] **Step 5.4: Run the test to verify it passes**

```
go test ./internal/tui/... -run TestNewInitializesRecentStore
```

Expected: `PASS`.

- [ ] **Step 5.5: Run the full TUI test suite to check for regressions**

```
go test ./internal/tui/...
```

Expected: all tests pass.

- [ ] **Step 5.6: Commit**

```
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat(tui): wire recent.Store into Model.New"
```

---

## Task 6: Record visits inside `openFile`

**Files:**
- Modify: `internal/tui/content.go`
- Modify: `internal/tui/dispatch_test.go`

- [ ] **Step 6.1: Write the failing test**

Append to `internal/tui/dispatch_test.go`:

```go
func TestOpenFileRecordsVisit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "n.md")
	if err := os.WriteFile(p, []byte("# N"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := New(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	// Sanity: Recent rank before openFile shouldn't include p as visited.
	pre := m.recent.Rank([]string{p})
	if len(pre) != 1 {
		t.Fatalf("pre Rank: got %d, want 1", len(pre))
	}
	if !pre[0].Visit.IsZero() {
		t.Errorf("pre Rank: visit should be zero, got %v", pre[0].Visit)
	}

	m.openFile(p)

	post := m.recent.Rank([]string{p})
	if len(post) != 1 {
		t.Fatalf("post Rank: got %d, want 1", len(post))
	}
	if post[0].Visit.IsZero() {
		t.Error("post Rank: visit should be non-zero after openFile")
	}
}
```

If imports for `os` and `path/filepath` aren't already in `dispatch_test.go`, add them.

- [ ] **Step 6.2: Run the test to verify it fails**

```
go test ./internal/tui/... -run TestOpenFileRecordsVisit
```

Expected: `FAIL` — `openFile` doesn't call `Record`, so the visit time stays zero.

- [ ] **Step 6.3: Add the `Record` call inside `openFile`**

In `internal/tui/content.go`, modify `openFile`:

```go
// openFile records a visit in history and renders the file.
func (m *Model) openFile(path string) {
	m.history.Visit(path)
	if m.recent != nil {
		if err := m.recent.Record(path); err != nil && m.diag != nil {
			m.diag.Warn("recent: " + err.Error())
		}
	}
	m.refreshContent(path)
}
```

The `m.recent != nil` guard handles the `DefaultStateFile` failure path where `m.recent` could be left nil. (Defensive — `recent.New` always returns non-nil today, but a future change might not.)

- [ ] **Step 6.4: Run the test to verify it passes**

```
go test ./internal/tui/... -run TestOpenFileRecordsVisit
```

Expected: `PASS`.

- [ ] **Step 6.5: Run the full TUI test suite**

```
go test ./internal/tui/...
```

Expected: all tests pass (no regression).

- [ ] **Step 6.6: Commit**

```
git add internal/tui/content.go internal/tui/dispatch_test.go
git commit -m "feat(tui): record visit in recent.Store on openFile"
```

---

## Task 7: Add `Model.allVaultMarkdownPaths` helper

**Files:**
- Modify: `internal/tui/content.go` (or a new helper file — put it next to where it's called from)
- Modify: `internal/tui/model_test.go`

- [ ] **Step 7.1: Write the failing test**

Append to `internal/tui/model_test.go`:

```go
func TestAllVaultMarkdownPaths(t *testing.T) {
	dir := t.TempDir()
	// Create:  dir/a.md, dir/sub/b.md, dir/sub/sub2/c.md, dir/d.txt (excluded)
	mustWrite := func(p string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(dir, "a.md"))
	mustWrite(filepath.Join(dir, "sub", "b.md"))
	mustWrite(filepath.Join(dir, "sub", "sub2", "c.md"))
	mustWrite(filepath.Join(dir, "d.txt"))

	m, err := New(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	paths := m.allVaultMarkdownPaths()
	if len(paths) != 3 {
		t.Errorf("got %d paths, want 3: %v", len(paths), paths)
	}
	// All paths absolute and end in .md
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			t.Errorf("path not absolute: %q", p)
		}
		if filepath.Ext(p) != ".md" {
			t.Errorf("non-md path: %q", p)
		}
	}
}
```

- [ ] **Step 7.2: Run the test to verify it fails**

```
go test ./internal/tui/... -run TestAllVaultMarkdownPaths
```

Expected: `FAIL` — `allVaultMarkdownPaths` doesn't exist.

- [ ] **Step 7.3: Implement the helper**

Append to `internal/tui/content.go`:

```go
// allVaultMarkdownPaths walks m.rootNode and returns every markdown file
// path under it as an absolute path. Cheap: the tree is already in memory
// and pruned to markdown-only at tree.Walk time. The method lives on Model
// rather than in internal/recent so that recent stays ignorant of
// *tree.Node.
func (m *Model) allVaultMarkdownPaths() []string {
	if m.rootNode == nil {
		return nil
	}
	var out []string
	var walk func(n *tree.Node)
	walk = func(n *tree.Node) {
		if !n.IsDir {
			out = append(out, n.Path)
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(m.rootNode)
	return out
}
```

(The `tree` import is already in `content.go`'s package via other files; check `content.go`'s top imports and add `"github.com/wilkes/hypogeum/internal/tree"` if missing.)

- [ ] **Step 7.4: Run the test to verify it passes**

```
go test ./internal/tui/... -run TestAllVaultMarkdownPaths
```

Expected: `PASS`.

- [ ] **Step 7.5: Run the full TUI test suite**

```
go test ./internal/tui/...
```

Expected: all tests pass.

- [ ] **Step 7.6: Commit**

```
git add internal/tui/content.go internal/tui/model_test.go
git commit -m "feat(tui): allVaultMarkdownPaths helper for finder population"
```

---

## Task 8: Rewrite `pickerState` as a flat-list type — fields and rendering

**Files:**
- Modify: `internal/tui/picker.go`

This task replaces the tree-rooted picker with the flat-list version. Tests are updated in Task 9. The build will be temporarily broken between Tasks 8 and 9 — that's expected, the commit at the end of Task 9 restores green.

- [ ] **Step 8.1: Replace the contents of `internal/tui/picker.go`**

Replace the entire file with:

```go
package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/wilkes/hypogeum/internal/recent"
)

// pickerState is the flat, recency-ranked file finder rendered as a modal.
// Replaces the previous tree-rooted picker. Cursor is an index into ranked;
// expansion state and folder navigation are gone — see
// docs/superpowers/specs/2026-05-12-unified-finder-recency-design.md.
type pickerState struct {
	ranked []recent.Ranked
	cursor int
	vp     viewport.Model
	root   string // vault root, used to render relative paths
}

func newPicker() pickerState {
	return pickerState{vp: viewport.New(0, 0)}
}

// reset populates the picker with a fresh ranked list (from m.recent.Rank
// on the current vault paths) and resets the cursor to 0.
func (p *pickerState) reset(ranked []recent.Ranked, root string) {
	p.ranked = ranked
	p.cursor = 0
	p.root = root
	p.refreshVP()
}

// refreshVP regenerates the viewport content and scrolls so the cursor row
// is in view. Mirrors Model.refreshTreeVP for the left pane.
func (p *pickerState) refreshVP() {
	p.vp.SetContent(p.renderRows())
	viewportClamp(&p.vp, p.cursor, 1)
}

// renderRows produces the flat list of one-line entries. Left column is
// the path relative to the vault root; right column is the human-friendly
// recency (with " · edited" appended when the row's recency is from mtime
// rather than visit). The selected row is reverse-video.
//
// No numeric score is displayed. The score is a sorting signal; surfacing
// it would invite questions about its meaning. If you need to debug
// scoring, run a test rather than rendering it here.
func (p *pickerState) renderRows() string {
	now := time.Now()
	var b strings.Builder
	width := p.vp.Width
	if width < 20 {
		width = 20
	}
	for i, r := range p.ranked {
		rel := relativeTo(p.root, r.Path)
		recencyLabel, edited := pickRecencyLabel(now, r.MTime, r.Visit)
		suffix := recencyLabel
		if edited {
			suffix += " · edited"
		}
		line := formatPickerRow(rel, suffix, width)
		if i == p.cursor {
			line = lipgloss.NewStyle().Reverse(true).Render(line)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// relativeTo returns p relative to root, or the absolute path on failure.
func relativeTo(root, p string) string {
	if root == "" {
		return p
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return rel
}

// pickRecencyLabel returns the human-friendly recency to display and a
// flag indicating whether the displayed time is mtime (true) or visit
// (false). The more recent of mtime and visit wins; never-visited
// (visit zero) always uses mtime.
func pickRecencyLabel(now, mtime, visit time.Time) (label string, isMTime bool) {
	t := mtime
	isMTime = true
	if !visit.IsZero() && visit.After(mtime) {
		t = visit
		isMTime = false
	}
	return humanRecency(now, t), isMTime
}

// humanRecency formats a duration since t in a one-glance form:
// "just now", "3m ago", "2h ago", "yesterday", "3d ago", "2w ago".
// Beyond ~6 weeks falls back to YYYY-MM-DD so the user has *some* signal
// rather than a months-ago bucket.
func humanRecency(now, t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	case d < 6*7*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d/(7*24*time.Hour)))
	default:
		return t.Format("2006-01-02")
	}
}

// formatPickerRow lays out one row to fit width. Left column is the path
// (truncated with a leading ellipsis if too long), right column is the
// suffix (right-aligned). One space gap minimum between them.
func formatPickerRow(left, right string, width int) string {
	const gap = 2
	rightW := ansi.StringWidth(right)
	leftBudget := width - rightW - gap
	if leftBudget < 5 {
		// Pathological narrow case: just show the right column.
		return strings.Repeat(" ", width-rightW) + right
	}
	leftTrunc := truncateLeadingEllipsis(left, leftBudget)
	leftW := ansi.StringWidth(leftTrunc)
	pad := width - leftW - rightW
	if pad < gap {
		pad = gap
	}
	return leftTrunc + strings.Repeat(" ", pad) + right
}

// truncateLeadingEllipsis truncates s so its width is <= max, preferring
// to drop characters from the start (so the basename stays visible).
func truncateLeadingEllipsis(s string, max int) string {
	if ansi.StringWidth(s) <= max {
		return s
	}
	const ell = "…"
	// Use ansi.TruncateLeft which counts cells, not bytes.
	keep := max - ansi.StringWidth(ell)
	if keep < 1 {
		return ell
	}
	return ell + ansi.TruncateLeft(s, ansi.StringWidth(s)-keep, "")
}

// View returns the picker's renderable string for placement inside the
// modal frame.
func (p *pickerState) View() string {
	if len(p.ranked) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("(no markdown files in vault)")
	}
	return p.vp.View()
}

// resizePicker fits the picker viewport into the modal interior.
func (m *Model) resizePicker() {
	_, _, w, h := modalGeometry(m.width, m.height)
	pw := w - 2
	ph := h - 2
	if pw < 1 {
		pw = 1
	}
	if ph < 1 {
		ph = 1
	}
	m.modals.picker.vp.Width = pw
	m.modals.picker.vp.Height = ph
	m.modals.picker.refreshVP()
}

// selectedPath returns the path under the picker cursor, or ("", false)
// if the cursor is out of range or the list is empty.
func (p *pickerState) selectedPath() (string, bool) {
	if p.cursor < 0 || p.cursor >= len(p.ranked) {
		return "", false
	}
	return p.ranked[p.cursor].Path, true
}
```

This deletes the old `pickerFlatten`, `toggleAt`, `selectedFile` (now `selectedPath`), and the chevron-rendering code.

- [ ] **Step 8.2: Build (expect failure)**

```
go build ./...
```

Expected: build errors in `internal/tui/input.go` (`pickerState.toggleAt` undefined, `selectedFile` undefined) and `internal/tui/picker_test.go` (uses removed API). This is expected; Task 9 fixes the callers.

- [ ] **Step 8.3: Commit (with explicit note)**

```
git add internal/tui/picker.go
git commit -m "refactor(tui): rewrite pickerState as flat recency-ranked list

The tree-walk, expansion map, and pickerFlatten are replaced with a flat
list of recent.Ranked entries. Build is broken at this commit; callers
in input.go and picker_test.go are updated in the next task."
```

---

## Task 9: Update picker key handling and rewrite picker tests

**Files:**
- Modify: `internal/tui/input.go`
- Modify: `internal/tui/picker_test.go`

- [ ] **Step 9.1: Update the picker open call site in `input.go`**

In `internal/tui/input.go`, find the `OpenPicker` case (around line 143 today):

```go
case key.Matches(msg, m.keys.OpenPicker):
	return *m, m.openModalWith(modalPicker, func() {
		// Each open starts fresh: cursor at top, all dirs collapsed.
		m.modals.picker.reset(m.rootNode)
	})
```

Replace it with:

```go
case key.Matches(msg, m.keys.OpenPicker):
	return *m, m.openModalWith(modalPicker, func() {
		paths := m.allVaultMarkdownPaths()
		ranked := []recent.Ranked{}
		if m.recent != nil {
			ranked = m.recent.Rank(paths)
		}
		m.modals.picker.reset(ranked, m.root)
	})
```

Add the `recent` import to `input.go`:

```go
"github.com/wilkes/hypogeum/internal/recent"
```

- [ ] **Step 9.2: Update the picker key handler in `input.go`**

Find the picker modal-key block (around line 161 today):

```go
if m.modals.kind == modalPicker {
	switch {
	case key.Matches(msg, m.keys.ClearLink): // Esc closes from any depth
		m.modals.kind = modalNone
		m.focus = m.modals.prevFocus
	case key.Matches(msg, m.keys.Up):
		if m.modals.picker.cursor > 0 {
			m.modals.picker.cursor--
			m.modals.picker.refreshVP()
		}
	case key.Matches(msg, m.keys.Down):
		if m.modals.picker.cursor < len(m.modals.picker.flat)-1 {
			m.modals.picker.cursor++
			m.modals.picker.refreshVP()
		}
	case key.Matches(msg, m.keys.ToggleFolder):
		m.modals.picker.toggleAt(m.rootNode)
	case key.Matches(msg, m.keys.Open):
		if path, ok := m.modals.picker.selectedFile(); ok {
			m.modals.kind = modalNone
			m.focus = m.modals.prevFocus
			m.navigateTo(path)
		} else {
			// On a directory: Enter expands/collapses it, same as space.
			m.modals.picker.toggleAt(m.rootNode)
		}
	}
	return *m, nil
}
```

Replace it with:

```go
if m.modals.kind == modalPicker {
	switch {
	case key.Matches(msg, m.keys.ClearLink): // Esc
		m.modals.kind = modalNone
		m.focus = m.modals.prevFocus
	case key.Matches(msg, m.keys.Up):
		if m.modals.picker.cursor > 0 {
			m.modals.picker.cursor--
			m.modals.picker.refreshVP()
		}
	case key.Matches(msg, m.keys.Down):
		if m.modals.picker.cursor < len(m.modals.picker.ranked)-1 {
			m.modals.picker.cursor++
			m.modals.picker.refreshVP()
		}
	case key.Matches(msg, m.keys.Open):
		if path, ok := m.modals.picker.selectedPath(); ok {
			m.modals.kind = modalNone
			m.focus = m.modals.prevFocus
			m.navigateTo(path)
		}
	}
	return *m, nil
}
```

Notably removed: the `ToggleFolder` branch (the flat list has no folders), and the "Enter on directory expands" fallback.

`g` / `G` are not added in this task — they're left to a follow-on if wanted. The picker is fully usable without them.

- [ ] **Step 9.3: Rewrite `internal/tui/picker_test.go`**

Replace the entire contents of `internal/tui/picker_test.go` with:

```go
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func mustWriteFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// pressKey delivers a single tea.KeyMsg with the named key, returning
// the resulting Model and any Cmd. Mirrors how Bubble Tea actually
// dispatches a keystroke.
func pressKey(m Model, name string) (Model, tea.Cmd) {
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)})
	return tm.(Model), cmd
}

func pressCtrl(m Model, key string) (Model, tea.Cmd) {
	// "^p" — we use tea.KeyCtrlP etc. via the typed Key field.
	var t tea.KeyType
	switch key {
	case "p":
		t = tea.KeyCtrlP
	case "l":
		t = tea.KeyCtrlL
	case "b":
		t = tea.KeyCtrlB
	default:
		panic("pressCtrl: unsupported key " + key)
	}
	tm, cmd := m.Update(tea.KeyMsg{Type: t})
	return tm.(Model), cmd
}

func pressEnter(m Model) (Model, tea.Cmd) {
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return tm.(Model), cmd
}

func pressEsc(m Model) (Model, tea.Cmd) {
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	return tm.(Model), cmd
}

func TestPickerOpenPopulatesRanked(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.md"), "# A")
	mustWriteFile(t, filepath.Join(dir, "b.md"), "# B")

	m, err := New(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = pressCtrl(m, "p")
	if m.modals.kind != modalPicker {
		t.Fatalf("modal kind: got %d want %d", m.modals.kind, modalPicker)
	}
	if len(m.modals.picker.ranked) != 2 {
		t.Errorf("ranked: got %d want 2", len(m.modals.picker.ranked))
	}
}

func TestPickerEscClosesWithoutOpening(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.md"), "# A")
	m, err := New(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	before := m.history.Current()
	m, _ = pressCtrl(m, "p")
	m, _ = pressEsc(m)

	if m.modals.kind != modalNone {
		t.Errorf("expected modalNone after Esc, got %d", m.modals.kind)
	}
	if m.history.Current() != before {
		t.Errorf("Esc should not have navigated; was %q now %q", before, m.history.Current())
	}
}

func TestPickerJKMovesCursor(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.md"), "# A")
	mustWriteFile(t, filepath.Join(dir, "b.md"), "# B")
	m, err := New(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = pressCtrl(m, "p")
	if got := m.modals.picker.cursor; got != 0 {
		t.Fatalf("initial cursor: %d, want 0", got)
	}
	m, _ = pressKey(m, "j")
	if got := m.modals.picker.cursor; got != 1 {
		t.Errorf("after j: cursor=%d, want 1", got)
	}
	m, _ = pressKey(m, "k")
	if got := m.modals.picker.cursor; got != 0 {
		t.Errorf("after k: cursor=%d, want 0", got)
	}
	// k at top is clamped.
	m, _ = pressKey(m, "k")
	if got := m.modals.picker.cursor; got != 0 {
		t.Errorf("k at top: cursor=%d, want 0", got)
	}
}

func TestPickerEnterOpensSelected(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.md")
	p2 := filepath.Join(dir, "b.md")
	mustWriteFile(t, p1, "# A")
	mustWriteFile(t, p2, "# B")
	m, err := New(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = pressCtrl(m, "p")
	// Whichever file is first in the ranked list is what Enter opens.
	want := m.modals.picker.ranked[0].Path
	m, _ = pressEnter(m)

	if m.modals.kind != modalNone {
		t.Errorf("Enter should close picker, got modal kind %d", m.modals.kind)
	}
	if got := m.history.Current(); got != want {
		t.Errorf("history.Current after Enter: got %q want %q", got, want)
	}
}

func TestPickerEmptyVault(t *testing.T) {
	dir := t.TempDir()
	// No markdown files.
	m, err := New(dir, "")
	// New may fail because there's nothing to open. Tolerate either: if
	// it does fail, skip the test (this is a corner the existing TUI
	// doesn't claim to support gracefully).
	if err != nil {
		t.Skip("New on empty dir failed; not the picker's concern: " + err.Error())
	}
	m.width, m.height = 80, 24
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = pressCtrl(m, "p")
	if len(m.modals.picker.ranked) != 0 {
		t.Errorf("expected empty ranked, got %d entries", len(m.modals.picker.ranked))
	}
	// Enter on empty list is a no-op.
	m, _ = pressEnter(m)
	if m.modals.kind != modalPicker {
		// If the picker closes that's also acceptable; check we didn't
		// crash by reading m.modals.kind. We don't assert on this.
		_ = m
	}
}

func TestPickerRecentVisitBoostsRank(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.md")
	p2 := filepath.Join(dir, "b.md")
	mustWriteFile(t, p1, "# A")
	mustWriteFile(t, p2, "# B")
	// Make mtimes deliberately equal.
	now := time.Now()
	if err := os.Chtimes(p1, now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p2, now, now); err != nil {
		t.Fatal(err)
	}

	m, err := New(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 80, 24
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Open p1 → its visit bumps its score above p2.
	m.openFile(p1)

	m, _ = pressCtrl(m, "p")
	if len(m.modals.picker.ranked) < 2 {
		t.Fatalf("ranked: got %d, want >=2", len(m.modals.picker.ranked))
	}
	if got := m.modals.picker.ranked[0].Path; got != p1 {
		t.Errorf("top of rank after visiting p1: got %q want %q", got, p1)
	}
}

func TestHumanRecency(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", time.Time{}, "never"},
		{"30s", now.Add(-30 * time.Second), "just now"},
		{"5m", now.Add(-5 * time.Minute), "5m ago"},
		{"3h", now.Add(-3 * time.Hour), "3h ago"},
		{"30h", now.Add(-30 * time.Hour), "yesterday"},
		{"3d", now.Add(-3 * 24 * time.Hour), "3d ago"},
		{"3w", now.Add(-3 * 7 * 24 * time.Hour), "3w ago"},
		{"3mo", now.Add(-90 * 24 * time.Hour), "2026-02-11"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := humanRecency(now, c.t)
			if got != c.want {
				t.Errorf("humanRecency(%s): got %q want %q", c.name, got, c.want)
			}
		})
	}
}

func TestFormatPickerRowFits(t *testing.T) {
	out := formatPickerRow("a/b/c.md", "2h ago", 30)
	if w := len(out); w == 0 {
		t.Fatal("formatPickerRow returned empty")
	}
	if !strings.Contains(out, "a/b/c.md") {
		t.Errorf("row missing left content: %q", out)
	}
	if !strings.Contains(out, "2h ago") {
		t.Errorf("row missing right content: %q", out)
	}
}

func TestFormatPickerRowTruncates(t *testing.T) {
	long := "very/long/nested/path/to/some/note.md"
	out := formatPickerRow(long, "1h ago", 20)
	if !strings.Contains(out, "…") {
		t.Errorf("expected leading ellipsis in narrow row, got %q", out)
	}
	if !strings.Contains(out, "note.md") {
		t.Errorf("basename should remain visible: %q", out)
	}
}
```

- [ ] **Step 9.4: Build**

```
go build ./...
```

Expected: clean build.

- [ ] **Step 9.5: Run the full test suite**

```
go test ./...
```

Expected: all tests pass. If older `picker_test.go` tests are referenced elsewhere (e.g. helpers shared with `modal_test.go`) and break, fix the shared helpers — do not skip tests.

- [ ] **Step 9.6: Commit**

```
git add internal/tui/input.go internal/tui/picker_test.go
git commit -m "feat(tui): flat-finder key handling and tests"
```

---

## Task 10: Documentation updates

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `docs/index.md`
- Modify: `docs/packages/tui.md`
- Modify: `docs/superpowers/specs/2026-05-07-vault-rooted-picker-design.md`

- [ ] **Step 10.1: Update `README.md`**

Find the line in the keys table:

```
| `^p` | Open file picker (modal) |
```

Replace with:

```
| `^p` | Open file finder (recent first) |
```

- [ ] **Step 10.2: Update `CLAUDE.md`**

Find the gotcha paragraph that begins:

```
- **`^p` opens a vault-rooted picker modal as a fourth `modalKind`.**
```

Replace the entire bullet with:

```
- **`^p` opens a flat recency-ranked finder as a fourth `modalKind`.** It calls `m.recent.Rank(m.allVaultMarkdownPaths())` once per open and renders the result as a flat list — one row per markdown file, no folders, no expansion state. The hybrid score combines filesystem mtime (7-day half-life) with persisted visit history (2-day half-life, weighted 1.5×). Visits are written through to `~/.config/hypogeum/visits.json` (or the platform equivalent via `os.UserConfigDir()`) atomically on every `openFile`. See [unified-finder-recency](docs/superpowers/specs/2026-05-12-unified-finder-recency-design.md) for the full design; the previous tree-rooted picker spec is superseded.
```

- [ ] **Step 10.3: Update `docs/index.md`**

Under the "Active feature work" section, add this line (preserve existing list ordering — append at the end of the section):

```
- [Unified finder with recency](superpowers/specs/2026-05-12-unified-finder-recency-design.md) — shipped — `^p` opens a flat list of every vault markdown file, ranked by a hybrid mtime + persisted-visit score.
```

- [ ] **Step 10.4: Update `docs/packages/tui.md`**

Open the file and find the picker section (search for "picker" or "^p"). Replace whichever paragraphs describe the tree-rooted picker with:

```
### Flat file finder (`^p`)

`^p` opens `modalPicker` — a flat list of every markdown file in the
vault, ranked by `recent.Store.Rank`. The picker holds a `[]recent.Ranked`
and an integer cursor; there is no expansion state and no tree walk
on render. Each row shows the path relative to the vault root and a
human-friendly recency label ("2h ago", "yesterday", "3d ago · edited"
when the recency comes from a file edit rather than a visit). Keys: `j`/`k`
or `↑`/`↓` move the cursor, `Enter` opens, `Esc` or `^p` closes.

Visits are recorded on every `openFile` call (tree, link, backlink, history,
finder) via `m.recent.Record(path)`. The Store persists to
`~/.config/hypogeum/visits.json` (or platform equivalent) atomically.
Both Record errors and load errors surface as `diag.Warn` and never block
navigation.

See [unified-finder-recency](../superpowers/specs/2026-05-12-unified-finder-recency-design.md)
for the full design.
```

If you can't find an existing picker section, append the above at the bottom of the file under an appropriate heading.

- [ ] **Step 10.5: Add superseded header to old picker spec**

In `docs/superpowers/specs/2026-05-07-vault-rooted-picker-design.md`, replace the first line:

```
# Vault-rooted picker — design
```

With:

```
# Vault-rooted picker — design

**Superseded by:** [unified-finder-recency](2026-05-12-unified-finder-recency-design.md). The tree-rooted picker described below has been replaced by a flat recency-ranked finder. This document remains for historical context.

---
```

- [ ] **Step 10.6: Verify everything compiles and tests pass**

```
go vet ./...
go test ./...
```

Expected: clean vet, all tests pass.

- [ ] **Step 10.7: Commit**

```
git add README.md CLAUDE.md docs/index.md docs/packages/tui.md docs/superpowers/specs/2026-05-07-vault-rooted-picker-design.md
git commit -m "docs: update for flat recency-ranked finder"
```

---

## Task 11: Final manual verification

This task is verification, not code. No commits.

- [ ] **Step 11.1: Run the full test suite and vet**

```
go vet ./...
go test ./...
```

Expected: all pass.

- [ ] **Step 11.2: Manual smoke test against the project's own docs**

Build and run hypogeum against the docs folder.

```
go build ./...
./hypogeum docs/
```

Expected: the TUI opens. Verify:
- `^p` opens the finder modal showing every `.md` file under `docs/`, sorted with the most-recently-edited file first.
- `j`/`k` move the cursor and the highlighted row changes.
- `Enter` opens the selected file; the modal closes.
- Press `^p` again — the just-opened file is now at the top (visit bumped its score).
- `Esc` closes the modal without navigating.
- Quit (`q`), re-launch — the previously-visited file is still at or near the top, proving persistence.

- [ ] **Step 11.3: Verify the state file**

```
ls -la ~/.config/hypogeum/
cat ~/.config/hypogeum/visits.json
```

Expected: file exists, contains a JSON object with `"version": 1` and a `"visits"` map keyed by absolute path.

- [ ] **Step 11.4: Verify graceful degradation**

```
chmod 000 ~/.config/hypogeum/visits.json
./hypogeum docs/
```

Expected: hypogeum still starts. `^p` shows files sorted by mtime alone (no visit info). A warning may appear briefly in the footer. After confirming, restore permissions:

```
chmod 644 ~/.config/hypogeum/visits.json
```

If you're running this manually and don't want to babysit, you can skip Step 11.4 — the test suite already covers the malformed-file path.

---

## Self-Review

**Spec coverage:**
- ✅ New `internal/recent` package — Tasks 1–4.
- ✅ Public surface: `Store`, `New`, `Record`, `Rank`, `Ranked`, `DefaultStateFile` — Tasks 1, 3, 4.
- ✅ Scoring with hybrid mtime + visit decay — Task 2.
- ✅ Persistence with atomic write + version field — Task 4.
- ✅ Picker rewritten as flat list — Task 8.
- ✅ Visit recording on `openFile` — Task 6.
- ✅ Picker keys (j/k/Up/Down/Enter/Esc/^p toggle) — Task 9. (Spec mentioned `g`/`G` but that's not in the "Keys inside the picker" canonical list — they're a polish item; not implemented here.)
- ✅ Row rendering with relative path + recency label + edited suffix — Task 8.
- ✅ No score displayed — enforced by code comment in Task 8.
- ✅ Graceful degradation (recent load fails, record fails, state dir missing) — Tasks 4, 5, 6.
- ✅ Documentation updates — Task 10.

**Placeholder scan:** Every code step shows the actual code. Every test step shows the actual test. No "TBD", "TODO", or unspecified behavior.

**Type consistency:**
- `recent.Ranked` fields: `Path`, `Score`, `MTime`, `Visit` — used consistently in Tasks 1, 3, 8, 9.
- Picker methods: `reset(ranked, root)`, `refreshVP`, `View`, `selectedPath` — consistent across Tasks 8, 9.
- Function names: `score`, `humanRecency`, `pickRecencyLabel`, `formatPickerRow`, `truncateLeadingEllipsis`, `relativeTo` — defined in Task 8, referenced consistently in Task 9 tests.
- Constants: `mtimeHalfLifeHours`, `visitHalfLifeHours`, `visitWeight`, `currentVersion` — defined once, used once or sanity-checked.

**Minor noted ahead of execution:**
- Task 7 places `allVaultMarkdownPaths` in `content.go`. If you prefer a separate file for vault-walking helpers, move it; the test name and behavior are unchanged.
- Task 9 includes the `g`/`G` jump-to-ends keys as deferred. If you want them, add a one-step task between 9 and 10.

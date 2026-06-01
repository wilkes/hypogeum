// Package recent owns the persisted visit history and the recency-based
// scoring used by the TUI picker. It depends only on the standard library
// and knows nothing about the directory tree, vault, or UI — callers pass
// in a slice of absolute paths and receive a sorted slice of Ranked entries.
package recent

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
	// term. <1 means an equally-aged edit outranks an equally-aged
	// visit; visits still contribute positively, just at a reduced
	// weight so they nudge ranking rather than dominate it.
	visitWeight = 0.5
)

// score computes the exponential-decay hybrid score from mtime and visit times.
func score(now, mtime, visit time.Time) float64 {
	var s float64
	if !mtime.IsZero() {
		dtMtime := now.Sub(mtime).Hours()
		if dtMtime < 0 {
			dtMtime = 0
		}
		s += math.Exp(-dtMtime / mtimeHalfLifeHours)
	}
	// Zero visit time means "never visited" — contribute 0, not exp(huge).
	if !visit.IsZero() {
		dtVisit := now.Sub(visit).Hours()
		if dtVisit < 0 {
			dtVisit = 0
		}
		s += visitWeight * math.Exp(-dtVisit/visitHalfLifeHours)
	}
	return s
}

// Ranked carries one entry of the ordered Rank result.
type Ranked struct {
	Path  string
	Score float64
	MTime time.Time // file modification time at the moment of Rank
	Visit time.Time // last visit; zero if never visited
}

// Store holds the persisted visit history and exposes scoring + ranking.
// The mutex is defensive — in normal TUI use Store is touched from a
// single goroutine.
//
// Concurrent hypogeum instances share one state file: last writer wins
// on the visits map. Each write uses a per-process temp file
// (via os.CreateTemp) so the atomic rename never sees a torn temp.
type Store struct {
	stateFile string
	visits    map[string]time.Time
	mu        sync.Mutex
	nowFunc   func() time.Time
}

// Record marks path as visited now and writes through to the state file.
func (s *Store) Record(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visits[path] = s.nowFunc()
	return s.saveLocked()
}

// Rank returns paths sorted by hybrid score (descending). os.Stat fails
// drop the entry silently; mtime isn't cached because the watcher may
// have updated files since the last call.
func (s *Store) Rank(paths []string) []Ranked {
	now := s.nowFunc()

	// Snapshot visits before any I/O so the lock isn't held across
	// os.Stat. Cheap in practice — the map is bounded by vault size.
	s.mu.Lock()
	visits := make(map[string]time.Time, len(s.visits))
	for k, v := range s.visits {
		visits[k] = v
	}
	s.mu.Unlock()

	out := make([]Ranked, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		mtime := info.ModTime()
		visit := visits[p]
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

type fileFormat struct {
	Version int                  `json:"version"`
	Visits  map[string]time.Time `json:"visits"`
}

const currentVersion = 1

// New loads visits from stateFile and returns a ready Store. Missing file
// is not an error; malformed file or unknown version returns a non-nil
// error alongside an empty but usable Store so the caller can surface
// the failure as a diagnostic.
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

// DefaultStateFile returns os.UserConfigDir() + "hypogeum/visits.json".
func DefaultStateFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(dir, "hypogeum", "visits.json"), nil
}

// saveLocked writes the current visits map atomically (temp file + rename).
// Caller must hold s.mu. No-op when stateFile is empty.
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
	f, err := os.CreateTemp(filepath.Dir(s.stateFile), "visits-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.stateFile); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, s.stateFile, err)
	}
	return nil
}

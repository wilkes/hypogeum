// Package recent owns the persisted visit history and the two independent
// recency orderings hypogeum exposes: edit-recency (filesystem mtime, used by
// the file finder and search re-rank) and visit-recency (last-opened time,
// used by the "recently opened" modal and the CLI recent verb). It depends
// only on the standard library and knows nothing about the directory tree,
// vault, or UI — callers pass in a slice of absolute paths and receive a
// sorted slice of Ranked entries.
//
// The two signals are deliberately *not* blended. Each ordering answers one
// question with a plain descending sort on a single key:
//   - RankByMTime  → "most recently edited first" (stateless, os.Stat-based).
//   - RankByVisit  → "most recently opened first" (visited-only, from the
//     persisted visit map).
package recent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Ranked carries one entry of an ordered ranking result. MTime is populated
// by RankByMTime; Visit is populated by RankByVisit. Each ordering sorts on
// its own field; there is no combined numeric score.
type Ranked struct {
	Path  string
	MTime time.Time // file modification time (RankByMTime); zero otherwise
	Visit time.Time // last visit (RankByVisit); zero if never visited
}

// RankByMTime returns paths sorted by filesystem mtime, newest first. It is
// stateless — no Store needed, because mtime lives on the filesystem. Paths
// that fail os.Stat (missing/unreadable) are dropped silently. mtime isn't
// cached because the watcher may have updated files since the last call.
//
// This is the edit-recency ordering used by the file finder (^p/o) and the
// search-result re-rank: "jump to what I'm working on" is almost always the
// most recently edited file.
func RankByMTime(paths []string) []Ranked {
	out := make([]Ranked, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		out = append(out, Ranked{Path: p, MTime: info.ModTime()})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].MTime.After(out[j].MTime)
	})
	return out
}

// RankPathsByMTime is RankByMTime reduced to just the ordered path slice.
// It exists so callers that only need the ordering (the search re-rank, the
// non-interactive query mode) don't each hand-roll the Rank → .Path map.
// Same drop-missing-files semantics as RankByMTime.
func RankPathsByMTime(paths []string) []string {
	ranked := RankByMTime(paths)
	out := make([]string, len(ranked))
	for i, r := range ranked {
		out[i] = r.Path
	}
	return out
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

// RankByVisit returns the subset of paths that have a recorded visit, sorted
// by visit time descending (most recently opened first). Paths never opened
// in hypogeum are excluded entirely — this is the "recently opened" list, not
// a recency score over the whole vault, so an unvisited file simply doesn't
// appear.
//
// It does not os.Stat the paths: visit ordering is purely about the recorded
// timestamp, so a file that was visited and later deleted still appears
// (callers may filter on existence if they care). This keeps visit-recency
// independent of the filesystem.
//
// This is the visit-recency ordering used by the "recently opened" modal (r)
// and the CLI recent verb.
func (s *Store) RankByVisit(paths []string) []Ranked {
	// Snapshot visits so the lock isn't held across the sort.
	s.mu.Lock()
	visits := make(map[string]time.Time, len(s.visits))
	for k, v := range s.visits {
		visits[k] = v
	}
	s.mu.Unlock()

	out := make([]Ranked, 0, len(paths))
	for _, p := range paths {
		visit, ok := visits[p]
		if !ok || visit.IsZero() {
			continue
		}
		out = append(out, Ranked{Path: p, Visit: visit})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Visit.After(out[j].Visit)
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

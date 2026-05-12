// Package recent owns the persisted visit history and the recency-based
// scoring used by the TUI picker. It depends only on the standard library
// and knows nothing about the directory tree, vault, or UI — callers pass
// in a slice of absolute paths and receive a sorted slice of Ranked entries.
package recent

import (
	"math"
	"os"
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
	// term. >1 means an equally-aged visit outranks an equally-aged edit.
	visitWeight = 1.5
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
type Store struct {
	stateFile string
	visits    map[string]time.Time
	mu        sync.Mutex
	nowFunc   func() time.Time
}

// Record marks path as visited now. Persistence wired up in Task 4.
func (s *Store) Record(path string) error {
	s.mu.Lock()
	s.visits[path] = s.nowFunc()
	s.mu.Unlock()
	return nil
}

// Rank returns paths sorted by hybrid score (descending). os.Stat fails
// drop the entry silently; we don't cache mtime because the watcher may
// have updated files since the last call.
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
		visit := s.visits[p]
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

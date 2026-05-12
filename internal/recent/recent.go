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

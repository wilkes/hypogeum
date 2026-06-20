// Package query is the non-interactive backend for hypogeum's scripting
// verbs. It orchestrates the pure tree/search/vault/recent packages and
// returns JSON-tagged result structs. It has no TUI dependencies.
package query

import (
	"context"
	"strings"
	"time"

	"github.com/wilkes/hypogeum/internal/highlight"
	"github.com/wilkes/hypogeum/internal/recent"
	"github.com/wilkes/hypogeum/internal/search"
	"github.com/wilkes/hypogeum/internal/tree"
)

// stateFileFn resolves the recent-visit state file. Overridable in tests
// so they never touch the real on-disk state.
var stateFileFn = recent.DefaultStateFile

// loadStore opens the persisted recency store. Returns (nil, err) on a
// hard failure; callers decide whether to degrade or surface the error.
func loadStore() (*recent.Store, error) {
	sf, err := stateFileFn()
	if err != nil {
		return nil, err
	}
	return recent.New(sf)
}

// sanitizeSnippet strips the highlight control chars search embeds so the
// JSON output is clean text.
func sanitizeSnippet(s string) string {
	s = strings.ReplaceAll(s, highlight.Open, "")
	return strings.ReplaceAll(s, highlight.Close, "")
}

// SearchHit is one full-text match.
type SearchHit struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

// Search scans every markdown file under root for term, recency-reranks
// the hits (same ordering as the TUI search modal), and returns at most
// max of them. A nil store degrades to unranked order.
func Search(root, term string, max int) ([]SearchHit, error) {
	paths, err := tree.MarkdownFiles(root)
	if err != nil {
		return nil, err
	}
	hits, err := search.Search(context.Background(), paths, term, max)
	if err != nil {
		return nil, err
	}

	var order func([]string) []string
	if store, serr := loadStore(); serr == nil && store != nil {
		order = func(ps []string) []string {
			ranked := store.Rank(ps)
			out := make([]string, len(ranked))
			for i, r := range ranked {
				out[i] = r.Path
			}
			return out
		}
	}
	hits = search.RerankByRecency(order, hits)

	out := make([]SearchHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, SearchHit{
			Path:    h.Path,
			Line:    h.Line,
			Snippet: sanitizeSnippet(h.Snippet),
		})
	}
	return out, nil
}

// RecentEntry is one recency-ranked note.
type RecentEntry struct {
	Path    string    `json:"path"`
	Score   float64   `json:"score"`
	MTime   time.Time `json:"mtime"`
	Visited time.Time `json:"visited"`
}

// Recent returns up to max markdown files under root, ranked by the
// persisted hybrid recency score.
func Recent(root string, max int) ([]RecentEntry, error) {
	paths, err := tree.MarkdownFiles(root)
	if err != nil {
		return nil, err
	}
	store, err := loadStore()
	if err != nil {
		return nil, err
	}
	ranked := store.Rank(paths)
	if max > 0 && len(ranked) > max {
		ranked = ranked[:max]
	}
	out := make([]RecentEntry, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, RecentEntry{
			Path:    r.Path,
			Score:   r.Score,
			MTime:   r.MTime,
			Visited: r.Visit,
		})
	}
	return out, nil
}

package search

import (
	"context"
	"sort"
	"sync"
)

// SearchAll scans every path for case-insensitive substring matches of
// query and returns ALL hits (no cap) in deterministic (path, line)
// order. Use this when results must be stable run-to-run — e.g. query
// mode, which recency-ranks then caps. The capped Search makes which
// hits survive the cap race-dependent across workers, which SearchAll
// avoids. An empty query or no paths returns nil. Cancellation returns
// the hits gathered so far — still sorted — alongside ctx.Err().
//
// SearchAll is a thin policy wrapper over scan: it collects every hit
// (onHit always returns true) under a mutex and sorts before returning.
func SearchAll(ctx context.Context, paths []string, query string) ([]Hit, error) {
	if query == "" || len(paths) == 0 {
		return nil, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var (
		mu  sync.Mutex
		out []Hit
	)
	err := scan(ctx, paths, query, func(h Hit) bool {
		mu.Lock()
		out = append(out, h)
		mu.Unlock()
		return true // never stop early — collect everything
	})

	// Sort before returning even on cancellation: the doc comment promises
	// deterministic (path, line) order, so a partial slice must be ordered too.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out, err
}

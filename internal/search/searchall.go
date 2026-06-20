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
// the hits gathered so far alongside ctx.Err().
func SearchAll(ctx context.Context, paths []string, query string) ([]Hit, error) {
	if query == "" || len(paths) == 0 {
		return nil, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	workCh := make(chan string, len(paths))
	for _, p := range paths {
		workCh <- p
	}
	close(workCh)

	var (
		mu  sync.Mutex
		out []Hit
		wg  sync.WaitGroup
	)
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range workCh {
				if ctx.Err() != nil {
					return
				}
				hits, err := scanFile(ctx, path, query)
				if err != nil {
					continue // skip unreadable file (matches Search)
				}
				if len(hits) > 0 {
					mu.Lock()
					out = append(out, hits...)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

package search

import (
	"context"
	"sync"
)

// scanFiles fans paths across numWorkers goroutines, calling scanFileLines
// on each and invoking onFile once per file that has at least one match. If
// onFile returns false, scanning stops early (the capped Search/SearchGrouped
// use this to halt once they have gathered enough). Returns ctx.Err() on
// cancellation, nil otherwise.
//
// Behavior shared by every public scanner lives here: workCh fill+close, the
// numWorkers fan-out, per-file ctx.Err() checks, skip-on-I/O-error, and
// stop-on-cancellation. onFile is called concurrently from multiple workers,
// so it must guard its own shared state. scanFiles assumes a non-empty query
// and non-empty paths — callers handle those early returns.
func scanFiles(ctx context.Context, paths []string, query string, onFile func(path string, lines []Line) bool) error {
	workCh := make(chan string, len(paths))
	for _, p := range paths {
		workCh <- p
	}
	close(workCh)

	// stopCtx lets onFile (via a false return) cancel the remaining work
	// without disturbing the caller's ctx.
	stopCtx, stopAll := context.WithCancel(ctx)
	defer stopAll()

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range workCh {
				if stopCtx.Err() != nil {
					return
				}
				lines, err := scanFileLines(stopCtx, path, query)
				if err != nil {
					if stopCtx.Err() != nil {
						return // cancelled — stop immediately
					}
					continue // I/O error on this file — skip and try the next
				}
				if len(lines) == 0 {
					continue // no matches in this file
				}
				if !onFile(path, lines) {
					stopAll() // onFile asked us to stop producing
					return
				}
				if stopCtx.Err() != nil {
					return
				}
			}
		}()
	}
	wg.Wait()

	// Report cancellation only when the caller's ctx was the cause; an
	// onFile-triggered stopAll leaves ctx healthy.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// scan is the hit-oriented adapter over scanFiles, preserved for Search and
// SearchAll: it expands each file's matched lines into Hits (building each
// snippet eagerly via RenderSnippet) and forwards them to onHit. onHit
// returning false stops the whole scan, exactly as before — the per-hit cap
// semantics are unchanged.
func scan(ctx context.Context, paths []string, query string, onHit func(Hit) bool) error {
	return scanFiles(ctx, paths, query, func(path string, lines []Line) bool {
		for _, ln := range lines {
			if !onHit(Hit{Path: path, Line: ln.Num, Snippet: RenderSnippet(ln, snippetBudget)}) {
				return false
			}
		}
		return true
	})
}

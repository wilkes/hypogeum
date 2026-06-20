package search

import (
	"context"
	"sync"
)

// scan fans paths across numWorkers goroutines, calling scanFile on each
// and invoking onHit for every resulting Hit. If onHit returns false,
// scanning stops early (the capped Search uses this to halt once it has
// gathered enough hits). Returns ctx.Err() on cancellation, nil otherwise.
//
// Behavior shared by both Search and SearchAll lives here: workCh
// fill+close, the numWorkers fan-out, per-file ctx.Err() checks,
// skip-on-I/O-error, and stop-on-cancellation. onHit is called
// concurrently from multiple workers, so it must guard its own shared
// state. scan assumes a non-empty query and non-empty paths — callers
// handle those early returns.
func scan(ctx context.Context, paths []string, query string, onHit func(Hit) bool) error {
	workCh := make(chan string, len(paths))
	for _, p := range paths {
		workCh <- p
	}
	close(workCh)

	// stopCtx lets onHit (via a false return) cancel the remaining work
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
				hits, err := scanFile(stopCtx, path, query)
				if err != nil {
					if stopCtx.Err() != nil {
						return // cancelled — stop immediately
					}
					continue // I/O error on this file — skip and try the next
				}
				for _, h := range hits {
					if !onHit(h) {
						stopAll() // onHit asked us to stop producing
						return
					}
					if stopCtx.Err() != nil {
						return
					}
				}
			}
		}()
	}
	wg.Wait()

	// Report cancellation only when the caller's ctx was the cause; an
	// onHit-triggered stopAll leaves ctx healthy.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

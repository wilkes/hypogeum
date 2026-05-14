// Package search scans a set of files for case-insensitive substring
// matches of a query string. It returns Hits in (file, line) order;
// callers responsible for any further sorting (the TUI re-ranks by
// recency before display).
//
// The package has no Bubble Tea, recency, or modal dependencies — it
// imports only stdlib. Workers fan out across paths and respect
// context.Context cancellation between files and roughly every 256
// lines within a file.
package search

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"sync"
)

// Hit is one match: a path, a 1-indexed line number, and a display
// snippet. The snippet is the matched line, optionally trimmed with
// leading/trailing "…" to fit a ~60-char display budget, with the
// matched substring wrapped in highlight markers.
//
// Highlight markers are \x11 (DC1, open) and \x12 (DC2, close). The
// TUI's snippet renderer (internal/tui/backlinks.go applyHighlight)
// turns these into bold yellow SGR. Using ASCII control chars keeps
// the markers invisible to plain-text processing.
type Hit struct {
	Path    string // absolute path
	Line    int    // 1-indexed line number in the source file
	Snippet string // see package doc
}

// snippetHighlightOpen / Close mirror the convention defined in
// internal/vault/snippet.go. They are the data contract between this
// package and the TUI's snippet renderer.
const (
	snippetHighlightOpen  = "\x11" // DC1
	snippetHighlightClose = "\x12" // DC2
)

// buildSnippet wraps the match in line at byte offset matchAt..matchAt+matchLen
// with snippetHighlightOpen/Close. If the resulting display would exceed
// budget runes (markers excluded), it trims with leading/trailing "…"
// biased so the match stays centered.
//
// budget must be >= matchLen+2 (room for the match plus two ellipses).
// Smaller budgets degrade gracefully — the match is preserved at the
// expense of any context.
func buildSnippet(line string, matchAt, matchLen, budget int) string {
	marked := line[:matchAt] +
		snippetHighlightOpen + line[matchAt:matchAt+matchLen] + snippetHighlightClose +
		line[matchAt+matchLen:]
	visibleLen := len(line) // markers add no visible width
	if visibleLen <= budget {
		return marked
	}

	// We need to trim. Compute how many chars of context fit on each
	// side of the match, then decide whether each side needs an ellipsis.
	contextBudget := budget - matchLen - 2 // reserve room for two ellipses
	if contextBudget < 0 {
		contextBudget = 0
	}
	// Initial per-side budgets reserve 1 extra each so redistribution
	// can give the full contextBudget to a single side when the other
	// has no context available (e.g. match at start or end of line).
	inner := contextBudget - 2
	if inner < 0 {
		inner = 0
	}
	leftBudget := inner / 2
	rightBudget := inner - leftBudget

	leftAvail := matchAt
	rightAvail := visibleLen - (matchAt + matchLen)

	leftTake := min(leftAvail, leftBudget)
	rightTake := min(rightAvail, rightBudget)

	// If one side has slack, give it to the other (up to contextBudget total).
	if leftTake < leftBudget {
		rightTake = min(rightAvail, contextBudget-leftTake)
	}
	if rightTake < rightBudget {
		leftTake = min(leftAvail, contextBudget-rightTake)
	}

	var b strings.Builder
	if leftTake < leftAvail {
		b.WriteString("…")
	}
	b.WriteString(line[matchAt-leftTake : matchAt])
	b.WriteString(snippetHighlightOpen)
	b.WriteString(line[matchAt : matchAt+matchLen])
	b.WriteString(snippetHighlightClose)
	b.WriteString(line[matchAt+matchLen : matchAt+matchLen+rightTake])
	if rightTake < rightAvail {
		b.WriteString("…")
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// snippetBudget is the visible-char budget for snippet rendering.
// 60 chars matches the spec's recommended width; the TUI may re-trim
// smaller without re-reading the file.
const snippetBudget = 60

// maxFileBytes caps the read budget for any single file. Files larger
// than this are scanned up to the cap and the rest is silently dropped.
// tree.Walk filters non-markdown so we shouldn't see huge files in
// practice — this is defense-in-depth.
const maxFileBytes = 1 << 20 // 1 MiB

// binaryProbe is the byte count examined for a NUL byte. NUL in the
// first binaryProbe bytes means we treat the file as binary and skip.
const binaryProbe = 512

// scanFile reads path and returns one Hit per line containing
// case-insensitive substring matches of query. The query is assumed
// non-empty (caller's responsibility — Search filters short queries).
//
// Returns an error only for I/O failures opening the file. Cancellation
// returns (nil, ctx.Err()).
func scanFile(ctx context.Context, path, query string) ([]Hit, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Binary probe.
	probe := make([]byte, binaryProbe)
	n, _ := f.Read(probe)
	for i := 0; i < n; i++ {
		if probe[i] == 0 {
			return nil, nil // skip binary file silently
		}
	}
	// Rewind so the scanner sees the same bytes.
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	loweredQuery := strings.ToLower(query)
	queryLen := len(query)
	var hits []Hit

	scanner := bufio.NewScanner(io.LimitReader(f, maxFileBytes))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum&0xFF == 0 { // check ctx every 256 lines
			if ctx.Err() != nil {
				return hits, ctx.Err()
			}
		}
		line := scanner.Text()
		idx := strings.Index(strings.ToLower(line), loweredQuery)
		if idx < 0 {
			continue
		}
		hits = append(hits, Hit{
			Path:    path,
			Line:    lineNum,
			Snippet: buildSnippet(line, idx, queryLen, snippetBudget),
		})
	}
	if err := scanner.Err(); err != nil {
		return hits, err
	}
	return hits, nil
}

// numWorkers is the goroutine fan-out width. Four is enough to overlap
// disk reads on a typical SSD without over-subscribing the OS.
const numWorkers = 4

// Search scans every path for case-insensitive substring matches of
// query. Returns hits in unspecified order (the TUI sorts them). An
// empty query returns nil immediately. Cancellation may return partial
// results; callers should check ctx.Err().
func Search(ctx context.Context, paths []string, query string, maxHits int) ([]Hit, error) {
	if query == "" || len(paths) == 0 || maxHits <= 0 {
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

	hitsCh := make(chan Hit, maxHits)
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
					select {
					case hitsCh <- h:
					case <-stopCtx.Done():
						return
					}
				}
			}
		}()
	}

	// Closer goroutine: when all workers finish, close hitsCh so the
	// collector loop terminates.
	go func() {
		wg.Wait()
		close(hitsCh)
	}()

	var out []Hit
	hitsCapped := false
	for h := range hitsCh {
		out = append(out, h)
		if len(out) >= maxHits {
			hitsCapped = true
			stopAll() // tell workers to stop producing
			// Drain remaining hits without appending so workers don't
			// block on hitsCh.
			go func() {
				for range hitsCh {
				}
			}()
			break
		}
	}
	if !hitsCapped && ctx.Err() != nil {
		return out, ctx.Err()
	}
	return out, nil
}

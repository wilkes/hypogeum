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

	"github.com/wilkes/hypogeum/internal/highlight"
)

// Hit is one match: a path, a 1-indexed line number, and a display
// snippet. The snippet is the matched line, optionally trimmed with
// leading/trailing "…" to fit a ~60-char display budget, with the
// matched substring wrapped in highlight markers.
//
// Highlight markers are highlight.Open (\x11 / DC1) and highlight.Close
// (\x12 / DC2), defined in internal/highlight as the single source of
// truth for the wire format. The TUI's snippet renderer
// (internal/tui/backlinks.go applyHighlight) turns these into bold
// yellow SGR. Using ASCII control chars keeps the markers invisible to
// plain-text processing.
type Hit struct {
	Path    string // absolute path
	Line    int    // 1-indexed line number in the source file
	Snippet string // see package doc
}

// buildSnippet wraps the match in line at byte offset matchAt..matchAt+matchLen
// with highlight.Open/Close. If the resulting display would exceed
// budget runes (markers excluded), it trims with leading/trailing "…"
// biased so the match stays centered.
//
// budget must be >= matchLen+2 (room for the match plus two ellipses).
// Smaller budgets degrade gracefully — the match is preserved at the
// expense of any context.
func buildSnippet(line string, matchAt, matchLen, budget int) string {
	marked := line[:matchAt] +
		highlight.Open + line[matchAt:matchAt+matchLen] + highlight.Close +
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
	b.WriteString(highlight.Open)
	b.WriteString(line[matchAt : matchAt+matchLen])
	b.WriteString(highlight.Close)
	b.WriteString(line[matchAt+matchLen : matchAt+matchLen+rightTake])
	if rightTake < rightAvail {
		b.WriteString("…")
	}
	return b.String()
}

// snippetBudget is the visible-char budget for snippet rendering.
// 60 chars matches the spec's recommended width; the TUI may re-trim
// smaller without re-reading the file.
const snippetBudget = 60

// maxFileBytes caps the read budget for any single file. Files larger
// than this are scanned up to the cap and the rest is silently dropped.
// The TUI filters to markdown so we shouldn't see huge files in
// practice — this is defense-in-depth.
const maxFileBytes = 1 << 20 // 1 MiB

// scanBufSize is the initial bufio.Scanner buffer size handed to each
// scanFile. A full-vault search allocates one of these per file; pooling
// them keeps the fan-out from churning ~64 KiB per scanned file through
// the GC. The scanner only grows past this for lines longer than 64 KiB,
// in which case it allocates internally and leaves the pooled buffer
// untouched — so returning the original to the pool is always safe.
const scanBufSize = 64 * 1024

// bufPool recycles scanFile's initial scanner buffers. Pooling *[]byte
// (not []byte) avoids the per-Put allocation of boxing a slice header.
var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, scanBufSize)
		return &b
	},
}

// scanFileLines reads path and returns one Line per line containing
// case-insensitive substring matches of query, in ascending line order.
// The query is assumed non-empty (caller's responsibility). Snippet
// construction is deferred — a Line carries only the raw text plus the
// match offset/length, so both the hit-oriented scan (which builds a
// Hit's snippet per line) and the grouped scan (which builds snippets
// lazily, only for displayed matches) share this single match-finding pass.
//
// Returns an error only for I/O failures opening the file. Cancellation
// returns the lines gathered so far alongside ctx.Err().
func scanFileLines(ctx context.Context, path, query string) ([]Line, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	loweredQuery := strings.ToLower(query)
	queryLen := len(query)
	var lines []Line

	bufp := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufp)

	scanner := bufio.NewScanner(io.LimitReader(f, maxFileBytes))
	scanner.Buffer(*bufp, 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum&0xFF == 0 { // check ctx every 256 lines
			if ctx.Err() != nil {
				return lines, ctx.Err()
			}
		}
		lineB := scanner.Bytes()
		idx := indexFold(lineB, loweredQuery)
		if idx < 0 {
			continue
		}
		lines = append(lines, Line{
			Num: lineNum,
			// string(lineB) allocates only here, on an actual match.
			Text: string(lineB),
			At:   idx,
			Len:  queryLen,
		})
	}
	if err := scanner.Err(); err != nil {
		return lines, err
	}
	return lines, nil
}

// foldByte lower-cases an ASCII letter; all other bytes pass through.
func foldByte(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// indexFold reports the byte index of the first ASCII-case-insensitive match
// of lowerNeedle in haystack, or -1 if absent. lowerNeedle must already be
// lower-cased. It allocates nothing — the previous implementation lower-cased
// a fresh copy of every scanned line via strings.ToLower.
//
// Semantics note: folding is ASCII-only (A–Z). Bytes >= 0x80 compare exactly,
// so a query "café" still matches "café" but an uppercase "CAFÉ" no longer
// matches a lowercase query the way the old Unicode-aware ToLower did. This is
// a deliberate narrowing; vault content is overwhelmingly ASCII-cased.
func indexFold(haystack []byte, lowerNeedle string) int {
	m := len(lowerNeedle)
	if m == 0 {
		return 0
	}
	n := len(haystack)
	if m > n {
		return -1
	}
	first := lowerNeedle[0]
	last := m - 1
	for i := 0; i+m <= n; i++ {
		if foldByte(haystack[i]) != first {
			continue
		}
		if foldByte(haystack[i+last]) != lowerNeedle[last] {
			continue
		}
		ok := true
		for j := 1; j < last; j++ {
			if foldByte(haystack[i+j]) != lowerNeedle[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// numWorkers is the goroutine fan-out width. Four is enough to overlap
// disk reads on a typical SSD without over-subscribing the OS.
const numWorkers = 4

// Search scans every path for case-insensitive substring matches of
// query, returning at most maxHits hits in unspecified order (the TUI
// sorts them). An empty query, no paths, or maxHits <= 0 returns nil
// immediately. Cancellation may return partial results; callers should
// check ctx.Err().
//
// Search is a thin policy wrapper over scan: it collects hits under a
// mutex and stops the fan-out early once it has gathered maxHits. Which
// hits survive the cap is race-dependent across workers — callers needing
// deterministic results should use SearchAll.
func Search(ctx context.Context, paths []string, query string, maxHits int) ([]Hit, error) {
	if query == "" || len(paths) == 0 || maxHits <= 0 {
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
		defer mu.Unlock()
		if len(out) >= maxHits {
			return false // already full — stop the fan-out
		}
		out = append(out, h)
		return len(out) < maxHits // false once this hit fills the cap
	})

	// A full cap is a normal stop, not a cancellation; only surface
	// ctx.Err() when we didn't reach maxHits.
	if len(out) < maxHits && err != nil {
		return out, err
	}
	return out, nil
}

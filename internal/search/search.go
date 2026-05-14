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

import "strings"

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

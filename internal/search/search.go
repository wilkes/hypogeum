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

package search

import (
	"context"
	"sync"
)

// Line is one matching line within a file: its 1-indexed number, the raw
// line text, and the byte offset+length of the match within that text.
// Snippet construction (highlight markers + ellipsis trimming) is deferred
// to RenderSnippet so a grouped scan can count every match cheaply without
// paying the snippet cost for matches the caller never displays.
type Line struct {
	Num  int    // 1-indexed line number
	Text string // the raw matching line
	At   int    // byte offset of the match within Text
	Len  int    // match length in bytes
}

// FileMatches is one file's full-text match summary: the file path, the
// total number of matching lines, and every matching Line in ascending
// line order. Count always equals len(Lines); it is surfaced explicitly
// because it is the value the grouped search modal renders as its headline.
type FileMatches struct {
	Path  string
	Count int
	Lines []Line
}

// SnippetBudget is the default visible-char budget for RenderSnippet —
// an exported mirror of the internal default so callers (the TUI) don't
// hard-code the width.
const SnippetBudget = snippetBudget

// RenderSnippet builds the display snippet for one matching line: the line
// text with the match wrapped in highlight markers, trimmed to budget
// visible chars. This is the lazy half of grouped search — the TUI calls it
// only for the matches of an expanded file, so a collapsed result set builds
// zero snippets.
func RenderSnippet(line Line, budget int) string {
	return buildSnippet(line.Text, line.At, line.Len, budget)
}

// SearchGrouped scans paths for case-insensitive substring matches of query
// and returns per-file match summaries, capped at maxFiles files — NOT hits.
// Counting within a scanned file is exhaustive, so a file's Count is always
// the true number of matching lines; the cap bounds how many files come back,
// which is far more coverage than the per-hit cap of Search. Results are in
// unspecified order (the caller sorts — the TUI by count desc, then recency).
// An empty query, no paths, or maxFiles <= 0 returns nil. Cancellation may
// return partial results; callers should check ctx.Err().
//
// Like Search, this is a thin policy wrapper over scanFiles: it collects one
// FileMatches per matching file under a mutex and stops the fan-out once it
// has gathered maxFiles. Which files survive the cap is race-dependent across
// workers, matching Search's documented behavior.
func SearchGrouped(ctx context.Context, paths []string, query string, maxFiles int) ([]FileMatches, error) {
	if query == "" || len(paths) == 0 || maxFiles <= 0 {
		return nil, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var (
		mu  sync.Mutex
		out []FileMatches
	)
	err := scanFiles(ctx, paths, query, func(path string, lines []Line) bool {
		mu.Lock()
		defer mu.Unlock()
		if len(out) >= maxFiles {
			return false // already full — stop the fan-out
		}
		out = append(out, FileMatches{Path: path, Count: len(lines), Lines: lines})
		return len(out) < maxFiles // false once this file fills the cap
	})

	// A full cap is a normal stop, not a cancellation; only surface ctx.Err()
	// when we didn't reach maxFiles.
	if len(out) < maxFiles && err != nil {
		return out, err
	}
	return out, nil
}

// Package highlight defines the in-band control-character markers used to
// bracket matched text inside snippets (search hits, backlink references).
// The TUI render path translates these into reverse-video styling.
//
// Using ASCII control chars (DC1/DC2) keeps the markers invisible to
// plain-text processing while staying distinguishable from anything in
// user content. This package is the single source of truth for the wire
// format; producers (internal/search, internal/vault) and the TUI
// consumer all agree on these bytes.
package highlight

import "strings"

const (
	Open  = "\x11" // DC1 — start of a highlighted span
	Close = "\x12" // DC2 — end of a highlighted span
)

// Wrap brackets s in the Open/Close highlight markers.
func Wrap(s string) string {
	return Open + s + Close
}

// Strip removes every Open and Close marker from s, leaving the bracketed
// text in place. It is the plain-text inverse of Wrap — use it when a
// snippet must be displayed without highlighting (the TUI's SGR-styling
// path uses applyHighlight instead). Markers may be unbalanced; each is
// removed independently.
func Strip(s string) string {
	if !strings.ContainsAny(s, Open+Close) {
		return s
	}
	return strings.NewReplacer(Open, "", Close, "").Replace(s)
}

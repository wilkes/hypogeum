package vault

import (
	"path/filepath"
)

// OutboundKind distinguishes a wikilink reference from a standard
// markdown-link reference. It mirrors the internal referenceKind but is
// part of the exported API.
type OutboundKind int

const (
	OutboundWikilink OutboundKind = iota
	OutboundStdLink
)

// Outbound is one outgoing reference from a file. It is the symmetric
// counterpart of Backlink (which describes references *into* a file).
type Outbound struct {
	DisplayText string       // the visible link text
	RawTarget   string       // wikilink name (e.g. "bar") or std-link href
	Resolved    string       // absolute target path, or "" if unresolved
	Line        int          // 1-indexed source line
	Snippet     string       // surrounding-line context
	Kind        OutboundKind // wikilink vs standard markdown link
}

// Outbound returns every outgoing reference from path, in document
// order. Returns an empty slice when path is not indexed.
func (v *Vault) Outbound(path string) []Outbound {
	v.mu.RLock()
	defer v.mu.RUnlock()

	abs, _ := filepath.Abs(path)
	entry, ok := v.files[abs]
	if !ok {
		return nil
	}
	out := make([]Outbound, 0, len(entry.refs))
	for _, ref := range entry.refs {
		kind := OutboundStdLink
		if ref.kind == refWikilink {
			kind = OutboundWikilink
		}
		out = append(out, Outbound{
			DisplayText: ref.displayText,
			RawTarget:   ref.target,
			Resolved:    ref.resolved,
			Line:        ref.line,
			Snippet:     ref.snippet,
			Kind:        kind,
		})
	}
	return out
}

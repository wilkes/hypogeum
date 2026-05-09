// Package wikilink parses the body of a [[...]] wikilink into its
// components. Both the vault index (which builds reference graphs)
// and the markdown renderer (which rewrites wikilinks to standard
// links before Glamour renders them) use this parser.
package wikilink

import "strings"

// Body is the parsed components of a [[Name#Heading^Block|Alias]] wikilink.
type Body struct {
	Name    string
	Heading string
	Block   string
	Alias   string
}

// Parse splits the inside of [[...]] into its components.
// Returns nil if the body is empty or the name is empty.
// The order of components in the source is fixed: Name first,
// then optional #Heading, then optional ^Block, then optional |Alias.
func Parse(body string) *Body {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	w := &Body{}
	if i := strings.IndexByte(body, '|'); i >= 0 {
		w.Alias = strings.TrimSpace(body[i+1:])
		body = body[:i]
	}
	if i := strings.IndexByte(body, '^'); i >= 0 {
		w.Block = strings.TrimSpace(body[i+1:])
		body = body[:i]
	}
	if i := strings.IndexByte(body, '#'); i >= 0 {
		w.Heading = strings.TrimSpace(body[i+1:])
		body = body[:i]
	}
	w.Name = strings.TrimSpace(body)
	if w.Name == "" {
		return nil
	}
	return w
}

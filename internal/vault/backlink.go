package vault

import (
	"path/filepath"
	"sort"
)

// Backlink is one cross-reference *to* a given file. Returned by Backlinks
// for the TUI to render in the backlinks modal.
type Backlink struct {
	SourceFile  string
	DisplayText string
	Snippet     string
	Line        int
	Kind        BacklinkKind
}

type BacklinkKind int

const (
	BacklinkWikilink BacklinkKind = iota
	BacklinkStdLink
)

// Backlinks returns every reference *to* path in document order across
// files. Includes both wikilink and standard-markdown-link references.
func (v *Vault) Backlinks(path string) []Backlink {
	v.mu.RLock()
	defer v.mu.RUnlock()

	abs, _ := filepath.Abs(path)
	var out []Backlink
	for src, entry := range v.files {
		for _, ref := range entry.refs {
			if ref.resolved == "" || ref.resolved != abs {
				continue
			}
			kind := BacklinkStdLink
			if ref.kind == refWikilink {
				kind = BacklinkWikilink
			}
			out = append(out, Backlink{
				SourceFile:  src,
				DisplayText: ref.displayText,
				Snippet:     ref.snippet,
				Line:        ref.line,
				Kind:        kind,
			})
		}
	}
	// Stable order: sort by source file, then by line, so test fixtures
	// don't depend on map iteration order.
	sortBacklinks(out)
	return out
}

func sortBacklinks(b []Backlink) {
	sort.Slice(b, func(i, j int) bool {
		if b[i].SourceFile != b[j].SourceFile {
			return b[i].SourceFile < b[j].SourceFile
		}
		return b[i].Line < b[j].Line
	})
}

package vault

import (
	"path/filepath"
	"sort"
	"strings"
)

// Resolve returns the absolute path the wikilink target resolves to,
// or ("", false) if no file matches. fromFile is the file containing
// the wikilink; it's used as the proximity reference when multiple
// files share a basename.
//
// The lookup is case-insensitive on basename (without extension).
// The block argument is recorded in references but not used for
// resolution in Phase 1 — the caller still gets the file path.
func (v *Vault) Resolve(fromFile, name, heading, block string) (string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	candidates := v.names[strings.ToLower(name)]
	if len(candidates) == 0 {
		return "", false
	}
	return scoreProximity(candidates, fromFile), true
}

// scoreProximity picks the best candidate for fromFile by counting
// shared leading path components. Ties broken by lexical order.
func scoreProximity(candidates []string, fromFile string) string {
	if len(candidates) == 1 {
		return candidates[0]
	}
	type scored struct {
		path string
		dist int
	}
	scoredCands := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		rel, err := filepath.Rel(filepath.Dir(fromFile), c)
		if err != nil {
			rel = c
		}
		scoredCands = append(scoredCands, scored{path: c, dist: len(rel)})
	}
	sort.Slice(scoredCands, func(i, j int) bool {
		if scoredCands[i].dist != scoredCands[j].dist {
			return scoredCands[i].dist < scoredCands[j].dist
		}
		return scoredCands[i].path < scoredCands[j].path
	})
	return scoredCands[0].path
}

// ResolveAnchor looks up the destination line for a heading or block
// anchor inside the file at path. Both args empty returns (0, false).
// When both heading and block are non-empty, block wins (it's more
// specific — Obsidian's `#Heading^block` syntax).
func (v *Vault) ResolveAnchor(path, heading, block string) (int, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	entry := v.files[path]
	if entry == nil {
		return 0, false
	}
	if block != "" {
		line, ok := entry.anchors.blocks[block]
		return line, ok
	}
	if heading != "" {
		line, ok := entry.anchors.headings[slugifyAnchor(heading)]
		return line, ok
	}
	return 0, false
}

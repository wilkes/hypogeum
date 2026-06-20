package vault

import (
	"path/filepath"

	"github.com/wilkes/hypogeum/internal/tree"
)

// OutboundFor returns the outgoing references of a single file without building
// the full reference graph. It walks root for markdown *filenames* only — the
// cheap half of Build, enough to resolve wikilinks by basename — and parses
// just `file`, skipping the read+parse of every other file in the vault.
//
// It cannot produce backlinks (those need every file's outgoing links — use
// Build). For the target file's outbound links the result is identical to
// Build(root).Outbound(file): wikilink resolution sees the same name index
// (every file's path is recorded either way), and std links resolve against
// the filesystem independent of the vault. Returns nil for a file Build would
// not index (non-markdown or unreadable), matching Build's Outbound.
func OutboundFor(root, file string, diag Diagnostics) ([]Outbound, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	fileAbs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	if !tree.IsMarkdown(filepath.Base(fileAbs)) {
		return nil, nil // Build only indexes markdown files
	}

	v := &Vault{
		root:  rootAbs,
		files: make(map[string]*fileEntry),
		names: make(map[string][]string),
		diag:  diag,
	}
	if err := v.indexNames(); err != nil {
		return nil, err
	}
	v.indexFile(newMarkdownParser(), fileAbs)
	v.resolveFileRefs(fileAbs)
	return v.Outbound(fileAbs), nil
}

// indexNames records every markdown file's path under its basename key in
// v.names, without reading any file's contents. It is Build's name index
// without the reference graph — enough for wikilink resolution by name.
func (v *Vault) indexNames() error {
	paths, err := v.markdownPaths()
	if err != nil {
		return err
	}
	for _, p := range paths {
		key := nameKey(p)
		v.names[key] = append(v.names[key], p)
	}
	return nil
}

package vault

import (
	"path/filepath"
	"sync"
)

// Vault is the in-memory index of a directory of markdown files.
//
// State:
//   - files: forward index, keyed by absolute path. Each entry's refs
//     are this file's outgoing references (wikilinks + standard links).
//   - names: name index, keyed by lowercased basename without extension.
//     Used by Resolve to find a file matching a wikilink target.
//
// The reverse index (which files link *to* a given path) is computed
// on demand from `files` — see Backlinks.
type Vault struct {
	root  string
	mu    sync.RWMutex
	files map[string]*fileEntry
	names map[string][]string
	diag  Diagnostics
}

type fileEntry struct {
	path string
	refs []reference
}

type referenceKind int

const (
	refWikilink referenceKind = iota
	refStdLink
)

type reference struct {
	kind        referenceKind
	target      string // raw [[Target]] (wikilink) or href (stdlink)
	resolved    string // absolute path of the target file, "" if unresolved
	heading     string
	block       string
	alias       string
	displayText string
	snippet     string
	line        int
}

// Backlink is one cross-reference *to* a given file. Returned by Backlinks
// for the TUI to render in the persistent pane and modal.
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

// Build walks root and indexes every .md file's wikilinks and standard
// markdown links. The diag sink receives non-fatal issues; pass
// NopDiagnostics for tests that don't care.
//
// Returns a Vault even when individual files fail to parse — those
// emit a diagnostic and are skipped. A fatal error (root unreadable)
// returns (nil, err).
func Build(root string, diag Diagnostics) (*Vault, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	v := &Vault{
		root:  abs,
		files: make(map[string]*fileEntry),
		names: make(map[string][]string),
		diag:  diag,
	}
	// Indexing populated in a later task. For now, Build returns an
	// empty vault — the TUI can wire it in without behavior changes.
	return v, nil
}

// RefreshFile re-parses one file's outgoing references and updates
// both indexes. Called on watch.FileModified.
func (v *Vault) RefreshFile(path string) error {
	// Implementation in Stage 6.
	return nil
}

// Rebuild re-walks the entire root. Called on watch.StructureChanged.
func (v *Vault) Rebuild() error {
	// Implementation in Stage 6.
	return nil
}

// Resolve returns the absolute path the wikilink target resolves to,
// or ("", false) if no file matches. fromFile is the file containing
// the wikilink (used for proximity tiebreaking when multiple files
// share a basename).
func (v *Vault) Resolve(fromFile, name, heading, block string) (path string, ok bool) {
	// Implementation in Stage 4.
	return "", false
}

// Backlinks returns every reference *to* path in document order across
// files. Includes both wikilink and standard-markdown-link references.
func (v *Vault) Backlinks(path string) []Backlink {
	// Implementation in Stage 4.
	return nil
}

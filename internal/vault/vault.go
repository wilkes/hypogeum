package vault

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
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
	if err := v.walkAndIndex(); err != nil {
		return nil, err
	}
	v.resolveAllRefs()
	return v, nil
}

// RefreshFile re-parses one file's outgoing references and updates
// both indexes. Called on watch.FileModified.
func (v *Vault) RefreshFile(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	// If the file is gone, drop its entry. The watcher will follow
	// up with a StructureChanged for the directory, which calls
	// Rebuild — so getting Resolve right here is best-effort.
	if _, statErr := os.Stat(abs); statErr != nil {
		v.mu.Lock()
		delete(v.files, abs)
		key := nameKey(abs)
		v.names[key] = removePath(v.names[key], abs)
		if len(v.names[key]) == 0 {
			delete(v.names, key)
		}
		v.mu.Unlock()
		v.diag.Info("vault: file vanished before refresh: " + abs)
		return nil
	}

	v.indexFile(abs)
	// Re-resolve all wikilinks in case this file's appearance/change
	// affects resolution (e.g. it newly satisfies a name lookup).
	v.resolveAllRefs()
	return nil
}

// Rebuild re-walks the entire root. Called on watch.StructureChanged.
func (v *Vault) Rebuild() error {
	v.mu.Lock()
	v.files = make(map[string]*fileEntry)
	v.names = make(map[string][]string)
	v.mu.Unlock()
	if err := v.walkAndIndex(); err != nil {
		return err
	}
	v.resolveAllRefs()
	return nil
}

func removePath(s []string, p string) []string {
	out := s[:0]
	for _, x := range s {
		if x != p {
			out = append(out, x)
		}
	}
	return out
}

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

// walkAndIndex populates v.files and v.names by walking v.root.
// Per-file parse failures emit a Warn diagnostic and are skipped.
// A walk-level error (root unreadable) is fatal.
func (v *Vault) walkAndIndex() error {
	return filepath.WalkDir(v.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != v.root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || !isMarkdownExt(d.Name()) {
			return nil
		}
		v.indexFile(path)
		return nil
	})
}

// indexFile parses one file and stores its outgoing references and
// name-index entry. Errors emit a Warn diagnostic and leave the file
// out of the index.
func (v *Vault) indexFile(path string) {
	src, err := os.ReadFile(path)
	if err != nil {
		v.diag.Warn("vault: read " + path + ": " + err.Error())
		return
	}
	refs := extractReferences(string(src), path)

	v.mu.Lock()
	defer v.mu.Unlock()

	v.files[path] = &fileEntry{path: path, refs: refs}

	key := nameKey(path)
	// Keep names index unique-by-path: drop any prior occurrence of this
	// path under this key (in case of rename-in-place edge cases).
	existing := v.names[key]
	deduped := existing[:0]
	for _, p := range existing {
		if p != path {
			deduped = append(deduped, p)
		}
	}
	v.names[key] = append(deduped, path)
}

// extractReferences parses src as markdown (with the wikilink extension)
// and returns one reference per outgoing link, in document order.
// Standard ast.Link nodes become refStdLink entries; wikilinkNode
// instances become refWikilink entries.
func extractReferences(src, fromPath string) []reference {
	source := []byte(src)
	md := goldmark.New(goldmark.WithExtensions(WikilinkExtension))
	doc := md.Parser().Parse(text.NewReader(source))

	var refs []reference
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch nn := n.(type) {
		case *wikilinkNode:
			disp := nn.Alias
			if disp == "" {
				disp = nn.Name
			}
			refs = append(refs, reference{
				kind:        refWikilink,
				target:      nn.Name,
				heading:     nn.Heading,
				block:       nn.Block,
				alias:       nn.Alias,
				displayText: disp,
				snippet:     snippetForNode(nn, source, disp),
				line:        lineForNode(nn, source),
			})
			return ast.WalkSkipChildren, nil
		case *ast.Link:
			href := string(nn.Destination)
			disp := linkText(nn, source)
			refs = append(refs, reference{
				kind:        refStdLink,
				target:      href,
				resolved:    resolveStdLink(fromPath, href),
				displayText: disp,
				snippet:     snippetForNode(nn, source, disp),
				line:        lineForNode(nn, source),
			})
			return ast.WalkSkipChildren, nil
		case *ast.Image:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return refs
}

// lineForNode returns the 1-indexed line of the first segment of n
// within source. Returns 0 if no segment is found (rare — defensive).
func lineForNode(n ast.Node, source []byte) int {
	var seg *ast.Text
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := c.(*ast.Text); ok {
			seg = t
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if seg == nil {
		return 0
	}
	stop := seg.Segment.Start
	if stop > len(source) {
		stop = len(source)
	}
	line := 1
	for i := 0; i < stop; i++ {
		if source[i] == '\n' {
			line++
		}
	}
	return line
}

// linkText returns the visible text under a *ast.Link.
func linkText(n ast.Node, source []byte) string {
	var out []byte
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			out = append(out, t.Segment.Value(source)...)
			continue
		}
		out = append(out, []byte(linkText(c, source))...)
	}
	return string(out)
}

// nameKey is the basename without extension, lowercased — the key used
// in v.names for wikilink lookups.
func nameKey(path string) string {
	name := filepath.Base(path)
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[:i]
	}
	return strings.ToLower(name)
}

// isMarkdownExt mirrors internal/tree's set; duplicated to keep vault
// independent of tree.
func isMarkdownExt(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	}
	return false
}

// fileCount is exposed for tests. Not part of the public API.
func (v *Vault) fileCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.files)
}

// resolveStdLink resolves a standard markdown link's href against
// the file containing it. Returns "" if the href is empty, an
// external URL, or a same-document anchor — none of which produce
// a backlink to another file in the vault.
func resolveStdLink(fromPath, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return ""
	}
	u, err := url.Parse(href)
	if err == nil && u.Scheme != "" && u.Scheme != "file" {
		return ""
	}
	target := href
	if u != nil && u.Path != "" {
		target = u.Path
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(fromPath), target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return ""
	}
	return abs
}

// resolveAllRefs fills in the `resolved` field for every wikilink
// reference now that the names index is fully populated. Standard
// links are already resolved during indexFile.
func (v *Vault) resolveAllRefs() {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, entry := range v.files {
		for i := range entry.refs {
			if entry.refs[i].kind != refWikilink {
				continue
			}
			path, ok := v.resolveLocked(entry.path, entry.refs[i].target)
			if ok {
				entry.refs[i].resolved = path
			}
		}
	}
}

// resolveLocked is Resolve without the read lock — used by
// resolveAllRefs which already holds the write lock.
func (v *Vault) resolveLocked(fromFile, name string) (string, bool) {
	candidates := v.names[strings.ToLower(name)]
	if len(candidates) == 0 {
		return "", false
	}
	if len(candidates) == 1 {
		return candidates[0], true
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
	return scoredCands[0].path, true
}

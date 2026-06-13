package vault

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wilkes/hypogeum/internal/pathutil"
	"github.com/wilkes/hypogeum/internal/tree"
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
		if strings.HasPrefix(d.Name(), ".") || !tree.IsMarkdown(d.Name()) {
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

// nameKey is the basename without extension, lowercased — the key used
// in v.names for wikilink lookups.
func nameKey(path string) string {
	name := filepath.Base(path)
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[:i]
	}
	return strings.ToLower(name)
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
	abs, err := pathutil.ResolveRelativeTo(fromPath, target)
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
	return scoreProximity(candidates, fromFile), true
}

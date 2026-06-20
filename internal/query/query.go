// Package query is the non-interactive backend for hypogeum's scripting
// verbs. It orchestrates the pure tree/search/vault/recent packages and
// returns JSON-tagged result structs. It has no TUI dependencies.
package query

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wilkes/hypogeum/internal/highlight"
	"github.com/wilkes/hypogeum/internal/recent"
	"github.com/wilkes/hypogeum/internal/search"
	"github.com/wilkes/hypogeum/internal/tree"
	"github.com/wilkes/hypogeum/internal/vault"
)

// stateFileFn resolves the recent-visit state file. Overridable in tests
// so they never touch the real on-disk state.
var stateFileFn = recent.DefaultStateFile

// loadStore opens the persisted recency store. Returns (nil, err) on a
// hard failure; callers decide whether to degrade or surface the error.
func loadStore() (*recent.Store, error) {
	sf, err := stateFileFn()
	if err != nil {
		return nil, err
	}
	return recent.New(sf)
}

// sanitizeSnippet strips the highlight control chars search embeds so the
// JSON output is clean text.
func sanitizeSnippet(s string) string {
	s = strings.ReplaceAll(s, highlight.Open, "")
	return strings.ReplaceAll(s, highlight.Close, "")
}

// SearchHit is one full-text match.
type SearchHit struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

// Search scans every markdown file under root for term, recency-reranks
// the hits (same ordering as the TUI search modal), and returns at most
// max of them. A nil store degrades to unranked order.
func Search(root, term string, max int) ([]SearchHit, error) {
	paths, err := tree.MarkdownFiles(root)
	if err != nil {
		return nil, err
	}
	hits, err := search.Search(context.Background(), paths, term, max)
	if err != nil {
		return nil, err
	}

	var order func([]string) []string
	if store, serr := loadStore(); serr == nil && store != nil {
		order = func(ps []string) []string {
			ranked := store.Rank(ps)
			out := make([]string, len(ranked))
			for i, r := range ranked {
				out[i] = r.Path
			}
			return out
		}
	}
	hits = search.RerankByRecency(order, hits)

	out := make([]SearchHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, SearchHit{
			Path:    h.Path,
			Line:    h.Line,
			Snippet: sanitizeSnippet(h.Snippet),
		})
	}
	return out, nil
}

// RecentEntry is one recency-ranked note.
type RecentEntry struct {
	Path    string    `json:"path"`
	Score   float64   `json:"score"`
	MTime   time.Time `json:"mtime"`
	Visited time.Time `json:"visited"`
}

// Recent returns up to max markdown files under root, ranked by the
// persisted hybrid recency score.
func Recent(root string, max int) ([]RecentEntry, error) {
	paths, err := tree.MarkdownFiles(root)
	if err != nil {
		return nil, err
	}
	store, err := loadStore()
	if err != nil {
		return nil, err
	}
	ranked := store.Rank(paths)
	if max > 0 && len(ranked) > max {
		ranked = ranked[:max]
	}
	out := make([]RecentEntry, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, RecentEntry{
			Path:    r.Path,
			Score:   r.Score,
			MTime:   r.MTime,
			Visited: r.Visit,
		})
	}
	return out, nil
}

// Link is one outbound edge from a file.
type Link struct {
	Text   string `json:"text"`
	Target string `json:"target"`
	Path   string `json:"path"`
	Kind   string `json:"kind"` // wikilink | relative | external
	Broken bool   `json:"broken"`
}

// BacklinkEntry is one reference into a file.
type BacklinkEntry struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
	Text    string `json:"text"`
}

// Neighborhood is a file's 1-hop context bundle (named Neighborhood, not
// Neighbors, because a type and the Neighbors function can't share a name in
// one package).
type Neighborhood struct {
	File      string          `json:"file"`
	Outbound  []Link          `json:"outbound"`
	Backlinks []BacklinkEntry `json:"backlinks"`
}

// isExternalURL reports whether target is an http/https URL.
func isExternalURL(target string) bool {
	u, err := url.Parse(target)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

// outboundLinks maps a vault file's outbound references into Link values.
//
// Broken-ness is determined here, not in the vault accessor:
//   - wikilink: broken when the vault left Resolved empty (existence-based
//     via the names index, so Resolved already implies the file exists).
//   - relative: the vault computes Resolved by pure path math without
//     checking existence, so we os.Stat it. A missing target is broken and
//     reports an empty Path (matching the spec's broken-relative example).
//   - external: never broken (we do not probe URLs).
func outboundLinks(v *vault.Vault, abs string) []Link {
	refs := v.Outbound(abs)
	out := make([]Link, 0, len(refs))
	for _, r := range refs {
		l := Link{
			Text: r.DisplayText,
			Path: r.Resolved,
		}
		switch {
		case r.Kind == vault.OutboundWikilink:
			l.Kind = "wikilink"
			l.Target = "[[" + r.RawTarget + "]]"
			l.Broken = r.Resolved == ""
		case isExternalURL(r.RawTarget):
			l.Kind = "external"
			l.Target = r.RawTarget
			l.Path = ""
			l.Broken = false
		default:
			l.Kind = "relative"
			l.Target = r.RawTarget
			if r.Resolved == "" || !fileExists(r.Resolved) {
				l.Path = ""
				l.Broken = true
			}
		}
		out = append(out, l)
	}
	return out
}

// fileExists reports whether path names an existing entry on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// mustExist returns an absolute path for file, or an error if file does
// not exist on disk. A missing file argument is an operational failure
// (exit 1), distinct from a file that simply has zero links.
func mustExist(file string) (string, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("file not found: %s", file)
	}
	return abs, nil
}

// Links returns the outbound edges from file within the vault at root.
func Links(root, file string) ([]Link, error) {
	abs, err := mustExist(file)
	if err != nil {
		return nil, err
	}
	v, err := vault.Build(root, vault.NopDiagnostics{})
	if err != nil {
		return nil, err
	}
	return outboundLinks(v, abs), nil
}

// Neighbors returns file's outbound links and its backlinks.
func Neighbors(root, file string) (Neighborhood, error) {
	abs, err := mustExist(file)
	if err != nil {
		return Neighborhood{}, err
	}
	v, err := vault.Build(root, vault.NopDiagnostics{})
	if err != nil {
		return Neighborhood{}, err
	}
	n := Neighborhood{
		File:     abs,
		Outbound: outboundLinks(v, abs),
	}
	for _, b := range v.Backlinks(abs) {
		n.Backlinks = append(n.Backlinks, BacklinkEntry{
			Path:    b.SourceFile,
			Line:    b.Line,
			Snippet: sanitizeSnippet(b.Snippet),
			Text:    b.DisplayText,
		})
	}
	if n.Backlinks == nil {
		n.Backlinks = []BacklinkEntry{}
	}
	if n.Outbound == nil {
		n.Outbound = []Link{}
	}
	return n, nil
}

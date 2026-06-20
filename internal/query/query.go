// Package query is the non-interactive backend for hypogeum's scripting
// verbs. It orchestrates the pure tree/search/vault/recent packages and
// returns JSON-tagged result structs. It has no TUI dependencies.
package query

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wilkes/hypogeum/internal/highlight"
	"github.com/wilkes/hypogeum/internal/markdown"
	"github.com/wilkes/hypogeum/internal/recent"
	"github.com/wilkes/hypogeum/internal/search"
	"github.com/wilkes/hypogeum/internal/tree"
	"github.com/wilkes/hypogeum/internal/vault"
)

// stateFileFn resolves the recent-visit state file. Overridable in tests
// so they never touch the real on-disk state.
var stateFileFn = recent.DefaultStateFile

// loadStore opens the persisted recency store.
//
// Contract: recent.New returns a USABLE (non-nil) Store even when the
// state file is malformed or carries an unknown version — the visit map
// is simply empty, but the store can still rank by filesystem mtime. We
// preserve that here: a non-nil store is always returned for callers to
// use, and any error is surfaced *alongside* it as a diagnostic rather
// than a fatal signal. Callers should prefer graceful degradation (use
// the store, ignore the error) over hard-failing — both the `search` and
// `recent` verbs do. Only a nil store (the stateFileFn failure path) is
// truly unusable.
func loadStore() (*recent.Store, error) {
	sf, err := stateFileFn()
	if err != nil {
		return nil, err
	}
	return recent.New(sf)
}

// sanitizeSnippet strips the highlight control chars search embeds so the
// JSON output is clean text. It is a thin alias for highlight.Strip kept
// for call-site readability at the query layer.
func sanitizeSnippet(s string) string {
	return highlight.Strip(s)
}

// SearchHit is one full-text match.
type SearchHit struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

// Search scans every markdown file under root for term, recency-reranks
// the hits (same ordering as the TUI search modal), and returns at most
// max of them. It uses SearchAll to collect all hits before capping so
// the result is deterministic across runs. A nil store degrades to
// unranked order.
func Search(root, term string, max int) ([]SearchHit, error) {
	paths, err := tree.MarkdownFiles(root)
	if err != nil {
		return nil, err
	}
	hits, err := search.SearchAll(context.Background(), paths, term)
	if err != nil {
		return nil, err
	}

	// A non-nil store is always usable for ranking even if loadStore also
	// returned a diagnostic error (malformed/old state file) — it ranks by
	// mtime with an empty visit map. Only a nil store leaves hits unranked.
	var order func([]string) []string
	if store, _ := loadStore(); store != nil {
		order = store.RankPaths
	}
	hits = search.RerankByRecency(order, hits)
	if max > 0 && len(hits) > max {
		hits = hits[:max]
	}

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
	// Degrade gracefully, matching Search: a malformed/old state file still
	// yields a usable (non-nil) store that ranks by mtime, so we only fail
	// when the store itself is nil (the stateFileFn failure path).
	store, err := loadStore()
	if store == nil {
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
	Kind   string `json:"kind"` // wikilink | relative | external | anchor
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

// outboundLinks maps a vault file's outbound references into Link values.
//
// Std-link classification is delegated to markdown.ResolveLink — the single
// source of truth shared with the vault and TUI — so non-http schemes
// (mailto:, ftp:, protocol-relative, etc.) are recognized as external and
// same-document #anchor targets get their own kind instead of being
// mis-classified as broken relative links.
//
// Broken-ness is determined here, not in the vault accessor:
//   - wikilink: broken when the vault left Resolved empty (existence-based
//     via the names index, so Resolved already implies the file exists).
//   - relative (LinkLocalFile): the vault computes Resolved by pure path
//     math without checking existence, so we os.Stat it via the shared
//     markdown.IsBrokenLocalLink primitive. A missing target is broken and
//     reports an empty Path (matching the spec's broken-relative example).
//   - external / anchor: never broken (we do not probe URLs, and an anchor
//     is intra-document).
func outboundLinks(refs []vault.Outbound, abs string) []Link {
	out := make([]Link, 0, len(refs))
	for _, r := range refs {
		if r.Kind == vault.OutboundWikilink {
			out = append(out, Link{
				Text:   r.DisplayText,
				Path:   r.Resolved,
				Kind:   "wikilink",
				Target: "[[" + r.RawTarget + "]]",
				Broken: r.Resolved == "",
			})
			continue
		}

		// Standard markdown link: classify the raw href once via the
		// shared markdown resolver.
		l := Link{Text: r.DisplayText, Target: r.RawTarget}
		switch resolved := markdown.ResolveLink(abs, r.RawTarget); resolved.Kind {
		case markdown.LinkExternal:
			l.Kind = "external"
		case markdown.LinkAnchor:
			l.Kind = "anchor"
		default: // LinkLocalFile / LinkInvalid
			l.Kind = "relative"
			l.Path = resolved.Target
			if markdown.IsBrokenLocalLink(resolved.Target) {
				l.Path = ""
				l.Broken = true
			}
		}
		out = append(out, l)
	}
	return out
}

// mustExist resolves file against the vault root and returns an absolute
// path, or an error if file does not exist on disk. A relative file
// argument is resolved relative to root (the --vault directory), NOT the
// process cwd — the vault index is keyed by paths under root, so resolving
// against cwd would silently miss every link for a file whose cwd-relative
// path isn't a vault key. A missing file argument is an operational failure
// (exit 1), distinct from a file that simply has zero links.
func mustExist(root, file string) (string, error) {
	abs := file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, file)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("file not found: %s", abs)
	}
	return abs, nil
}

// Links returns the outbound edges from file within the vault at root.
func Links(root, file string) ([]Link, error) {
	abs, err := mustExist(root, file)
	if err != nil {
		return nil, err
	}
	// Fast path: a file's outbound links need only its own parse plus the
	// vault's name index (for wikilink resolution) — not the full reference
	// graph that Build constructs. OutboundFor skips reading every other file.
	refs, err := vault.OutboundFor(root, abs, vault.NopDiagnostics{})
	if err != nil {
		return nil, err
	}
	return outboundLinks(refs, abs), nil
}

// Neighbors returns file's outbound links and its backlinks.
func Neighbors(root, file string) (Neighborhood, error) {
	abs, err := mustExist(root, file)
	if err != nil {
		return Neighborhood{}, err
	}
	v, err := vault.Build(root, vault.NopDiagnostics{})
	if err != nil {
		return Neighborhood{}, err
	}
	// Outbound is always non-nil (outboundLinks returns a made slice);
	// Backlinks is init'd up-front so JSON emits [] not null even with
	// zero backlinks (matching tree.MarkdownFiles' init-then-append style).
	n := Neighborhood{
		File:      abs,
		Outbound:  outboundLinks(v.Outbound(abs), abs),
		Backlinks: []BacklinkEntry{},
	}
	for _, b := range v.Backlinks(abs) {
		n.Backlinks = append(n.Backlinks, BacklinkEntry{
			Path:    b.SourceFile,
			Line:    b.Line,
			Snippet: sanitizeSnippet(b.Snippet),
			Text:    b.DisplayText,
		})
	}
	return n, nil
}

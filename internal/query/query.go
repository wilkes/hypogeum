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

	// Search results re-rank by edit-recency (mtime), matching the TUI
	// search modal and the file finder. This is stateless — no store needed.
	hits = search.RerankByRecency(recent.RankPathsByMTime, hits)
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

// RecentEntry is one visit-ranked note. There is no score field: the
// ordering is a plain descending sort on the visit timestamp.
type RecentEntry struct {
	Path    string    `json:"path"`
	Visited time.Time `json:"visited"`
}

// Recent returns up to max markdown files under root that have been opened in
// hypogeum, ordered by last-visited time descending (most recent first).
// Files never opened are excluded — this is "recently opened," matching the
// TUI r modal, not a recency score over the whole vault.
func Recent(root string, max int) ([]RecentEntry, error) {
	paths, err := tree.MarkdownFiles(root)
	if err != nil {
		return nil, err
	}
	// Degrade gracefully, matching Search: a malformed/old state file still
	// yields a usable (non-nil) store with an empty visit map (which simply
	// returns no entries), so we only fail when the store itself is nil (the
	// stateFileFn failure path).
	store, err := loadStore()
	if store == nil {
		return nil, err
	}
	ranked := store.RankByVisit(paths)
	if max > 0 && len(ranked) > max {
		ranked = ranked[:max]
	}
	out := make([]RecentEntry, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, RecentEntry{
			Path:    r.Path,
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

// Neighbors returns file's outbound links and its backlinks. It builds the
// full forward graph (vault.Build) from cold — a long-lived caller holding a
// warm *vault.Vault should call NeighborsFromVault to skip the rebuild.
func Neighbors(root, file string) (Neighborhood, error) {
	abs, err := mustExist(root, file)
	if err != nil {
		return Neighborhood{}, err
	}
	v, err := vault.Build(root, vault.NopDiagnostics{})
	if err != nil {
		return Neighborhood{}, err
	}
	return neighborhoodFromVault(v, abs), nil
}

// NeighborsFromVault assembles file's neighborhood from an already-built vault,
// skipping the vault.Build that a cold Neighbors call pays. root is the vault
// root v was built from; file resolves against it (relative to root, not cwd —
// matching Neighbors). The output is identical to Neighbors(root, file) for the
// same vault: this is the warm-cache fast path the MCP server uses for repeated
// queries, and the CLI Neighbors above now shares its assembly via
// neighborhoodFromVault so the two paths can't drift.
func NeighborsFromVault(v *vault.Vault, root, file string) (Neighborhood, error) {
	abs, err := mustExist(root, file)
	if err != nil {
		return Neighborhood{}, err
	}
	return neighborhoodFromVault(v, abs), nil
}

// neighborhoodFromVault is the shared assembly for Neighbors / NeighborsFromVault.
// Outbound is always non-nil (outboundLinks returns a made slice); Backlinks is
// init'd up-front so JSON emits [] not null even with zero backlinks (matching
// tree.MarkdownFiles' init-then-append style).
func neighborhoodFromVault(v *vault.Vault, abs string) Neighborhood {
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
	return n
}

// GraphNode is one document in the vault graph.
type GraphNode struct {
	Path string `json:"path"`
}

// GraphEdge is one directed link from a vault file to a target. The target
// may be another vault file (wikilink/relative), an external URL, or a
// same-document anchor; broken internal links carry To:"" and Broken:true.
type GraphEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	Broken bool   `json:"broken"`
}

// Graph is the whole-vault link graph: every markdown document as a node
// (orphans included) and every link as a directed edge.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// linkToEdge re-shapes a classified Link into a graph edge. Internal links
// (wikilink/relative) point at their resolved file path; external links and
// anchors point at the raw target. This is a pure re-shape of outboundLinks'
// output — no new classification logic.
func linkToEdge(from string, l Link) GraphEdge {
	to := l.Target
	if l.Kind == "wikilink" || l.Kind == "relative" {
		to = l.Path // "" when broken, matching l.Broken
	}
	return GraphEdge{From: from, To: to, Kind: l.Kind, Broken: l.Broken}
}

// GraphFor returns the whole-vault link graph rooted at root. Nodes are every
// indexed markdown file sorted by path (orphans included); edges are grouped
// by source file (sorted) preserving each file's document order. It builds the
// full forward graph via vault.Build — not the OutboundFor fast path, which
// parses a single file — because a graph needs every file's edges.
//
// Named GraphFor to avoid colliding with the Graph result type in this package.
func GraphFor(root string) (Graph, error) {
	v, err := vault.Build(root, vault.NopDiagnostics{})
	if err != nil {
		return Graph{}, err
	}
	return GraphFromVault(v), nil
}

// GraphFromVault builds the whole-vault graph from an already-built vault,
// skipping the vault.Build that GraphFor pays from cold. The output is identical
// to GraphFor for the same vault — it's the warm-cache fast path used by the MCP
// server, with GraphFor delegating here so the two paths stay in lockstep.
func GraphFromVault(v *vault.Vault) Graph {
	files := v.Files() // already sorted ascending
	g := Graph{
		Nodes: make([]GraphNode, 0, len(files)),
		Edges: make([]GraphEdge, 0, len(files)),
	}
	for _, f := range files {
		g.Nodes = append(g.Nodes, GraphNode{Path: f})
		for _, l := range outboundLinks(v.Outbound(f), f) {
			g.Edges = append(g.Edges, linkToEdge(f, l))
		}
	}
	return g
}

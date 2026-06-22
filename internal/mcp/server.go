// Package mcp serves a hypogeum vault to agents over the Model Context
// Protocol. It is a third frontend over the same lower layers as the TUI and
// the CLI query verbs — internal/query, internal/vault, internal/watch — and
// has no TUI dependency. Tools are thin wrappers over internal/query so their
// JSON output is identical to the corresponding CLI verb; the one thing the
// server adds is a watcher-refreshed warm vault index (see index.go) that lets
// neighbors/graph reuse a built graph instead of rebuilding from cold per call.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wilkes/hypogeum/internal/query"
	"github.com/wilkes/hypogeum/internal/watch"
)

// Server holds the vault root, the warm index, and the live-refresh watcher.
// Construct with New, serve with Run, and Close when done. The tool handlers
// (handleSearch, etc.) are split from the SDK transport so they can be tested
// directly over a fixture vault without a live stdio pipe.
type Server struct {
	root    string // absolute vault root
	version string // reported as the MCP server implementation version
	idx     *index
	watcher *watch.Watcher
	errOut  io.Writer // diagnostics sink (stderr); stdout is the MCP channel
}

// New constructs a server rooted at the given vault directory. root is made
// absolute (the warm vault is keyed by absolute paths, so a relative root would
// never match a tool's resolved file argument). It starts a best-effort
// watcher: if watch.New fails (e.g. inotify limits), live refresh is disabled
// and a notice is written to stderr, but the server still serves — same
// graceful-degradation rule the TUI follows.
func New(root, version string) (*Server, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", abs)
	}
	s := &Server{
		root:    abs,
		version: version,
		idx:     newIndex(abs),
		errOut:  os.Stderr,
	}
	s.startWatcher()
	return s, nil
}

// startWatcher wires watch events into the warm index. StructureChanged
// rebuilds the graph; FileModified re-parses the touched files in place. Both
// no-op until something warms the cache, so a quiet server pays no refresh cost.
func (s *Server) startWatcher() {
	w, err := watch.New(s.root)
	if err != nil {
		fmt.Fprintf(s.errOut, "hypogeum mcp: live refresh disabled: %v\n", err)
		return
	}
	s.watcher = w
	go func() {
		for ev := range w.Events() {
			switch ev.Kind {
			case watch.StructureChanged:
				s.idx.rebuild()
			case watch.FileModified:
				for _, p := range ev.Paths {
					s.idx.refreshFile(p)
				}
			}
		}
	}()
}

// Close stops the watcher. Safe to call when the watcher never started.
func (s *Server) Close() error {
	if s.watcher != nil {
		return s.watcher.Close()
	}
	return nil
}

// Run registers the tools and serves MCP over stdin/stdout until the context is
// cancelled or the peer disconnects.
func (s *Server) Run(ctx context.Context) error {
	return s.mcpServer().Run(ctx, &mcpsdk.StdioTransport{})
}

// --- tool argument schemas (jsonschema struct tags drive the advertised
// input schema and the per-field descriptions an agent sees) ---

type searchArgs struct {
	Term string `json:"term" jsonschema:"case-insensitive substring to search for across the vault"`
	Max  int    `json:"max,omitempty" jsonschema:"maximum number of hits to return (default 50)"`
}

type fileArgs struct {
	File string `json:"file" jsonschema:"path to a file, relative to the vault root or absolute"`
}

// readNoteResult is the read_note payload: the resolved absolute path plus the
// file's raw contents. It is the deliberate complement to the pointer-only
// query verbs — they answer where, this answers what.
type readNoteResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

const defaultSearchMax = 50

// handleSearch backs search_vault. Stateless full-text scan (no warm index).
func (s *Server) handleSearch(a searchArgs) ([]query.SearchHit, error) {
	if strings.TrimSpace(a.Term) == "" {
		return nil, fmt.Errorf("search_vault: missing query term")
	}
	max := a.Max
	if max <= 0 {
		max = defaultSearchMax
	}
	return query.Search(s.root, a.Term, max)
}

// handleLinks backs outbound_links. Uses query.Links' OutboundFor fast path —
// a single-file parse, so no warm index is needed.
func (s *Server) handleLinks(a fileArgs) ([]query.Link, error) {
	if a.File == "" {
		return nil, fmt.Errorf("outbound_links: missing file argument")
	}
	return query.Links(s.root, a.File)
}

// handleNeighbors backs neighbors. Needs the full forward graph (for
// backlinks), so it reads the warm index instead of rebuilding per call.
func (s *Server) handleNeighbors(a fileArgs) (query.Neighborhood, error) {
	if a.File == "" {
		return query.Neighborhood{}, fmt.Errorf("neighbors: missing file argument")
	}
	v, err := s.idx.get()
	if err != nil {
		return query.Neighborhood{}, err
	}
	return query.NeighborsFromVault(v, s.root, a.File)
}

// handleGraph backs vault_graph. Whole-vault graph from the warm index.
func (s *Server) handleGraph() (query.Graph, error) {
	v, err := s.idx.get()
	if err != nil {
		return query.Graph{}, err
	}
	return query.GraphFromVault(v), nil
}

// handleReadNote backs read_note. Reads a file's raw contents, refusing any
// path that resolves outside the vault root.
func (s *Server) handleReadNote(a fileArgs) (readNoteResult, error) {
	if a.File == "" {
		return readNoteResult{}, fmt.Errorf("read_note: missing file argument")
	}
	abs, err := s.resolveUnderRoot(a.File)
	if err != nil {
		return readNoteResult{}, err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return readNoteResult{}, fmt.Errorf("read_note: %w", err)
	}
	return readNoteResult{Path: abs, Content: string(b)}, nil
}

// resolveUnderRoot resolves file (relative to the vault root, or absolute) and
// rejects any target that escapes the root via "..". This is the containment
// guard for read_note — the query verbs don't need it because they only ever
// surface paths the vault already indexed.
func (s *Server) resolveUnderRoot(file string) (string, error) {
	abs := file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(s.root, file)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(s.root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes vault root: %s", file)
	}
	return abs, nil
}

// mcpServer builds the SDK server with every tool registered. Split from Run so
// dispatch tests can construct the server without serving.
func (s *Server) mcpServer() *mcpsdk.Server {
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "hypogeum",
		Version: s.version,
	}, nil)

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "search_vault",
		Description: "Full-text search the vault for a case-insensitive substring. Returns path + line + snippet pointers, ranked by edit recency. Use read_note to fetch a match's contents.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, a searchArgs) (*mcpsdk.CallToolResult, any, error) {
		return toolResult(s.handleSearch(a))
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "outbound_links",
		Description: "List the outbound links (wikilinks, relative, external, anchor) from a single file, each with its resolved target path and a broken flag.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, a fileArgs) (*mcpsdk.CallToolResult, any, error) {
		return toolResult(s.handleLinks(a))
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "neighbors",
		Description: "Return a file's 1-hop context bundle: its outbound links and its backlinks (who links to it, with line + snippet).",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, a fileArgs) (*mcpsdk.CallToolResult, any, error) {
		return toolResult(s.handleNeighbors(a))
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "vault_graph",
		Description: "Return the whole-vault link graph as {nodes, edges}. Every markdown doc is a node (orphans included); every link is a directed edge with its kind and broken flag.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, _ struct{}) (*mcpsdk.CallToolResult, any, error) {
		return toolResult(s.handleGraph())
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "read_note",
		Description: "Read a file's raw contents by path. Complements the pointer-only search/links/neighbors tools. Refuses paths outside the vault root.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, a fileArgs) (*mcpsdk.CallToolResult, any, error) {
		return toolResult(s.handleReadNote(a))
	})

	return srv
}

// toolResult adapts a handler's (value, error) into the SDK's tool return. The
// value is marshaled to JSON text content — identical to the bytes the
// corresponding CLI verb writes (minus the trailing newline) — so the two
// transports agree. A non-nil error is returned as-is: the SDK packs it into
// the result content with IsError set, so the agent sees the failure and can
// self-correct rather than the call surfacing as a protocol-level error.
func toolResult[T any](v T, err error) (*mcpsdk.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	data, mErr := json.Marshal(v)
	if mErr != nil {
		return nil, nil, mErr
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}, nil, nil
}

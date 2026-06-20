# `graph` Query Verb Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a non-interactive `hypogeum graph [--vault DIR]` CLI verb that emits the whole-vault link graph as JSON (`{nodes, edges}`).

**Architecture:** A new `query.Graph(root)` builds the full forward graph once via `vault.Build`, enumerates every indexed file as a node (orphans included), and re-shapes each file's `outboundLinks` classification into edges. The CLI gains a fourth-and-a-half reserved verb wired exactly like `neighbors`. No new link-classification logic — edges are a pure re-shape of the existing `query.Link`.

**Tech Stack:** Go 1.24, stdlib `encoding/json` + `flag`, existing `internal/vault` + `internal/query` packages. Tests are plain `testing` with `t.TempDir()` fixture vaults.

## Global Constraints

- Go module path: `github.com/wilkes/hypogeum`.
- Query layer (`internal/query`) has **no TUI dependencies** — keep it that way.
- JSON only on stdout, errors on stderr (existing `runQuery` discipline).
- Slices returned from `query` are initialized (`make(...)`) so empty results encode as `[]`, never `null`.
- Paths in query output are **absolute** (matches `links` / `neighbors`).
- Tests live next to the code they test; keep the suite race-clean (`go test -race ./...`).
- Reserved verbs route to query mode *before* path resolution — a verb name never opens a file of that name.
- All work lands on branch `feat/graph-verb` (already created); commit per task.

---

### Task 1: `(*Vault).Files()` accessor

A graph needs every indexed file. The vault has no exported "all files" accessor; add one returning a sorted copy of the `files` map keys under the read lock. Sorting here gives `Graph` its deterministic node order for free.

**Files:**
- Modify: `internal/vault/vault.go` (add method near `fileCount`, ~line 233)
- Test: `internal/vault/vault_test.go` (add test)

**Interfaces:**
- Produces: `func (v *Vault) Files() []string` — sorted (ascending) slice of absolute paths of every indexed markdown file. Returns an empty (non-nil) slice for an empty vault.

- [ ] **Step 1: Write the failing test**

Add to `internal/vault/vault_test.go`:

```go
func TestVaultFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.md", "a.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := v.Files()
	want := []string{
		filepath.Join(dir, "a.md"),
		filepath.Join(dir, "b.md"),
		filepath.Join(dir, "c.md"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Files() = %v, want %v", got, want)
	}
}

func TestVaultFilesEmpty(t *testing.T) {
	v, err := Build(t.TempDir(), NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := v.Files(); got == nil || len(got) != 0 {
		t.Errorf("Files() = %v, want empty non-nil slice", got)
	}
}
```

Ensure `reflect` is imported in the test file (add to the import block if absent).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/vault/ -run 'TestVaultFiles' -v`
Expected: FAIL — `v.Files undefined (type *Vault has no field or method Files)` (compile error).

- [ ] **Step 3: Write minimal implementation**

Add to `internal/vault/vault.go` (after `fileCount`):

```go
// Files returns the absolute paths of every indexed markdown file, sorted
// ascending. The result is a copy — callers may retain or mutate it freely.
// An empty vault yields an empty (non-nil) slice.
func (v *Vault) Files() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]string, 0, len(v.files))
	for p := range v.files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
```

Ensure `sort` is in the `internal/vault/vault.go` import block (add it if absent).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/vault/ -run 'TestVaultFiles' -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/vault/vault.go internal/vault/vault_test.go
git commit -m "feat(vault): add Files() accessor for whole-vault enumeration"
```

---

### Task 2: `query.Graph` and result types

The core. Build the vault once, map every file to a node, and re-shape each file's `outboundLinks` into edges via a small `linkToEdge` helper.

**Files:**
- Modify: `internal/query/query.go` (add types + `Graph` + `linkToEdge` at end of file)
- Test: `internal/query/graph_test.go` (create)

**Interfaces:**
- Consumes: `vault.Build`, `(*vault.Vault).Files()` (Task 1), `(*vault.Vault).Outbound`, existing `outboundLinks(refs []vault.Outbound, abs string) []Link`.
- Produces:
  - `type GraphNode struct { Path string \`json:"path"\` }`
  - `type GraphEdge struct { From string \`json:"from"\`; To string \`json:"to"\`; Kind string \`json:"kind"\`; Broken bool \`json:"broken"\` }`
  - `type Graph struct { Nodes []GraphNode \`json:"nodes"\`; Edges []GraphEdge \`json:"edges"\` }`
  - `func Graph(root string) (Graph, error)`

NOTE: Go forbids a function and type sharing a name in one package (`Neighborhood`/`Neighbors` already dance around this). The result type is `Graph`; the function is therefore named `GraphFor`. The test below already calls `GraphFor` — write it that way from the start.

- [ ] **Step 1: Write the failing test**

Create `internal/query/graph_test.go`:

```go
package query

import (
	"os"
	"path/filepath"
	"testing"
)

// writeGraphVault builds a small vault exercising every edge kind plus an orphan.
func writeGraphVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		// index.md: a resolved wikilink, a broken wikilink, an external URL,
		// a self anchor, and a resolved relative link.
		"index.md": "[[arch]] [[ghost]] [site](https://charm.sh) [top](#intro) [a](./arch.md)\n",
		"arch.md":  "# Arch\n",
		"orphan.md": "# Orphan, no links\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestGraphNodes(t *testing.T) {
	dir := writeGraphVault(t)
	g, err := GraphFor(dir)
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	// Nodes are every md file, orphan included, sorted by path.
	want := []string{
		filepath.Join(dir, "arch.md"),
		filepath.Join(dir, "index.md"),
		filepath.Join(dir, "orphan.md"),
	}
	if len(g.Nodes) != len(want) {
		t.Fatalf("got %d nodes, want %d: %+v", len(g.Nodes), len(want), g.Nodes)
	}
	for i, w := range want {
		if g.Nodes[i].Path != w {
			t.Errorf("Nodes[%d].Path = %q, want %q", i, g.Nodes[i].Path, w)
		}
	}
}

func TestGraphEdges(t *testing.T) {
	dir := writeGraphVault(t)
	g, err := GraphFor(dir)
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	index := filepath.Join(dir, "index.md")
	arch := filepath.Join(dir, "arch.md")

	// All five edges originate from index.md, in document order.
	if len(g.Edges) != 5 {
		t.Fatalf("got %d edges, want 5: %+v", len(g.Edges), g.Edges)
	}
	for i, e := range g.Edges {
		if e.From != index {
			t.Errorf("Edges[%d].From = %q, want index.md", i, e.From)
		}
	}
	// e0: resolved wikilink [[arch]]
	if g.Edges[0].Kind != "wikilink" || g.Edges[0].To != arch || g.Edges[0].Broken {
		t.Errorf("Edges[0] = %+v, want resolved wikilink -> arch.md", g.Edges[0])
	}
	// e1: broken wikilink [[ghost]]
	if g.Edges[1].Kind != "wikilink" || g.Edges[1].To != "" || !g.Edges[1].Broken {
		t.Errorf("Edges[1] = %+v, want broken wikilink with empty To", g.Edges[1])
	}
	// e2: external URL
	if g.Edges[2].Kind != "external" || g.Edges[2].To != "https://charm.sh" || g.Edges[2].Broken {
		t.Errorf("Edges[2] = %+v, want external https://charm.sh", g.Edges[2])
	}
	// e3: self anchor
	if g.Edges[3].Kind != "anchor" || g.Edges[3].To != "#intro" || g.Edges[3].Broken {
		t.Errorf("Edges[3] = %+v, want anchor #intro", g.Edges[3])
	}
	// e4: resolved relative link
	if g.Edges[4].Kind != "relative" || g.Edges[4].To != arch || g.Edges[4].Broken {
		t.Errorf("Edges[4] = %+v, want resolved relative -> arch.md", g.Edges[4])
	}
}

func TestGraphEmptyVault(t *testing.T) {
	g, err := GraphFor(t.TempDir())
	if err != nil {
		t.Fatalf("Graph: %v", err)
	}
	if g.Nodes == nil || g.Edges == nil {
		t.Errorf("Graph on empty vault must init slices, got %+v", g)
	}
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Errorf("empty vault should have no nodes/edges, got %+v", g)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/query/ -run 'TestGraph' -v`
Expected: FAIL — `undefined: Graph` (compile error).

- [ ] **Step 3: Write minimal implementation**

Append to `internal/query/query.go`:

```go
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
	return g, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/query/ -run 'TestGraph' -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Run the full query + vault suites race-clean**

Run: `go test -race ./internal/query/ ./internal/vault/`
Expected: PASS (ok for both packages).

- [ ] **Step 6: Commit**

```bash
git add internal/query/query.go internal/query/graph_test.go
git commit -m "feat(query): GraphFor builds the whole-vault link graph"
```

---

### Task 3: CLI verb wiring

Wire `graph` into the reserved-verb dispatcher. No positional arg, no `-n` — only `--vault`.

**Files:**
- Modify: `cmd/hypogeum/query.go` (`queryVerbs` map ~line 16; `runQuery` switch ~line 62)
- Test: `cmd/hypogeum/query_test.go` (add test)

**Interfaces:**
- Consumes: `query.GraphFor(root string) (query.Graph, error)` (Task 2).

- [ ] **Step 1: Write the failing test**

Add to `cmd/hypogeum/query_test.go` (match the existing test style there — it captures `runQuery` output into a `bytes.Buffer` and unmarshals). If a helper for running a verb already exists, reuse it; otherwise:

```go
func TestRunQueryGraph(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("[[b]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("# b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runQuery([]string{"graph", "--vault", dir}, &buf); err != nil {
		t.Fatalf("runQuery graph: %v", err)
	}

	var g query.Graph
	if err := json.Unmarshal(buf.Bytes(), &g); err != nil {
		t.Fatalf("unmarshal: %v (output: %s)", err, buf.String())
	}
	if len(g.Nodes) != 2 {
		t.Errorf("got %d nodes, want 2: %+v", len(g.Nodes), g.Nodes)
	}
	if len(g.Edges) != 1 || g.Edges[0].Kind != "wikilink" || g.Edges[0].Broken {
		t.Errorf("edges = %+v, want one resolved wikilink", g.Edges)
	}
}

func TestGraphIsQueryVerb(t *testing.T) {
	if !isQueryVerb("graph") {
		t.Error("graph should be a reserved query verb")
	}
}
```

Ensure the test file imports `bytes`, `encoding/json`, `os`, `path/filepath`, and `github.com/wilkes/hypogeum/internal/query` (add any missing).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/hypogeum/ -run 'Graph' -v`
Expected: FAIL — `TestGraphIsQueryVerb` fails (graph not reserved) and `TestRunQueryGraph` errors with `unknown query verb: graph`.

- [ ] **Step 3: Write minimal implementation**

In `cmd/hypogeum/query.go`, add to the `queryVerbs` map:

```go
var queryVerbs = map[string]bool{
	"search":    true,
	"links":     true,
	"recent":    true,
	"neighbors": true,
	"graph":     true,
}
```

Add a case to the `runQuery` switch (alongside the others, before `default`):

```go
	case "graph":
		g, err := query.GraphFor(root)
		if err != nil {
			return err
		}
		result = g
```

(No `-n` registration and no positional read — `graph` ignores any positional, and `-n graph` is already an unknown-flag parse error since `n` is only registered for `search`/`recent`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/hypogeum/ -run 'Graph' -v`
Expected: PASS (both tests).

- [ ] **Step 5: Build and smoke-test against the repo's own docs vault**

Run: `go run ./cmd/hypogeum graph --vault docs | head -c 400; echo`
Expected: a JSON object beginning `{"nodes":[{"path":"/...docs/...md"}...],"edges":[...]}` — absolute paths, no error on stderr.

- [ ] **Step 6: Commit**

```bash
git add cmd/hypogeum/query.go cmd/hypogeum/query_test.go
git commit -m "feat(cli): add reserved graph verb"
```

---

### Task 4: Documentation

Record the new verb in the two human-facing surfaces. The spec + index entry already landed in the spec commit; this task covers `CLAUDE.md` and `README.md`.

**Files:**
- Modify: `CLAUDE.md` (reserved-query-verbs gotcha bullet)
- Modify: `README.md` (verb list / query-mode section)

**Interfaces:** none (docs only).

- [ ] **Step 1: Update CLAUDE.md**

In the **"Reserved query verbs shadow file paths"** gotcha bullet, extend the verb list from `search`/`links`/`recent`/`neighbors` to include `graph`, and append a sentence:

```
`graph` returns the whole-vault link graph as `{nodes, edges}` JSON — every markdown doc is a node (orphans included), every link an edge (`{from, to, kind, broken}`, all four kinds). It builds the full forward graph via `vault.Build`, not the `OutboundFor` fast path, because it needs every file's edges; edges reuse the same `outboundLinks` classifier as `links`/`neighbors`.
```

Update the `queryVerbs` enumeration string in that bullet (`search`/`links`/`recent`/`neighbors`) to `search`/`links`/`recent`/`neighbors`/`graph`.

- [ ] **Step 2: Update README.md**

Find the query-mode / reserved-verbs section (search README for `neighbors`) and add a `graph` entry mirroring the existing format, with an example:

```
- `hypogeum graph [--vault DIR]` — emit the whole-vault link graph as JSON (`{nodes, edges}`). Nodes are every markdown document (orphans included); edges are every link with `{from, to, kind, broken}`. Example: `hypogeum graph --vault docs | jq '.edges | length'`.
```

Match whatever list/heading style the surrounding README uses (verify by reading the section first).

- [ ] **Step 3: Verify the whole suite still builds and passes**

Run: `go build ./... && go test ./...`
Expected: PASS across all packages.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs(graph): document the graph query verb"
```

---

## Self-Review

**Spec coverage:**
- JSON `{nodes, edges}` output → Task 2 (types + `GraphFor`), Task 3 (CLI emits it).
- Nodes = every md doc, orphans included → Task 2 (`v.Files()` over all indexed files; `TestGraphNodes` asserts orphan present).
- Edges = all four kinds, with brokenness → Task 2 (`linkToEdge` + `TestGraphEdges` covers wikilink/relative/external/anchor + broken).
- Absolute paths → inherited from `outboundLinks`/`Files()`; asserted in tests.
- No edge dedup → `GraphFor` appends every `outboundLinks` entry; `TestGraphEdges` expects 5 edges from one file with no collapsing.
- Deterministic order → `Files()` sorts; document order preserved per file; asserted by index-position checks.
- No `-n`, no positional → Task 3 (only `--vault`); smoke test uses `--vault` only.
- Reserved-before-path-resolution → Task 3 adds to `queryVerbs`; `TestGraphIsQueryVerb`.
- `[]` not `null` → Task 2 `make(...)`; `TestGraphEmptyVault`.
- Docs (CLAUDE.md gotcha + README) → Task 4. (index.md + spec already committed.)

**Placeholder scan:** No TBD/TODO; every code step shows complete code; commands have expected output.

**Type consistency:** `GraphFor` (function) / `Graph` (type) split is consistent across Tasks 2–3 and the tests. `GraphNode`/`GraphEdge` field names (`Path`, `From`, `To`, `Kind`, `Broken`) match between definition (Task 2) and assertions (Tasks 2–3). `linkToEdge` consumes the existing `Link` fields (`Kind`, `Path`, `Target`, `Broken`) verified against `query.go`.

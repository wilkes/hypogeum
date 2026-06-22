# MCP Server — Design

**Status:** Implemented (this branch).
**Date:** 2026-06-22
**Branch:** `claude/hypogeum-rpc-server-5u4ksg`

> **Implementation note (2026-06-22).** Shipped as designed: the `mcp` verb
> (`cmd/hypogeum/mcp.go`), the `internal/mcp` package (warm `index` +
> watcher-wired `Server` + five tools), and `query.NeighborsFromVault` /
> `query.GraphFromVault` so the warm index reuses a built vault. The official Go
> MCP SDK (`github.com/modelcontextprotocol/go-sdk` v1.6.1) was reachable and
> pinned — its one cost is a **Go toolchain bump to 1.25** (the module requires
> `go >= 1.25.0`), which CI picks up automatically via `go-version-file: go.mod`.
> End-to-end verified by driving the built binary with a real MCP client over
> stdio. `recent`/visit-state and health tools remain deferred as planned.

## Goal

Expose hypogeum's vault as a [Model Context Protocol](https://modelcontextprotocol.io)
server so Claude and other agents can query a directory of interlinked markdown
files as a first-class tool — full-text search, link traversal, neighborhood
bundles, and the whole-vault graph — without shelling out to the CLI verbs once
per question.

This is the same retrieval job the [scriptable query mode](2026-06-20-scriptable-query-mode-design.md)
already serves, delivered over a long-lived transport instead of process-per-call.
The agent connects once; we keep a **warm vault index** in memory and answer
repeated queries against it.

### Why a server, not "just use the CLI verbs"

The query verbs already emit JSON and are agent-friendly. The one thing they
*can't* do is amortize `vault.Build`. Today every `neighbors`/`graph`
invocation walks and parses the entire vault from cold (`search`/`links` have
fast paths, but `neighbors`/`graph` need the full forward graph). For an agent
that traverses a vault across many turns, that cold rebuild is paid on every
call. A persistent server builds the index once and feeds it incremental
updates through the existing [`internal/watch`](../../packages/watch.md)
package. **That amortization is the entire reason this is a server and not a
shell loop** — if it built per-call, the CLI would already win.

## Non-goals

- **No new query logic.** Every tool is a thin call into `internal/query`. If a
  capability isn't already in `query`, it's out of scope (the lone exception is
  `read_note`, a trivial file read — see below).
- **No wrapping of the TUI.** The TUI is a terminal program; it is not driven
  over RPC. This server is a *third frontend* over the same lower layers
  (`query`/`vault`/`watch`), peer to the TUI and the CLI verbs, consistent with
  the repo's "one package, one job" layering.
- **No remote/HTTP/SSE transport** in this iteration. stdio only — the standard
  for locally-spawned MCP servers (what Claude Desktop/Code launch). A network
  transport can be added later behind the same handlers.
- **No write tools.** Read-only over the vault. No note creation/editing.
- **No `recent` tool** in v1. Visit-recency is user UI state (the `r` modal),
  low value to an agent that has no session-visit history of its own. Trivial to
  add later if a consumer wants it.

## Architecture

### New binary surface: the `mcp` verb

`cmd/hypogeum` gains one reserved verb, dispatched alongside the existing query
verbs (git-style), *before* path resolution:

```
hypogeum mcp [vault]      # serve MCP over stdio; vault defaults to cwd
```

Same collision rule as the query verbs: `mcp` wins over a same-named file at the
dispatch position; `./mcp` reaches a literal file. One binary, no second
artifact to ship or version. Registration in a Claude config is just:

```json
{ "mcpServers": { "hypogeum": { "command": "hypogeum", "args": ["mcp", "/path/to/vault"] } } }
```

### New package: `internal/mcp`

Holds the server: tool registration, argument schemas, the warm-index cache, and
the watcher wiring. Depends only on `query`, `vault`, `watch`, and the MCP SDK —
**no TUI dependency**. `cmd/hypogeum` stays a thin dispatcher that constructs the
server with a root and runs it.

The handlers are split from the transport so they're unit-testable without a live
stdio pipe — call `(*Server).handleSearch(ctx, args)` directly in tests,
mirroring the `model_test.go` / `query` test philosophy.

### Dependency: the official Go MCP SDK

`github.com/modelcontextprotocol/go-sdk` (the official Go implementation). One
new direct dependency. **Before committing to it I will verify the module is
fetchable in this environment's network policy and pin a version;** if it isn't
reachable, that's a blocker to surface, not a thing to work around with a
hand-rolled JSON-RPC loop in v1.

### Vault root fixed at launch

The root is a launch argument, not a per-tool parameter. This is what lets a
single warm index serve every call. Per-call roots would force a cache keyed by
root and re-introduce cold builds on root changes — out of scope. One server
instance, one vault.

File arguments to tools resolve relative to that root (matching `query.mustExist`,
which already resolves a relative `file` against `--vault`, not process cwd).

## The warm index

A small index holder owned by the server:

```go
type index struct {
    mu   sync.RWMutex
    root string
    v    *vault.Vault   // nil until first build; rebuilt on StructureChanged
}
```

- **Lazy first build.** Built on the first tool call that needs it (or eagerly on
  startup — decided in the plan; lazy keeps startup instant and tolerates a
  vault that isn't fully present yet).
- **Refresh via `watch`.** The server starts a `watch.Watcher` on the root. Its
  debounced, markdown-aware events drive refresh:
  - `StructureChanged` → rebuild the index (`vault.Build`).
  - `FileModified` → `vault.RefreshFile` for the one path (the accessor built for
    exactly this; it uses its own fresh goldmark parser per the vault concurrency
    contract).
  Same best-effort posture as the TUI: if `watch.New` fails (inotify limits), the
  server logs once and runs without live refresh rather than refusing to start.
- **Read/write discipline.** Tool calls take `RLock`; refresh takes `Lock`. This
  is the one piece of genuinely new concurrency in the design. The vault's own
  build-time invariants (map writes serialized by `v.mu`, order-independent
  results) are unchanged — we're only guarding the *swap* of the whole `*Vault`
  pointer and `RefreshFile` mutations against concurrent readers.

### How tools use the index

| MCP tool | Backs onto | Index use |
|---|---|---|
| `search_vault(term, max?)` | `query.Search` | none — pure file scan; no warm index needed |
| `outbound_links(file)` | `query.Links` | none — uses `vault.OutboundFor` fast path (single-file parse) |
| `neighbors(file)` | `query.Neighbors` | **warm** — needs the full forward graph for backlinks |
| `vault_graph()` | `query.GraphFor` | **warm** — whole-vault `{nodes, edges}` |
| `read_note(file)` | *new, trivial* | none — raw file read, resolved via `query.mustExist`-style logic |

`search` and `links` deliberately keep their existing fast paths — wrapping them
in the warm index would be slower, not faster. The warm index pays off precisely
where `query` does a full `vault.Build` today: `neighbors` and `graph`.

> **Refactor note:** `query.Neighbors`/`query.GraphFor` currently call
> `vault.Build` internally. To let them reuse the warm `*Vault`, factor the
> post-build assembly into `query` functions that take a `*vault.Vault`
> (e.g. `NeighborsFromVault(v, abs)`), with the existing `Neighbors(root, file)`
> kept as a thin wrapper that builds then delegates. The CLI path is unchanged;
> the server passes its cached vault. This keeps the JSON output identical
> between CLI and MCP (the same invariant the `OutboundFor`/`Build` lockstep test
> already guards for `links`).

## Tool schemas

All tools return their result as JSON text content (the same structs
`internal/query` already marshals), so the MCP output is byte-identical to the
corresponding CLI verb — one schema, two transports.

- **`search_vault`** — `{ "term": string, "max"?: int }` → `query.SearchHit[]`
  (`{path, line, snippet}`). `max` defaults to 50.
- **`outbound_links`** — `{ "file": string }` → `query.Link[]`
  (`{text, target, path, kind, broken}`).
- **`neighbors`** — `{ "file": string }` → `query.Neighborhood`
  (`{file, outbound[], backlinks[]}`).
- **`vault_graph`** — `{}` → `query.Graph` (`{nodes[], edges[]}`).
- **`read_note`** — `{ "file": string }` → `{ "path": string, "content": string }`
  raw markdown. The deliberate complement to the pointer-only query verbs: the
  query tools answer *where*, `read_note` answers *what*, so an agent never has
  to fall back to an out-of-band file read for vault content. Restricted to paths
  under the vault root (no `../` escape).

Each tool gets a description tuned for agent selection ("Search the vault for a
case-insensitive substring; returns path + line + snippet pointers, recency
ranked"), so the model picks the right one without trial and error.

## Errors

- A bad argument (missing `file`, file-not-found, term empty) returns an MCP tool
  error result with a plain message — the agent sees the failure and can correct,
  rather than the server crashing.
- Operational/transport failures (can't bind stdio, watcher dies) are logged to
  stderr; stdout is the MCP channel and stays protocol-only.
- Empty results are success: `[]` / empty fields, never an error — matching the
  query verbs.

## Testing (TDD)

- `internal/mcp/*_test.go` — drive each handler directly over a temp fixture
  vault; assert the JSON struct values (reuse the `query` fixtures). No live
  stdio, no TTY.
- **Concurrency:** a `-race` test that fires concurrent `neighbors`/`graph` reads
  while a simulated `FileModified`/`StructureChanged` refresh runs, asserting no
  race and a consistent post-refresh result. CI gates on `go test -race ./...`,
  so this must be clean.
- **CLI/MCP lockstep:** assert `read`-free verbs (`neighbors`, `graph`) produce
  the same JSON whether routed through the CLI (`vault.Build` per call) or the
  warm-index server path, extending the existing `OutboundFor`-vs-`Build`
  identity discipline to the refactored `*FromVault` helpers.
- `cmd/hypogeum` — dispatch test: `mcp` routes to the server constructor (without
  actually serving), bare path still routes to the TUI.

## Rollout

1. Verify + pin the Go MCP SDK dependency (blocker check).
2. Refactor `query.Neighbors`/`GraphFor` to expose `*FromVault` variants
   (no behavior change; existing tests stay green).
3. `internal/mcp`: index holder + watcher wiring (`-race` tested) → tool handlers
   → stdio server.
4. `cmd/hypogeum`: `mcp` verb dispatch + `--help` entry.
5. Docs: update `docs/index.md`, the `internal/query`/architecture notes, and add
   a registration snippet to `README.md`.

## Open questions

- **Eager vs. lazy first build** — lazy keeps startup instant; eager warms before
  the first `neighbors`. Lean lazy; revisit if first-call latency matters.
- **`recent`/visit state** — left out of v1 deliberately. Add a `recently_opened`
  tool only if a consumer asks; it would read the same `recent.Store` the CLI
  `recent` verb and `r` modal use.
- **Health tools** (broken-link / orphan sweeps) — the data falls out of
  `vault_graph` already (`broken:true` edges, zero-degree nodes), so an agent can
  derive them. A dedicated `vault_health` tool is a natural follow-up but not in
  this iteration.

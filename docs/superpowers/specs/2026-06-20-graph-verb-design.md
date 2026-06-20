# `graph` query verb — design

Status: approved, ready for implementation plan.

## Goal

Add a non-interactive CLI verb that emits the **whole-vault link graph** as
JSON: every markdown document as a node, every link as an edge. This rounds out
the existing query family (`search`, `links`, `recent`, `neighbors`) with the
one view that is inherently about the vault as a whole rather than a single
file.

```sh
hypogeum graph [--vault DIR]
```

## Output

JSON object on stdout, errors on stderr (same discipline as the other verbs):

```json
{
  "nodes": [
    {"path": "/abs/docs/architecture.md"},
    {"path": "/abs/docs/orphan.md"}
  ],
  "edges": [
    {"from": "/abs/docs/index.md", "to": "/abs/docs/architecture.md", "kind": "wikilink", "broken": false},
    {"from": "/abs/docs/index.md", "to": "https://charm.sh", "kind": "external", "broken": false},
    {"from": "/abs/docs/index.md", "to": "", "kind": "wikilink", "broken": true}
  ]
}
```

## Decisions

- **Nodes** = every `.md` file under the vault root, **orphans included** (files
  with no inbound or outbound links still appear). One field per node: `path`.
  Nodes are *documents only* — external URLs and `#anchor` targets never become
  nodes, even though edges may point at them.
- **Edges** = **all four link kinds** (`wikilink`, `relative`, `external`,
  `anchor`). The `to` field is:
  - resolved **absolute file path** for `wikilink` / `relative`,
  - the **URL string** for `external`,
  - the **`#anchor`** for `anchor` (same-document).
- **Broken internal links** are still emitted as edges, with `to: ""` and
  `broken: true`. External and anchor edges are never broken (`broken: false`)
  — we don't probe URLs, and an anchor is intra-document. This matches
  `outboundLinks`' existing brokenness rules exactly.
- **Paths are absolute**, matching `neighbors` and `links` output (`mustExist`
  resolves to abs; `Neighborhood.File` and resolved link `Path`s are abs).
- **No edge dedup.** A file that links the same target twice yields two edges —
  consistent with how `links` lists each link occurrence rather than collapsing
  them.
- **Deterministic output.** `nodes` sorted by `path`. `edges` grouped by sorted
  `from`, preserving each source file's document order within its group (the
  order `vault.Outbound` already returns). This leans on the vault's existing
  total-order resolution guarantees, so worker scheduling during `vault.Build`
  cannot change the output.
- **No `-n` cap and no positional arg.** A graph is complete or it isn't, and it
  is whole-vault by nature. Only `--vault` applies. Passing a positional or `-n`
  is a usage error (the flag/arg simply isn't registered for this verb).

## Implementation

Three touches, mirroring the established add-a-verb pattern:

1. **`internal/query/query.go` — `Graph(root string)`**
   - One `vault.Build(root, NopDiagnostics{})` (the full forward graph — *not*
     the `OutboundFor` fast path, which parses a single file; a graph needs
     every file's edges).
   - Enumerate all vault files. The vault currently has no exported "all files"
     accessor; add a small one (e.g. `(*Vault).Files() []string` returning a
     sorted copy of the `files` map keys, lock-held) rather than reaching into
     internals from `query`. This keeps the package boundary clean.
   - For each file, call the existing `outboundLinks(v.Outbound(abs), abs)`
     helper to classify edges — the single source of truth for kind + broken
     already shared by `links` and `neighbors`. Map each `Link` into a graph
     edge `{from, to, kind, broken}` where `to` is `Link.Path` for resolved
     internal links, else `Link.Target` (URL) / the anchor.
   - Return a `Graph` struct: `{Nodes []GraphNode, Edges []GraphEdge}`, both
     slices initialized so empty vaults emit `[]` not `null`.

2. **`cmd/hypogeum/query.go`**
   - Add `"graph": true` to `queryVerbs`.
   - Add `case "graph":` in `runQuery` — no positional, no `-n`; call
     `query.Graph(root)`.

3. **Tests + docs**
   - `internal/query/*_test.go`: a `Graph` test over a small fixture vault
     asserting node set (incl. orphan), edge kinds (all four), broken edge,
     absolute paths, and deterministic ordering. An empty-vault test for the
     `[]`-not-`null` contract.
   - `cmd/hypogeum/query_test.go`: verb routing + JSON shape.
   - Update the reserved-query-verbs gotcha in `CLAUDE.md` and the verb list in
     `README.md` to include `graph`.

## Edge `to` mapping (precise)

Given a classified `query.Link`:

| Link.Kind  | Link.Broken | edge `to`        | edge `broken` |
|------------|-------------|------------------|---------------|
| wikilink   | false       | `Link.Path`      | false         |
| wikilink   | true        | `""`             | true          |
| relative   | false       | `Link.Path`      | false         |
| relative   | true        | `""`             | true          |
| external   | false       | `Link.Target`    | false         |
| anchor     | false       | `Link.Target`    | false         |

This is a pure re-shape of `outboundLinks` output; no new classification logic.

## Out of scope

- Visual export formats (DOT, Mermaid). The JSON is the programmable substrate;
  conversion is a downstream concern. Revisit only if a concrete need appears.
- A TUI graph view. This verb is non-interactive only.
- Edge weights, transitive closure, connected-component analysis. The verb
  returns the raw graph; analysis belongs to whatever consumes the JSON.

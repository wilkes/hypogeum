# Scriptable Query Mode — Design

**Status:** Approved, ready for implementation plan.
**Date:** 2026-06-20
**Branch:** `feat/scriptable-query-mode`

## Goal

Make hypogeum useful *outside* the interactive TUI by adding a non-interactive
query mode that emits JSON to stdout and exits. The primary consumer is an LLM
agent (Claude Code) working alongside the user: instead of the user pulling up
the TUI to look something up mid-session, the agent runs a query command and
reads the answer directly.

The job these commands share is **pulling the right vault context into a
session** — retrieval, not vault health/CI. Four capabilities, in priority
order:

- **Full-text search** — locate the note(s) relevant to the conversation.
- **Outbound links** — follow the trail out from a starting note.
- **Recency** — re-establish "what was I recently working on."
- **Neighborhood** — a note's 1-hop links + backlinks as a context bundle.

Output is **pointers, not content**: paths, line numbers, and short snippets.
The agent already has a `Read` tool, so hypogeum answers *where* and the agent
handles *what*. This keeps output small, stable, and predictable to parse.

## Non-goals

- No inline file content in output (pointers only).
- No vault-health/CI commands (broken-link reports, orphan detection) in this
  iteration. (The data is available — `broken:true` falls out of `links` — but
  no dedicated health verb or failing exit code is in scope.)
- No `--format text` / human-formatted output yet. JSON is the only format.
- No changes to TUI behavior.

## Architecture

### New pure package: `internal/query`

Orchestrates the existing pure packages (`search`, `vault`, `recent`,
`markdown`) and returns JSON-tagged result structs. No TUI dependencies; fully
unit-testable without a terminal, mirroring the `model_test.go` philosophy.

`query` is a pure orchestrator — it assembles output structs from the lower
layers and sanitizes them for JSON. It does not parse markdown or walk the
filesystem itself beyond what the packages it calls already do.

### Dispatch in `cmd/hypogeum`

`run()` gains one step before path resolution:

- If `args[0]` is a **reserved verb** (`search`, `links`, `recent`,
  `neighbors`), route to query mode → marshal JSON to stdout → exit.
- Otherwise, the existing path-resolution → TUI flow is untouched.

This is the git-style dispatch model. `cmd/hypogeum` stays a thin dispatcher;
all query logic lives in `internal/query`.

**Verb collision rule:** a reserved verb always wins over a same-named file at
the dispatch position. Documented in `--help`. A user who genuinely wants to
open a file named `search` can pass `./search`, which is a path, not a verb.

### Vault root and path args

- Vault root defaults to **cwd**; `--vault <dir>` overrides.
- Path arguments (e.g. `foo.md`) resolve relative to cwd.

## The one pure-package change: `Vault.Outbound`

`internal/vault` exposes `Backlinks()` (who links *to* me) but not outbound
links — the `refs` are private. Both `links` and `neighbors` need outbound
edges, so we add a symmetric exported accessor:

```go
type Outbound struct {
    DisplayText string
    RawTarget   string // the raw link target as written (e.g. "[[bar]]" or "./bar.md")
    Resolved    string // absolute path, or "" if unresolved (broken)
    Line        int
    Snippet     string
    Kind        OutboundKind // wikilink | stdlink
}

func (v *Vault) Outbound(path string) []Outbound
```

It surfaces the already-indexed private `refs` for a file — no new parsing, no
TUI behavior change. It belongs in `vault` (not `query`) because wikilinks are
invisible to `markdown.ExtractLinks` (they are parsed by vault's own goldmark
extension), and the vault is the only place holding *resolved* wiki + std edges
together. Output ordering is stable (document order, matching `Backlinks`'
stability guarantee).

The `kind` reported to the CLI maps `OutboundKind` plus the resolved/target
shape to one of `wikilink | relative | external`:

- `external` — `Resolved == ""` **and** `RawTarget` has an http/https scheme.
- `wikilink` — `Kind == wikilink`.
- `relative` — `Kind == stdlink` with a non-URL target.

`broken` is `true` when the link is local (`wikilink`/`relative`) and
`Resolved == ""`. External links are never "broken" (we don't probe URLs).

## Command surface & JSON schemas

All commands emit JSON to stdout, newline-terminated, then exit 0. Array
outputs are empty `[]` when there are no results (not an error).

### `hypogeum search "term" [-n 50] [--vault dir]`

Full-text substring scan across every vault markdown file, recency-reranked via
`recent.Rank` (same ordering as the TUI search modal). `-n` caps hits
(default 50).

```json
[{"path":"/abs/notes/foo.md","line":12,"snippet":"…plain text, markers stripped…"}]
```

Snippets have the `highlight.Open`/`highlight.Close` control chars
(`\x11`/`\x12`) stripped before marshaling.

### `hypogeum links foo.md [--vault dir]`

Outbound edges from `foo.md`:

```json
[{"text":"Foo","target":"[[bar]]","path":"/abs/notes/bar.md","kind":"wikilink","broken":false},
 {"text":"site","target":"https://x.com","path":"","kind":"external","broken":false}]
```

- `kind` ∈ `wikilink | relative | external`
- `path` is the resolved absolute path, or `""` for external/broken
- `broken:true` when a local target doesn't resolve

### `hypogeum recent [-n 20] [--vault dir]`

Recency-ranked notes from the persisted visit store
(`recent.DefaultStateFile`), so output reflects real session history. `-n` caps
results (default 20).

```json
[{"path":"/abs/notes/foo.md","score":8.42,"mtime":"2026-06-19T10:00:00Z","visited":"2026-06-18T22:00:00Z"}]
```

`visited` is omitted or zero-valued (`"0001-01-01T00:00:00Z"`) for never-visited
files. Timestamps are RFC 3339 / `time.Time` default JSON encoding.

### `hypogeum neighbors foo.md [--vault dir]`

1-hop context bundle. Outbound reuses the `links` logic; backlinks reuse
`vault.Backlinks` (which already carries line + snippet).

```json
{"file":"/abs/notes/foo.md",
 "outbound":[{"path":"/abs/notes/bar.md","text":"Bar","kind":"wikilink","broken":false}],
 "backlinks":[{"path":"/abs/notes/baz.md","line":7,"snippet":"…ref context…","text":"Foo"}]}
```

## Errors and exit codes

- **`0`** — success, including zero results (empty `[]` / empty fields).
- **`1`** — operational failure (file not found, vault unreadable, bad flag),
  with a plain message to **stderr**. stdout stays clean JSON-only so output is
  always safe to pipe.

## Testing (TDD)

- `internal/query/*_test.go` — table tests over a temp fixture vault asserting
  exact JSON struct values for each verb. No TTY.
- `internal/vault` — a test for the new `Outbound` accessor (wiki + std links,
  resolved + broken, stable ordering).
- `cmd/hypogeum` — dispatch tests: reserved verb → query path, bare path → TUI
  path (without actually launching the program), unknown flag → exit 1.
- Keep the suite race-clean (`go test -race ./...`) per CI gate.

## Open questions

None blocking. Future iterations could add `--format text`, a `backlinks` verb
(currently only reachable via `neighbors`), and vault-health commands with
failing exit codes for CI.

---
name: hypogeum-vault
description: Use when exploring or auditing a directory of interlinked markdown files — a vault with [[wikilinks]] or a cross-linked docs/ tree. Query the link graph (neighbors, backlinks, outbound links, full-text search, whole-vault graph export) and audit link health (broken links, orphan notes) with hypogeum's query CLI instead of grep. Only when hypogeum is installed and the directory is actually a linked vault.
---

# Navigating a markdown vault with hypogeum

`hypogeum` is a terminal markdown-vault browser with a non-interactive query
CLI. When you're working in a **vault** — a directory of markdown files that
link to each other via `[[wikilinks]]` or relative `[text](other.md)` links —
its query verbs answer graph questions that `grep` cannot: what links *to* this
note, what a file references, where a concept is discussed, which notes are
orphans.

## When to use this

Use hypogeum's query verbs when **both** are true:

1. **The directory is a linked vault** — it contains `[[wikilinks]]`, or a
   `docs/`-style tree whose files cross-reference each other with relative
   links. (A lone README or a flat folder of unrelated notes is not a vault.)
2. **`hypogeum` is available** (see next section).

Reach for it for: "what links to X", "what does Y reference", "where is concept
Z discussed", "are there broken links / orphan notes".

**When NOT to use it — prefer `grep`/ripgrep:** a single file; a flat folder
with no link structure; an exact-string hunt across source code; or any case
where the link graph adds nothing over a substring match.

## Is hypogeum available?

```bash
command -v hypogeum
```

If it prints a path, use it. If not:

- Install it: `go install github.com/wilkes/hypogeum/cmd/hypogeum@latest`
  (puts `hypogeum` on `$GOBIN`/`$PATH`).
- Or, inside a checkout: `go build -o /tmp/hypo ./cmd/hypogeum` and use `/tmp/hypo`.
- If Go isn't available or the user declines, **fall back to `grep`/ripgrep** —
  don't block work on the binary.

## If the MCP server is registered, prefer its tools

`hypogeum` can also run as an MCP server (`hypogeum mcp [vault]`) exposing the
same query surface as tools. **If those tools are present in your session, use
them instead of the CLI** — they answer the identical questions over a warm,
watcher-refreshed vault index (no per-call rebuild), and you skip the shell +
`jq` plumbing. The mapping is 1:1:

| MCP tool | CLI equivalent |
|----------|----------------|
| `search_vault(term, max?)` | `search "<term>" [-n N]` |
| `outbound_links(file)` | `links <file>` |
| `neighbors(file)` | `neighbors <file>` |
| `vault_graph()` | `graph` |
| `read_note(file)` | (read the file directly) |

The tools' JSON output is identical to the verbs' (so the audit/triage logic
below applies unchanged), and `file` arguments are still vault-relative — the
server is launched with a fixed vault root, so pass `index.md`, not the verb's
`--vault` flag. There is no MCP equivalent of `recent` (visit history is
TUI/CLI-only). **The CLI is the fallback** whenever the tools aren't registered
in your session — a skill can't add them mid-session, so everything below stays
the primary, zero-config path.

## The one gotcha that bites: `--vault` paths are vault-relative

With `--vault <root>`, every file argument resolves **relative to the vault
root**, not your shell's cwd:

```bash
hypogeum neighbors --vault docs docs/index.md   # WRONG → file not found: docs/docs/index.md
hypogeum neighbors --vault docs index.md        # RIGHT
```

Other essentials:
- JSON is written to **stdout**; errors go to **stderr**. Pipe stdout to `jq`.
- Snippet text from `search` is wrapped in `\x11`/`\x12` control bytes (the
  match highlight). Strip them for display:
  `jq -r '.snippet | gsub("[\\u0011\\u0012]";"")'` or `tr -d '\021\022'`.

## The five verbs

All take `--vault <root>`. The first four take a vault-relative file/query
argument; `graph` takes **no positional** — it's whole-vault.

| Verb | Question | Output (JSON) | Backed by |
|------|----------|---------------|-----------|
| `neighbors <file>` | full 1-hop context of a note | `{file, outbound[], backlinks[]}` | full `vault.Build` |
| `links <file>` | what this file links *out* to | `[{text, target, path, kind, broken}]` | `OutboundFor` fast path |
| `search "<term>"` | where a phrase appears | `[{path, line, snippet}]` (recency-ranked) | substring scan |
| `recent` | notes you've *opened* lately | `[{path, visited}]` (visited-only, newest first) | visit history |
| `graph` | the **whole** vault link graph | `{nodes:[{path}], edges:[{from,to,kind,broken}]}` | full `vault.Build` |

`kind` is one of `wikilink` / `relative` / `external` / `anchor`. `broken` is
true when a `wikilink`/`relative` target doesn't resolve in the vault.
`recent` is *visit*-recency (what you read in the TUI), distinct from edit
(mtime) recency — it only lists files you've actually opened.

`graph` is the only verb with no file argument — it emits **every** markdown doc
as a node (orphans included, sorted by path) and **every** link as a directed
edge. Node `path`, edge `from`, and resolved edge `to` are **absolute**
filesystem paths (same as the `path` field on `links`/`neighbors` output) — not
vault-relative like the input args. `to` is the *resolved* path for
`wikilink`/`relative` edges (and `""` when `broken`), or the *raw target* for
`external` URLs and same-document `anchor`s. It's the one-shot way to audit the
whole vault — no `find | while read` loop needed.

### Examples

```bash
# Full neighborhood of a note (counts):
hypogeum neighbors --vault docs index.md | jq '{outbound:(.outbound|length), backlinks:(.backlinks|length)}'

# What does this file reference, grouped by kind:
hypogeum links --vault docs architecture.md | jq -r 'group_by(.kind)[] | "\(.[0].kind): \(length)"'

# Where is a concept discussed (unique files):
hypogeum search --vault docs "proximity tiebreaker" | jq -r '.[].path' | sort -u

# Whole-vault graph shape (node + edge counts):
hypogeum graph --vault docs | jq '{nodes:(.nodes|length), edges:(.edges|length)}'
```

## Explore recipes

- **Trace a concept's neighborhood.** Start at the note, read its strongest
  edges, follow them:
  ```bash
  hypogeum neighbors --vault docs concepts/vault-index.md \
    | jq -r '"OUT:", (.outbound[] | "  \(.kind) \(.target)"), "IN:", (.backlinks[] | "  \(.path)")'
  ```
- **"Where is X discussed?"** — full-text to a file set:
  ```bash
  hypogeum search --vault docs "RankByVisit" | jq -r '.[].path' | sort | uniq -c | sort -rn
  ```
- **"What depends on this doc?"** — who points *in* (backlinks):
  ```bash
  hypogeum neighbors --vault docs packages/recent.md | jq -r '.backlinks[].path'
  ```

## Audit recipes — whole-vault sweeps

`graph` does the whole-vault sweep in **one** call — prefer it over a
`find | while read` loop (the loop re-builds the name index per file; `graph`
builds it once). The loop form still works on older binaries without `graph`, so
it's kept below as a fallback. **A sweep surfaces candidates, not verdicts —
always triage the output.**

### Broken-link sweep

One shot — every broken edge across the vault, as `from -> to [kind]`:

```bash
hypogeum graph --vault docs \
  | jq -r '.edges[] | select(.broken) | "\(.from) -> \(.to) [\(.kind)]"' \
  | sed 's|.*/docs/||g'   # paths are absolute; strip for readable output
```

(Broken internal edges carry `to:""`, so use `.from`/`.kind` for the report.)

Fallback (no `graph` verb): loop `links` per file —

```bash
find docs -name '*.md' | while read -r f; do rel="${f#docs/}"
  hypogeum links --vault docs "$rel" \
    | jq -r --arg F "$rel" '.[] | select(.broken) | "\($F) -> \(.target) [\(.kind)]"'
done
```

Triage each candidate — not every `broken == true` is a defect:
- **Syntax example.** A doc *about* linking that contains a literal `[[Name]]`
  or `[text](path.md)` as an illustration. False positive — leave it.
- **Cross-vault link.** A relative link whose target lives *outside* the vault
  root (e.g. `../../.claude/.../memory/note.md`). Broken from the vault's view
  but intentional — the file exists, just not in this vault.
- **Dated-filename mismatch.** A `[[concept]]` wikilink that can't resolve
  because the file is `2026-..-concept.md` (the basename stem includes the date
  prefix). A genuine dead link — fix the wikilink or rename.
- **Real dead link.** Target was moved/deleted. Fix it.

### Orphan finder (notes nothing links to)

One shot — nodes that are no edge's resolved `to` (i.e. nothing points in):

```bash
hypogeum graph --vault docs | jq -r '
  (.edges | map(.to) | unique) as $targets
  | .nodes[].path | select(. as $p | ($targets | index($p)) | not)' \
  | sed 's|.*/docs/||' | sort
```

Fallback (no `graph` verb): loop `neighbors` per file —

```bash
find docs -name '*.md' | while read -r f; do rel="${f#docs/}"
  hypogeum neighbors --vault docs "$rel" \
    | jq -r 'select((.backlinks|length)==0) | .file'
done | sed 's|.*/docs/||' | sort
```

Triage: **leaf plans** (`superpowers/plans/*.md`) are commonly unreferenced by
design — nothing links *to* a plan. The vault root (`index.md`) will appear
here if nothing links to it yet, but typically accumulates backlinks as the
vault matures. Filter out the expected cases; what's left ("an unexpectedly
disconnected note") is the real signal. To exclude plan leaves:

```bash
... | grep -vE '^index\.md$|^superpowers/plans/'
```

## Installing globally

This skill lives in the hypogeum repo (`.claude/skills/hypogeum-vault/`) so it's
version-controlled with the tool and dogfoods itself against `docs/`. To make it
fire in *any* repo you work in, symlink (or copy) it into your user skills dir:

```bash
mkdir -p ~/.claude/skills   # ln won't create the parent dir; without this it errors
ln -s "$PWD/.claude/skills/hypogeum-vault" ~/.claude/skills/hypogeum-vault
```

A symlink keeps it in sync with the repo; a copy pins a snapshot. (If `~/.claude/skills/`
doesn't exist yet, `ln -s` fails with `No such file or directory` pointing at the *link*
path — the `mkdir -p` avoids that.)

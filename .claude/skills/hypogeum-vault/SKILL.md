---
name: hypogeum-vault
description: Use when exploring or auditing a directory of interlinked markdown files — a vault with [[wikilinks]] or a cross-linked docs/ tree. Query the link graph (neighbors, backlinks, outbound links, full-text search) and audit link health (broken links, orphan notes) with hypogeum's query CLI instead of grep. Only when hypogeum is installed and the directory is actually a linked vault.
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

## The four verbs

All take `--vault <root>`; the file/query argument is vault-relative.

| Verb | Question | Output (JSON) | Backed by |
|------|----------|---------------|-----------|
| `neighbors <file>` | full 1-hop context of a note | `{file, outbound[], backlinks[]}` | full `vault.Build` |
| `links <file>` | what this file links *out* to | `[{text, target, path, kind, broken}]` | `OutboundFor` fast path |
| `search "<term>"` | where a phrase appears | `[{path, line, snippet}]` (recency-ranked) | substring scan |
| `recent` | notes you've *opened* lately | `[{path, visited}]` (visited-only, newest first) | visit history |

`kind` is one of `wikilink` / `relative` / `external` / `anchor`. `broken` is
true when a `wikilink`/`relative` target doesn't resolve in the vault.
`recent` is *visit*-recency (what you read in the TUI), distinct from edit
(mtime) recency — it only lists files you've actually opened.

### Examples

```bash
# Full neighborhood of a note (counts):
hypogeum neighbors --vault docs index.md | jq '{outbound:(.outbound|length), backlinks:(.backlinks|length)}'

# What does this file reference, grouped by kind:
hypogeum links --vault docs architecture.md | jq -r 'group_by(.kind)[] | "\(.[0].kind): \(length)"'

# Where is a concept discussed (unique files):
hypogeum search --vault docs "proximity tiebreaker" | jq -r '.[].path' | sort -u
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

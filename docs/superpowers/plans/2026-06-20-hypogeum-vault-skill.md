# hypogeum-vault skill — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Also consult **superpowers:writing-skills** for SKILL.md frontmatter/description conventions before Task 1.

**Goal:** Ship `.claude/skills/hypogeum-vault/SKILL.md` — an agent-facing skill that teaches exploring and auditing a markdown vault with hypogeum's query CLI instead of grep.

**Architecture:** A single SKILL.md (playbook form: prose + copy-pasteable recipes, no bundled scripts). Proactive-but-gated trigger encoded in the `description` frontmatter. Every documented command is verified against this repo's own `docs/` vault during authoring. A short global-install pointer is added to `README.md`.

**Tech Stack:** Markdown (the skill), the existing `hypogeum` query CLI (`search`/`links`/`recent`/`neighbors`), `jq`, POSIX `find`/`while`. No Go code changes.

## Global Constraints

- **No hypogeum code changes.** Skill + docs only. `go build ./...` and `go test ./...` must remain unaffected (they don't touch `.claude/` or `docs/`).
- **Vault-relative paths.** With `--vault <root>`, file args resolve relative to the vault root. Always pass `index.md`, never `docs/index.md`, when `--vault docs`.
- **JSON on stdout, errors on stderr.** Snippet highlights carry `\x11`/`\x12` control bytes; strip for display.
- **Playbook form only.** No `.sh` scripts in the skill dir — whole-vault sweeps are inline recipes.
- **Build the binary for verification** with `go build -o /tmp/hypo ./cmd/hypogeum`; use `/tmp/hypo` in verification commands (the skill text references `hypogeum` as the installed name).
- **Branch:** all work on `feat/hypogeum-vault-skill`. Commit messages end with the repo's `Co-Authored-By:` / `Claude-Session:` trailers.

---

### Task 1: Scaffold SKILL.md — frontmatter, gating, availability, the gotcha

**Files:**
- Create: `.claude/skills/hypogeum-vault/SKILL.md`

**Interfaces:**
- Produces: the skill file with frontmatter (`name: hypogeum-vault`, gated `description`) and the first three body sections. Later tasks append the verbs reference, recipes, and global-install section to this same file.

- [ ] **Step 1: Build the binary for verification**

Run: `go build -o /tmp/hypo ./cmd/hypogeum && /tmp/hypo --version`
Expected: prints `hypogeum devel ...` (build succeeds).

- [ ] **Step 2: Confirm the path-doubling gotcha is real (so the doc is accurate)**

Run: `/tmp/hypo neighbors --vault docs docs/index.md; echo "---"; /tmp/hypo neighbors --vault docs index.md | jq '.file'`
Expected: the first form errors to stderr with `file not found: .../docs/docs/index.md`; the second prints the absolute path to `docs/index.md`. This proves the "vault-relative" rule.

- [ ] **Step 3: Write the scaffold**

Create `.claude/skills/hypogeum-vault/SKILL.md` with exactly this content:

````markdown
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
````

- [ ] **Step 4: Verify the file is well-formed and the gate is in the description**

Run: `head -4 .claude/skills/hypogeum-vault/SKILL.md && grep -c "Only when hypogeum is installed and the directory is actually a linked vault" .claude/skills/hypogeum-vault/SKILL.md`
Expected: frontmatter prints with `name: hypogeum-vault`; grep count is `1` (both gates present in the description).

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/hypogeum-vault/SKILL.md
git commit -m "feat(skill): scaffold hypogeum-vault — frontmatter, gating, gotcha

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01SozSUyuyW9fNBS1KeHd5ri"
```

---

### Task 2: The four verbs + explore recipes

**Files:**
- Modify: `.claude/skills/hypogeum-vault/SKILL.md` (append)

**Interfaces:**
- Consumes: the scaffold from Task 1 (the `--vault` gotcha is assumed known).
- Produces: the verb reference + explore recipes. Task 3 builds the audit recipes on the same `find … | while read` lifting idea introduced here.

- [ ] **Step 1: Verify each verb's invocation and JSON shape against `docs/`**

Run:
```bash
/tmp/hypo neighbors --vault docs index.md | jq 'keys'
/tmp/hypo links --vault docs architecture.md | jq '.[0] | keys'
/tmp/hypo search --vault docs "RankByVisit" | jq '.[0] | keys'
/tmp/hypo recent --vault docs | jq '.[0] | keys'
```
Expected shapes (confirm before documenting them):
- neighbors → `["backlinks","file","outbound"]`
- links → `["broken","kind","path","target","text"]`
- search → `["line","path","snippet"]`
- recent → `["path","visited"]`

- [ ] **Step 2: Append the verbs + explore section**

Append to `.claude/skills/hypogeum-vault/SKILL.md`:

````markdown
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
````

- [ ] **Step 3: Verify copy-paste fidelity (run the doc's own commands)**

Run the three Example commands and the three Explore-recipe commands verbatim
(substituting `/tmp/hypo` for `hypogeum`). 
Expected: each returns non-empty, well-formed output (no `file not found`, no jq parse error). E.g. the "where is X" recipe lists `packages/recent.md` among the `RankByVisit` hits.

- [ ] **Step 4: Commit**

```bash
git add .claude/skills/hypogeum-vault/SKILL.md
git commit -m "feat(skill): hypogeum-vault verbs reference + explore recipes

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01SozSUyuyW9fNBS1KeHd5ri"
```

---

### Task 3: Audit recipes — the lift pattern (broken-link sweep + orphan finder)

**Files:**
- Modify: `.claude/skills/hypogeum-vault/SKILL.md` (append)

**Interfaces:**
- Consumes: the per-file verbs from Task 2.
- Produces: the whole-vault audit section, the skill's distinctive value.

- [ ] **Step 1: Verify the broken-link sweep and its triage on `docs/`**

Run:
```bash
find docs -name '*.md' | while read -r f; do rel="${f#docs/}"; \
  /tmp/hypo links --vault docs "$rel" | jq -r --arg F "$rel" '.[] | select(.broken) | "\($F) -> \(.target) [\(.kind)]"'; done
```
Expected: exactly these 4 candidate lines (order may vary):
```
concepts/link-cursor.md -> [[pre-select-inline-link]] [wikilink]
diary/index.md -> ../../.claude/projects/-Users-wilkes-Projects-wilkes-hypogeum/memory/project_finder_first_navigation.md [relative]
superpowers/plans/2026-05-07-wikilinks-and-backlinks.md -> [[Name]] [wikilink]
superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md -> [[Name]] [wikilink]
```
Confirm the triage classification: the two `[[Name]]` are syntax examples in
wikilink docs (false positives); the `diary/index.md` relative link escapes the
vault (intentional cross-vault); `[[pre-select-inline-link]]` is a genuine
dated-filename mismatch (file is `2026-05-09-pre-select-inline-link.md`).

- [ ] **Step 2: Verify the orphan finder on `docs/`**

Run:
```bash
find docs -name '*.md' | while read -r f; do rel="${f#docs/}"; \
  /tmp/hypo neighbors --vault docs "$rel" | jq -r 'select((.backlinks|length)==0) | .file' ; done \
  | sed 's|.*/docs/||' | sort
```
Expected: a list containing `index.md` (the true root) and a set of
`superpowers/plans/*.md` leaf plans (nothing links *to* them). Confirm `index.md`
and at least several `superpowers/plans/...` entries appear.

- [ ] **Step 3: Append the audit section**

Append to `.claude/skills/hypogeum-vault/SKILL.md`:

````markdown
## Audit recipes — whole-vault sweeps

The verbs are per-file. To audit the whole vault, lift them with a loop:
`find <vault> -name '*.md'` → strip the vault prefix so the path is
vault-relative → run the verb. **A sweep surfaces candidates, not verdicts —
always triage the output.**

### Broken-link sweep

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

```bash
find docs -name '*.md' | while read -r f; do rel="${f#docs/}"
  hypogeum neighbors --vault docs "$rel" \
    | jq -r 'select((.backlinks|length)==0) | .file'
done | sed 's|.*/docs/||' | sort
```

Triage: the vault **root** (`index.md`) is an expected orphan, and **leaf
plans** (`superpowers/plans/*.md`) are commonly unreferenced by design — nothing
links *to* a plan. Filter those out; what's left ("an unexpectedly disconnected
note") is the real signal. To exclude the expected cases:

```bash
... | grep -vE '^index\.md$|^superpowers/plans/'
```
````

- [ ] **Step 4: Verify copy-paste fidelity**

Run the two appended sweep commands verbatim (with `/tmp/hypo`). 
Expected: broken-link sweep prints the 4 known candidates; orphan finder prints `index.md` + plan leaves; the `grep -vE` filter removes both expected classes.

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/hypogeum-vault/SKILL.md
git commit -m "feat(skill): hypogeum-vault audit recipes (broken-link + orphan sweeps)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01SozSUyuyW9fNBS1KeHd5ri"
```

---

### Task 4: Global-install section, README pointer, final validation

**Files:**
- Modify: `.claude/skills/hypogeum-vault/SKILL.md` (append the install section)
- Modify: `README.md` (add a short pointer)

**Interfaces:**
- Consumes: the completed skill body.
- Produces: the shippable skill + discoverability pointer.

- [ ] **Step 1: Append the global-install section to SKILL.md**

````markdown
## Installing globally

This skill lives in the hypogeum repo (`.claude/skills/hypogeum-vault/`) so it's
version-controlled with the tool and dogfoods itself against `docs/`. To make it
fire in *any* repo you work in, symlink (or copy) it into your user skills dir:

```bash
ln -s "$PWD/.claude/skills/hypogeum-vault" ~/.claude/skills/hypogeum-vault
```

A symlink keeps it in sync with the repo; a copy pins a snapshot.
````

- [ ] **Step 2: Add a pointer to README.md**

Find the section of `README.md` that lists features or related tooling (e.g. a
"Query mode" / "Scriptable" section, or near the end). Add this line (adapt the
surrounding markdown to match the file's style):

```markdown
- **Agent skill:** [`.claude/skills/hypogeum-vault/`](.claude/skills/hypogeum-vault/SKILL.md) teaches Claude Code (or any skill-aware agent) to explore and audit a markdown vault with the query CLI. Symlink it into `~/.claude/skills/` to use it in any repo.
```

If no natural section exists, add a short `## Agent skill` section near the end.

- [ ] **Step 3: Confirm no code was touched / build + tests still green**

Run: `go build ./... && go test ./... 2>&1 | tail -3 && git status --short`
Expected: build + tests pass; `git status` shows only `.claude/skills/hypogeum-vault/SKILL.md` and `README.md` (plus the already-committed spec/plan/index) — no `.go` files modified.

- [ ] **Step 4: Skim the whole skill for coherence and trigger clarity**

Run: `cat .claude/skills/hypogeum-vault/SKILL.md`
Check: the `description` names both gates (linked vault + installed); every `hypogeum` command uses a vault-relative path; the four sections read in order (gate → availability → gotcha → verbs → explore → audit → install). Consult superpowers:writing-skills if the description needs tightening for trigger accuracy. Fix anything inline.

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/hypogeum-vault/SKILL.md README.md
git commit -m "feat(skill): hypogeum-vault global-install section + README pointer

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01SozSUyuyW9fNBS1KeHd5ri"
```

---

## Self-Review (completed during planning)

- **Spec coverage:** gate/description → Task 1; availability + gotcha → Task 1;
  four verbs → Task 2; explore recipes → Task 2; audit lift pattern + triage →
  Task 3; fallback note → Task 1 ("When NOT") + Task 1 availability; global
  install → Task 4; README pointer → Task 4; dogfooding/validation → the verify
  steps in every task. All spec sections map to a task.
- **Placeholder scan:** every command and the full SKILL.md prose is spelled out;
  no "TBD"/"add X"/"similar to". The only adapt-to-context step is the README
  insertion point (Step 4.2), which gives the exact line to add.
- **Type/shape consistency:** the JSON shapes asserted in Task 2 Step 1
  (`{file,outbound,backlinks}`, `{text,target,path,kind,broken}`,
  `{path,line,snippet}`, `{path,visited}`) match the table and recipes
  throughout; the vault-relative-path rule is stated once (Task 1) and obeyed in
  every later command.

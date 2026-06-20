# hypogeum-vault skill — design

Status: design

## Problem

`hypogeum` ships a non-interactive query CLI (`search` / `links` / `recent` /
`neighbors`, JSON on stdout) that understands a markdown vault's link graph —
wikilinks, backlinks, proximity-resolved relative links. An agent working in a
vault (this repo's `docs/`, an Obsidian vault, any cross-linked `docs/` tree)
will reach for `grep` by default and never discover that a graph-aware tool is
sitting right there. `grep` answers "where does this string appear"; it can't
answer "what links *to* this note", "what does this file reference", or "which
notes are orphans". This skill teaches an agent to reach for hypogeum's query
verbs when the question is about the *graph*, and to fall back to grep when it
isn't.

## Goal

A single agent-facing skill — `.claude/skills/hypogeum-vault/SKILL.md` — that
makes an agent:

1. **Explore** a vault by following its link graph (neighbors, backlinks,
   outbound links, full-text search) instead of grepping blindly.
2. **Audit** a vault's link health (broken links, orphan notes) by lifting the
   per-file query verbs into whole-vault sweeps.

Scope is **both** (explore + audit), delivered as a **playbook** (prose +
copy-pasteable command recipes, no bundled scripts).

## Non-goals

- No new hypogeum code. The skill documents and promotes the *existing* CLI.
- No bundled shell scripts (the maintenance/rot surface the playbook approach
  deliberately avoids — whole-vault loops ship as inline recipes the agent
  runs and adapts).
- Not a replacement for grep. The skill names when grep is the better tool.

## Trigger / gating

The skill is **proactive but gated**. The `description` frontmatter — the text
the harness reads to decide whether to surface the skill — encodes both gates so
the skill never fires and then self-cancels:

> Use when exploring or auditing a directory of interlinked markdown files — a
> vault with `[[wikilinks]]` or a cross-linked `docs/` tree. Query the link
> graph (neighbors, backlinks, outbound links, full-text search) and audit link
> health (broken links, orphan notes) with hypogeum's query CLI instead of
> grep. Only when hypogeum is installed and the directory is actually a linked
> vault.

Two gates: **(1) the directory is a linked vault** (wikilinks present, or a
docs tree with relative cross-links — not a lone README or a flat folder), and
**(2) `hypogeum` is on PATH** (or worth installing). Both are restated in the
body's "When to use / when NOT" section.

## Skill body structure

`SKILL.md` is a single file with this outline:

1. **Frontmatter** — `name: hypogeum-vault`, the gated `description` above.

2. **When to use / when NOT.** Fire on explore-or-audit questions about a
   linked vault with the binary present. Skip for: a single file, a flat folder
   with no link structure, or a plain content grep where the graph adds nothing.

3. **Availability check + fallback.** `command -v hypogeum`. If missing, offer
   `go install github.com/wilkes/hypogeum/cmd/hypogeum@latest` (or build from a
   checkout with `go build -o /tmp/hypo ./cmd/hypogeum`); if the user declines
   or Go is absent, fall back to grep. Never block work on the binary.

4. **The one gotcha that bites — `--vault` paths are vault-relative.** With
   `--vault <root>`, file arguments resolve **relative to the vault root**, not
   the cwd. `hypogeum neighbors --vault docs docs/index.md` fails with a
   path-doubling error (`docs/docs/index.md`); the correct form is
   `--vault docs index.md`. JSON goes to stdout, errors to stderr. Snippet
   highlights are wrapped in `\x11`/`\x12` control bytes — strip them for
   display (`jq … | gsub("[\\u0011\\u0012]";"")` or `tr -d '\021\022'`).

5. **The four verbs** — a compact reference table plus, for each, a runnable
   example against a `docs` vault and the JSON shape:
   - `neighbors <file>` — full 1-hop context: `{file, outbound[], backlinks[]}`.
     Backed by a full `vault.Build`. Use to understand a note's whole
     neighborhood at once.
   - `links <file>` — outbound only: `[{text, target, path, kind, broken}]`.
     Backed by the `OutboundFor` fast path (no full build). Use for "what does
     this reference" and the broken-link sweep.
   - `search "<term>"` — case-insensitive full-text:
     `[{path, line, snippet}]`, recency-ranked. Use for "where is X discussed".
   - `recent` — visit-recency: `[{path, visited}]`, files actually opened in the
     TUI, newest first. Distinct from edit-recency; visited-only.

6. **Explore recipes** (per-file, prose + commands):
   - *Trace a concept's neighborhood* — `neighbors`, then follow the highest-
     value outbound/backlink edges.
   - *"Where is X discussed?"* — `search "X"`, dedup to the file set.
   - *"What depends on this doc?"* — `neighbors … | jq '.backlinks'` (who points
     in).

7. **Audit recipes — the lift pattern** (whole-vault sweeps). The verbs are
   per-file; the skill teaches lifting them with
   `find <vault> -name '*.md'` → relativize each path to the vault root →
   per-file verb, dodging the vault-relative-path gotcha inside the loop. Two
   flagship recipes, **each paired with a triage step** — the sweep surfaces
   *candidates*, not verdicts:
   - **Broken-link sweep** — loop `links` over every file, keep `broken == true`,
     report `file → target [kind]`. Triage: a candidate may be a genuine dead
     link, but also a **literal syntax example** in prose (e.g. a doc *about*
     wikilinks containing `[[Name]]`), a **cross-vault link** that escapes the
     vault root (relative path to a file outside the vault — broken from the
     vault's view but intentional), or a **dated-filename mismatch** (a
     `[[concept]]` wikilink that can't resolve because the file is
     `2026-..-concept.md`). The skill teaches separating these, not treating
     every `broken == true` as a defect.
   - **Orphan finder** — loop `neighbors`, keep files whose `backlinks` is empty.
     Triage: the true root (`index.md`) is an expected orphan, and **leaf plans**
     under `superpowers/plans/` are commonly unreferenced by design (nothing
     links *to* a plan). The recipe filters those out / flags them as expected,
     so the signal is "an unexpectedly disconnected note," not the raw list.

8. **Fallback note** — when grep is genuinely better: exact-string hunts across
   non-vault code, tiny folders, or when the binary isn't available.

9. **Installing globally** — a short section: the skill lives in this repo
   (version-controlled with the tool, dogfoods itself), and copying or
   symlinking the skill directory into `~/.claude/skills/` makes it fire in any
   repo, not just hypogeum.

## Data flow

```
agent has a vault question
  └─ gate: linked vault?  +  hypogeum on PATH?
       ├─ no  → grep / normal tools
       └─ yes → explore (per-file verb) or audit (find→relativize→verb loop)
                  └─ parse JSON (stdout) → answer / report
```

## Testing & validation

- **Recipe dogfooding:** every example and recipe is run against this repo's
  own `docs/` vault during implementation. The four verbs are already proven to
  work this session; the skill codifies the exact, correct invocations
  (including the vault-relative path form).
- **Audit recipes produce expected results on `docs/`:** ground-truth as of
  authoring — the broken-link sweep surfaces **4 candidates**, which the triage
  step correctly classifies: two `[[Name]]` literal syntax examples in the
  wikilink specs (false positives), one intentional cross-vault link in
  `diary/index.md` (escapes the vault root), and one genuine dated-filename
  wikilink mismatch (`[[pre-select-inline-link]]` in `concepts/link-cursor.md`,
  pre-existing). The orphan finder returns `index.md` (the true root) plus the
  unreferenced leaf plans under `superpowers/plans/`. These exact results are
  the skill's worked examples — the recipe's value is demonstrably *surfacing
  candidates for triage*, not a clean pass/fail.
- **Trigger sanity:** the `description` is checked against the writing-skills
  guidance for trigger clarity — it names the gates, not just the topic.
- **No code changes:** `go build ./...` / `go test ./...` are untouched by this
  change (docs + skill only); CI stays green trivially.

## Risks & mitigations

- **Binary drift.** If the CLI's flags/JSON shape change, the prose recipes go
  stale. Mitigation: the skill lives in the hypogeum repo, so a CLI change and
  the skill update land together (same review surface as the `docs/` sync
  convention). No separate scripts to rot.
- **Over-eager firing.** Mitigated by the two-gate description; the body's
  "when NOT" section and the grep-fallback keep it from crowding out grep on
  non-vault work.
- **Path-doubling footgun in audit loops.** Called out explicitly (gotcha
  section) and demonstrated correctly in every recipe, since the loop relativizes
  paths to the vault root before passing them in.

## Workflow

Lands on `feat/hypogeum-vault-skill` → this spec → implementation plan →
`SKILL.md` + the global-install pointer → PR, merged with `--merge`.

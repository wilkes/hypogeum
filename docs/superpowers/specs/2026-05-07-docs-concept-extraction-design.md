# Docs concept extraction — design

**Status:** spec — not yet implemented.
**Scope:** reorganize `docs/` so cross-cutting ideas (sentinel render, vault index, diagnostics, modal geometry, return cursor, link cursor) each live in their own short file under `docs/concepts/`. Existing specs and package docs reference those concept files instead of restating their content. The goal is a meaningfully populated backlinks pane when reading any doc.
**Out of scope:** splitting the wikilinks-and-backlinks spec into per-section specs; converting package-doc cross-references to wikilink syntax; rewriting the dated implementation plans under `superpowers/plans/`.

See also: [docs index](../../index.md), [architecture](../../architecture.md), [wikilinks-and-backlinks-design](2026-05-07-wikilinks-and-backlinks-design.md).

## Motivation

`docs/` already cross-links well — every file has a "See also" header, the index lists all top-level docs, and package docs reference each other. The problem isn't missing links; it's *granularity*. Two docs are large and multi-topic:

- `superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md` (360 lines) covers motivation, vault architecture, modal geometry, resolution rules, diagnostics, error handling, testing, and phasing.
- `superpowers/specs/2026-05-07-backlinks-navigation-design.md` (267 lines) covers cursor mechanics, scroll-to-line, and return-cursor lifetime.

These are the docs the new wikilinks/backlinks feature was designed in. Several ideas inside them — the sentinel-render trick, the vault index, the diagnostic stream, modal geometry, the return cursor, the link cursor — are referenced by *other* docs (architecture, package docs, the older link-following plan) but live as embedded sections inside the big specs. The backlinks pane on those concepts is therefore sparse: a reader pressing `b` while looking at the spec sees nothing pointing back, because the rest of the codebase's docs link to the spec as a whole, not to the section.

Extracting cross-cutting concepts into peer files turns each into a discrete node that multiple specs and package docs can reference by name. The backlinks pane then surfaces "this idea is also discussed in X, Y, Z" with no manual upkeep.

## Architecture

A new `docs/concepts/` folder becomes a peer of `docs/packages/`. Six initial concept files extract material that ≥3 existing docs already discuss:

```
docs/
  index.md                  (updated: adds Concepts section)
  architecture.md           (updated: links to concepts where relevant)
  link-following.md         (updated: short-summary + concept pointers)
  packages/
    {tree,nav,markdown,tui,watch}.md   (updated: link to concepts)
  concepts/                                          (NEW)
    sentinel-render.md
    vault-index.md
    diagnostics.md
    modal-geometry.md
    return-cursor.md
    link-cursor.md
  superpowers/
    specs/
      2026-05-07-wikilinks-and-backlinks-design.md   (updated: section summaries + concept pointers)
      2026-05-07-backlinks-navigation-design.md      (updated: same)
      2026-05-07-docs-concept-extraction-design.md   (NEW: this spec)
    plans/                                           (untouched)
```

The vault's basename-only resolution rule means the folder a concept lives in doesn't change how it's referenced — `[[diagnostics]]` resolves the same from any doc. Filenames must be unique-ish across the vault to avoid disambiguation surprises; the chosen names avoid generic words.

## Components

### The six initial concept docs

| Concept doc | Material it absorbs | Primary referrers (after the move) |
|---|---|---|
| `sentinel-render.md` | `packages/markdown.md` (sentinel-trick section); `link-following.md` (approach narrative); wikilinks-spec (renderer integration paragraphs) | `[[markdown]]`, `[[link-following]]` |
| `vault-index.md` | wikilinks-spec (architecture, components, resolution rules — Vault internals) | wikilinks-spec, future Phase 2 specs |
| `diagnostics.md` | wikilinks-spec (Diagnostics section + relevant error-handling rows) | `[[tui]]`, wikilinks-spec |
| `modal-geometry.md` | wikilinks-spec (Geometry subsection); backlinks-navigation-spec (modal-layout-adjacent paragraphs) | `[[tui]]`, both backlinks specs |
| `return-cursor.md` | backlinks-navigation-spec (return-cursor section) | `[[tui]]`, backlinks-navigation-spec |
| `link-cursor.md` | `link-following.md` (Phase 1 mechanics); `packages/tui.md` (link-cursor invariants); `packages/markdown.md` (Link types where they describe cursor semantics) | `[[link-following]]`, `[[markdown]]`, `[[tui]]` |

Each is roughly 50–100 lines.

### Concept-doc template

```markdown
# <concept name>

<1–2 sentence what-it-is>

See also: [[architecture]], [[index]]. Used primarily by [[<top consumer 1>]] and [[<top consumer 2>]]; press `b` for the full backlinks list.

## Why it exists

<the constraint or problem this solves — usually 1 paragraph>

## How it works

<the mechanism — code-pointer-heavy, links to specific files with line numbers where useful>

## Invariants / gotchas

<bullet list, same style as CLAUDE.md and the package docs use>
```

The "See also" line is hybrid: it names a couple of primary consumers as a starting point for GitHub readers, then defers to the backlinks pane for the rest. This keeps the file readable without a vault and surfaces the in-app feature for hypogeum users.

### Source-spec editing rule

For each section whose material is moving into a concept doc:

1. Keep the section heading. The spec's structure stays intact.
2. Replace the body with 2–3 sentences of "what + why" and a `[[concept]]` pointer to "how."
3. Do not rewrite surrounding sections; the dated specs remain historical records.

Example: the wikilinks spec's `Diagnostics` section (~30 lines) becomes:

> ### Diagnostics
> Errors and warnings during indexing feed a single internal stream with three observers: a transient footer status, an append-only log file, and an in-app log viewer modal (`?`). Severity levels are `info`/`warn`/`error`. Full design and rationale: [[diagnostics]].

### Index updates

`docs/index.md` gains a new section between "Architecture" and "Active feature work":

```markdown
## Concepts

Cross-cutting ideas that show up in multiple specs and packages. Each is its own short file so the backlinks pane (`b`) shows everywhere it's referenced.

- [[sentinel-render]] — how link positions are recovered from Glamour's ANSI output
- [[vault-index]] — forward + reverse reference index, basename resolution, proximity tiebreak
- [[diagnostics]] — the warn/error stream, footer transient, log file, `?` modal
- [[modal-geometry]] — single-modal invariant and layout recompute on `B`/`?`
- [[return-cursor]] — path-keyed cursor restoration on Back navigation
- [[link-cursor]] — content-pane link selection (`n`/`p`/`Enter`/`Esc`) state model
```

The index entries use wikilink syntax to dogfood the feature. The rest of `index.md` keeps its standard markdown links per the existing repo convention.

## Data flow (for a reader)

**Reading a concept doc cold (e.g. arrived from a PR description):**
1. Reader opens `docs/concepts/diagnostics.md` on GitHub or in hypogeum.
2. The "See also" line gives them the two primary consumers as starting points.
3. In hypogeum, they press `b` to see every doc that references `[[diagnostics]]` — likely `tui.md`, the wikilinks spec, future feature specs.
4. In GitHub, they have to traverse manually — but the primary consumers in the See-also line are usually enough.

**Reading the wikilinks spec:**
1. Reader opens the dated spec.
2. Each major section (Vault, Diagnostics, Modal Geometry) has a 2–3 sentence summary and a `[[concept]]` link.
3. Reader either keeps reading top-to-bottom (summaries are self-contained) or jumps into a concept for deeper detail.

**Reading a package doc:**
1. Reader opens `packages/tui.md`.
2. References to cross-cutting concepts use `[[concept]]` instead of inlining the explanation.
3. Backlinks on the concept doc later show this package doc as a referrer.

## Error handling

This is a documentation reorg; failures are limited to:

| Failure | Behavior |
|---|---|
| Concept-doc filename collides with an existing file (e.g. an `internal/diagnostics` package later) | Vault proximity tiebreak handles it; document the collision in the affected concept's "Gotchas" section. |
| `[[concept]]` reference written before the concept doc exists | Renders as broken link with `?` suffix — caught by the post-move verification step (open every doc, confirm no broken wikilinks). |
| GitHub reader hits a `[[wikilink]]` | Renders as literal text. The hybrid See-also line and the concept's `## Why/How` sections are still readable cold; this is the accepted Phase 1 wikilinks-spec tradeoff. |

## Testing

This is a docs change; "testing" means readability and link-integrity verification.

1. **No code touched.** `go build ./...` and `go test ./...` must remain green (smoke check; the changes are markdown-only).
2. **Run hypogeum against the docs folder.** `go run ./cmd/hypogeum docs/` and walk every doc:
   - Each concept doc has a non-empty backlinks pane (the whole point of the exercise).
   - The backlinks pane shows both wikilink referrers (e.g. the index) and standard-markdown-link referrers (e.g. package docs that updated their cross-refs to point at concepts) — verifies the indexer covers both syntaxes uniformly.
   - No broken `[[wikilink]]` markers (`?` suffix) in any doc.
3. **GitHub render check.** Push the branch, open a few changed docs on github.com:
   - Standard markdown links to `concepts/*.md` work (clickable).
   - `[[concept]]` references render as literal text without breaking the surrounding prose.
4. **Spec read-through.** Read both dated specs end-to-end after the edits — confirm each summary is self-contained enough that a reader who skips the concept docs still understands the design.

## Phasing

**Phase 1 (this spec):**
- Create `docs/concepts/` and the six initial concept files using the template above.
- Edit the two dated specs to replace extracted sections with summaries + concept pointers.
- Update `architecture.md`, `link-following.md`, and the five `packages/*.md` files to reference concepts where they currently inline the same material.
- Add the "Concepts" section to `docs/index.md` with wikilink-style entries.
- Run the verification steps above.

**Phase 2 (separate spec, only if needed):**
- Split the wikilinks-and-backlinks spec into per-section specs (e.g. `wikilinks-resolution-design.md`, `backlinks-ui-design.md`). Defer until Phase 2 implementation work needs it; the 360-line spec is fine if it stays a historical record.

**Phase 3 (separate spec, only if needed):**
- Convert package-doc cross-references to wikilink syntax (`[[markdown]]` instead of `[markdown](markdown.md)`). Cost: GitHub renders lose the link. Benefit: shorter doc source, pure-vault dogfooding. Not a clear win; defer.

## Open questions / accepted risks

- **Concept-doc names may collide with future package or feature names.** Mitigation: chosen names avoid generic words. The vault's proximity tiebreaker handles ambiguity automatically; in the worst case a concept gets renamed and referrers get updated in one pass.
- **The two big specs become slightly less useful as standalone reads.** Acceptable: option C from brainstorming kept 2–3 sentence summaries in each section, so a top-to-bottom read of the spec is still coherent. Concept docs are for going deeper, not for replacing the narrative.
- **Backlinks coverage depends on referrers actually being updated.** If the package docs aren't edited to point at concepts, the concept docs are orphans nobody arrives at by browsing. The implementation plan must include the package-doc edits, not just concept creation.
- **`docs/index.md` mixes wikilinks and standard markdown links.** Deliberate: the new Concepts section uses `[[wikilinks]]` to dogfood the feature; the older sections keep standard links so GitHub renders them. Worth a one-line note in the index explaining the mix.
- **Some material may not have a clean concept boundary.** The wikilinks-spec's "Architecture" section discusses both vault internals and the markdown-renderer integration. Splitting it requires judgment about which sentences go to `[[vault-index]]` vs `[[sentinel-render]]`. Done case-by-case in the implementation; not pre-planned here.

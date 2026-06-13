# Development diary

A chronological log of how `hypogeum` was built, reconstructed from the commit history and merged pull requests. Each entry covers one active development day: what shipped, and — where the commits make it visible — *why* it was built that way.

For the durable architecture, start at [docs/index.md](../index.md). This diary is the narrative; the package docs are the reference.

- **Span:** 2026-05-05 → 2026-06-13
- **Released:** v0.1.2, v0.2.0, v0.3.0, v0.4.0 (tag-driven via GoReleaser)

---

## 2026-05-05 — Day one: scaffold to a working browser

The whole spine of the app landed in a single day, all on `main` (before the feature-branch workflow took hold).

- **Scaffold + entrypoint.** Initial commit, then the `cmd/hypogeum` CLI and the first `CLAUDE.md`. An early bug — depth-first auto-open landing on the deepest alphabetical leaf — was fixed immediately to open the first *top-level* file instead.
- **Link following, Phase 1.** `markdown.ExtractLinks` (AST-level discovery) → `RenderWithLinks` (follow-aware rendering) → wired through `refreshContent` → `n`/`p`/`Enter`/`Esc` bindings to cycle and follow links.
- **House style.** A hypogeum look layered on Glamour, iterated three times in one sitting: "page-like" → "feel page-like" → "modern web minimalism." Render width capped at 80 columns. Misaligning tables were converted to definition lists.
- **Mouse + watcher.** Mouse support for tree rows and content links (later routed through BubbleZone), and an `fsnotify`-backed live-update watcher wired into the Bubble Tea model.
- **Hygiene.** Early refactors — dropped an unused `fs.FS` abstraction in `tree`, made Update-path helpers pointer receivers, split `render.go` and `model.go` into focused files, added the first `tree` and history-key tests, and cached terminal-background detection across renderers.

> **Why it matters:** the layering invariant (`tui` depends on the lower packages; they know nothing about the TUI) was set on day one and held for the rest of the project.

---

## 2026-05-07 — Wikilinks, backlinks, and the modal system

The day the app gained a "vault" brain. Two streams: feature commits direct to `main`, then the first six PRs.

- **Diagnostics first.** A `Diagnostics` interface with a `Nop` default, a ring buffer + JSON-line file logger, and a platform-conventional log path (XDG fallback). Building the observability *before* the feature paid off when wikilink resolution needed warnings.
- **The vault.** `internal/vault` scaffolded, a goldmark wikilink inline parser added (with a test proving it doesn't disturb standard-link parsing), then root-walking to index outgoing references, a `Backlinks` query, and wikilink resolution via a basename index with a proximity tiebreaker. `RefreshFile`/`Rebuild` wired into the watcher.
- **Rendering wikilinks.** A `Resolver` interface in `markdown`, options on `NewRenderer` (`WithResolver`, `SetFromFile`), and rewriting `[[wikilinks]]` to standard links *before* Glamour sees them.
- **Modals are born.** A `modalKind` enum + shared modal viewport, a backlinks modal (`B`), and a log-viewer modal — establishing the **single-modal-swap invariant** that governs all later modals. Transient diagnostics shown in the footer, auto-cleared after 3s.
- **Backlinks navigation (Phase 1 follow-on).** Cursor `j`/`k` in the backlinks pane, `Enter` to follow with `scrollToLine`, and a `returnCursor` so `Back` restores where you were. Carefully tested: cursor clamp when the list shrinks, path-keyed (not time-keyed) cursor lifetime, modal reopen after back-from-followed-backlink.
- **Concept docs extraction.** A meta-move: extracted six standalone concept docs (sentinel-render, vault-index, diagnostics, modal-geometry, return-cursor, link-cursor) so the specs could *delegate* to them and the backlinks feature would have real content to index.
- **PRs #1–#6.** Hidable tree pane + collapsible folders, ANSI-aware modal overlay, the `^p` file-picker modal, auto-hide tree below 80 columns, tree-pane scrolling when the flat tree overflows, and a vault-rooted picker showing only markdown + parents.

---

## 2026-05-09 — Help modal, the big refactor sweep, and link-following Phase 2

- **Help modal (#7).** Added `?` help, moved the log viewer to `^l`, trimmed the footer. Post-review: hardened an anchor invariant and deduped the modal-cases test table with a `sized()` helper.
- **Four-track refactor sweep (#8–#12).** Parallel cleanup branches: lower-layer hygiene (exported `tree.IsHidden`/`IsHiddenPath`, split `watch` into watch/debounce/classify), vault carve-up (`backlink.go`, `extract.go`, deduped proximity scoring), a new `internal/wikilink` package to dedupe two parsers, and a TUI Model carve into focused sub-structs (`treeUIState`, `contentUIState`, `backlinksUIState`, `modalUIState`) plus dispatch helpers. Closed with a docs sweep aligning every package doc to the new layout.
- **Link styling (#13).** Dotted-underline links with the URL portion hidden. OSC 8 hyperlinks were tried and **reverted** — they broke BubbleZone hit-testing. A clean example of backing out a change that fought the existing architecture.
- **Link following, Phase 2 (#14).** `HighlightMarker` for reverse-video selection of the active link, with a subtle fix: defer `openMark` until the first non-escape byte inside the link span.
- **Pre-select inline link (#15).** On backlink-follow / Back / Forward, the originating inline link is pre-selected at the destination — `pendingPreselectTarget` consumed in `refreshContent`, with a guard to clear it before early returns.

---

## 2026-05-12 — The finder becomes primary navigation

A pivotal design day: the tree stopped being a side pane and the recency-ranked finder took over as the main way to move around.

- **Unified finder with recency (#17).** New `internal/recent` package — a hybrid mtime + visit-decay score (mtime dominates at equal age), atomic JSON persistence. `pickerState` rewritten as a flat recency-ranked list; visits recorded on `openFile`. Tests isolate `$HOME` so they don't pollute real `visits.json`.
- **Fuzzy filter (#18).** Added `sahilm/fuzzy`; the picker grew a `textinput`, `^j`/`^k` cursor movement (since `j`/`k` now type), match-score-with-recency-tiebreaker sorting, matched-char highlighting, a 200-row cap with an overflow footer, and a no-match state.
- **Finder-first, tree-as-modal (#19, #23, #24, #25).** Tree pane hidden by default → tree moved from side pane to a modal → tree defaults to collapsed (only the current file's ancestor chain opens) → `←`/`→` collapse/expand inside the tree modal. The content now fills the screen; `^p` is the front door. *(This is the decision captured in the [finder-first navigation](../../.claude/projects/-Users-wilkes-Projects-wilkes-hypogeum/memory/project_finder_first_navigation.md) project note.)*
- **Backlinks modal-only (#27).** The persistent backlinks pane was replaced by a modal too — `b` opens it, consistent with the single-modal-swap world.
- **External URL handoff (#28).** `http`/`https` links arm a one-keystroke confirm, then detach via `open`/`xdg-open`/`cmd start`. Non-web schemes are rejected to avoid shell handoffs of executable URLs.

---

## 2026-05-13 — A feature marathon, then the first releases

The single busiest shipping day: nine PRs and the first tagged release.

- **Code-file rendering (#29).** `internal/code` — a Chroma → 256-color ANSI → line-number gutter → soft-wrap pipeline for non-markdown files. Notable fixes: suppressing a phantom trailing gutter row from Chroma's SGR reset, and carrying SGR state across wrap boundaries. The watcher's *write* classifier was widened to any path; the *structure* classifier stayed markdown-only.
- **Source embeds + line-range links (#30, #32).** `![[file#L10-L20]]` embeds preprocess into fenced code blocks with a literal-text gutter; `#L10-L20` fragments on local links navigate into the source with a reverse-video gutter highlight. Live-sync tracks embed deps and re-renders on source change. Four post-merge follow-ups hardened it: skip fenced blocks in `preprocessEmbeds`, a `Row<0` no-scroll sentinel, preserve viewport scroll on Esc-clear, and capture `pendingPreselectRange` in `followBacklink`.
- **Multi-segment cursor (#33).** TDD'd with a failing test first: the link highlight re-opens on every wrapped row, matching the `less`/`vim` visual-mode idiom.
- **Directory listing (#34).** Pointing at a directory synthesizes a markdown listing (dirs-first, trailing `/`) so link cycling still works.
- **Releases + CI (#35, #36, #37, #38).** A `--version` flag and tag-driven GoReleaser pipeline; a fix preserving table column width when suppressing link URLs (using `ansi.StringWidth` instead of a hand-rolled rune decoder); a PR build/test workflow with auto-committed `CHANGELOG.md`; and a GoReleaser exclude-filter fix for scoped/breaking commit variants. **v0.1.2** ships.

---

## 2026-05-14–16 — Full-text search

- **`internal/search` (#39).** A pure, dependency-free substring scanner: `Hit` type, centered-window snippet builder, per-file scan with binary skip and `ctx` cancellation, and `Search` with goroutine fan-out. The TUI side debounces scans at 150ms (each keystroke cancels the prior `scanCtx`), reranks hits by recency, and `Enter` scrolls to the hit line via the same `pendingPreselectRange` plumbing range-links use. A run of five post-merge fixes settled modal repaint, backspace, stale hits, and the cursor-block column.
- **v0.2.0** ships (05-16).

---

## 2026-05-25 — Broken-link tally (#40)

A quieter day: `markdown.CountUnresolvedWikilinks` and a footer tally so unresolved wikilinks are visible at a glance. Unresolved links render as plain text with a `?` suffix and stay *out* of the link cycler — a broken link can't be followed, so making it selectable would be a confusing no-op.

---

## 2026-06-01 — Keybinding dialects and a Glamour upgrade

- **Keybinding dialects (#44).** The big one: a `keyMap` factory (`pagerKeys()` default, `modernKeys()` opt-in) selected via `keysFor(opts.Dialect)`, chosen from a TOML config at the OS-canonical user-config path. Dispatch code stays dialect-agnostic — it just calls `key.Matches`. Pager search bound to `/`, prev-link rebound from `p` to `N`.
- **Glamour table-wrap (#45).** Upgraded Glamour to v0.10.0 so table cells *wrap* instead of character-truncating, pinned with a test for the wrap invariant and two opt-ins (inline table links off, separator chars re-pinned to `│`/`─`).
- **Cleanups (#46, #47, #48, #43).** Skip inline-code backtick spans when rewriting wikilinks (with a fast-path scanner), drop the dead `RenderFile` helper, a docs status sweep, and demoting the finder's visit-weight so mtime wins at equal age.
- **v0.3.0** ships (06-02).

---

## 2026-06-12 — Clipboard ergonomics + an architectural review

- **Drag-to-select with auto-copy (#51).** Left-press in the content pane starts a selection; the *first motion* turns it into a drag, so press-release-without-motion still follows a link. Selected text is cut ANSI-aware from `content.rendered`, copied via both the OS clipboard (`atotto/clipboard`) **and** OSC 52 (`termenv.Copy`) — covering local terminals *and* SSH/tmux. The design was corrected mid-flight (OSC 52, not `tea.SetClipboard`) and the OS-clipboard path was added after noticing OSC 52 alone misses macOS Terminal.app.
- **Copy current path (#52).** `y` (pager) / `^y` (modern) copies the open file's absolute path, added to both dialects and surfaced in the help cheat sheet.
- **DDD architecture review (#53).** A reflective doc reviewing the codebase through a domain-driven-design lens — the seed for the refactor trilogy that followed.
- **v0.4.0** ships.

---

## 2026-06-13 — Refactor trilogy from the DDD review

Four refactors acting on the previous day's review, each its own PR (reason-to-change as the organizing principle):

- **`pathutil.ResolveRelativeTo` (#54)** — extracted the shared relative-path logic out of `markdown` and `vault`.
- **Centralized snippet highlight markers (#55)** — unified the `\x11`/`\x12` control-char markers across `search` and `vault`.
- **`pendingNav` + status split (#56)** — pulled navigation intent into `pendingNav` and split status into `currentPath` + `footerMessage`.
- **Split `links_render.go` by reason-to-change (#57)** — the most recent commit on `main`.

> **Where things stand:** content-first markdown browsing with link following (both phases), wikilinks + backlinks, recency fuzzy finder, full-text search, code-file rendering, source embeds, directory listings, two keybinding dialects, drag-to-copy, and tag-driven releases. Still open per CLAUDE.md: block references (`[[note#^blockid]]`) and a configurable vault root.

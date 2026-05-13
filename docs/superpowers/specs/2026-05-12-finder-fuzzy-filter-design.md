# Finder fuzzy filter — design

**Status:** drafted on 2026-05-12.
**Scope:** add type-to-filter fuzzy matching to the recency-ranked finder modal (`^p`). Always-on text input, fzf-style subsequence matcher, case-insensitive, match-score-first with recency as tiebreaker. Highlights matched characters in the rendered rows.
**Out of scope:** content (body-text) search, multi-mode picker (sort cycling), configurable matcher constants, regex queries, query history.

See also: [docs index](../../index.md), [unified-finder-recency](2026-05-12-unified-finder-recency-design.md) (the recency phase this extends), [`internal/tui`](../../packages/tui.md).

## Motivation

The recency-ranked finder shipped on 2026-05-12 answers "what was I just on?" well, but answers "open `hypogeum.md`" poorly. With a vault of any size the file you want by name is rarely in the top of the recency list — you scroll, or you give up and use the tree.

The original [unified-finder-recency](2026-05-12-unified-finder-recency-design.md) spec deferred name-filter typing to Phase 2 explicitly. This spec is that Phase 2.

Replacing the picker again is unnecessary — the recency list is the right base. We layer a text-input prompt on top, run a fuzzy matcher over the captured ranked list on every keystroke, and re-sort by match score with the existing recency score as a tiebreaker. Empty query reverts to the recency-only behavior.

## Architecture

No new internal package. Changes are confined to `internal/tui/picker.go` and the modal-key dispatch in `internal/tui/input.go`. `internal/recent` stays ignorant of text filtering — it still owns the time-decay scoring, returns `[]Ranked` against a full path list, and is unchanged.

Two new module dependencies:

- `github.com/charmbracelet/bubbles/textinput` — the query prompt widget. Same module as the already-vendored `bubbles/viewport`.
- `github.com/sahilm/fuzzy` (MIT, ~250 LOC, zero deps) — fzf-style subsequence matcher. Returns `Match{Index, Score, MatchedIndexes}` for each input string that scored above zero.

The package dependency graph is unchanged:

```
internal/tui ── textinput, fuzzy, recent (recent unchanged)
```

## Components

### `internal/tui/picker.go` — `pickerState`

```go
type pickerState struct {
    all     []recent.Ranked  // full ranked list captured at open time
    ranked  []recent.Ranked  // currently visible (filtered or all)
    matches []fuzzy.Match    // parallel to ranked when filtered; nil when query empty
    cursor  int              // index into ranked
    vp      viewport.Model
    root    string
    input   textinput.Model  // the query line
}
```

Invariants:

- `len(matches) == len(ranked)` when `input.Value() != ""`; `matches == nil` otherwise.
- `ranked` always holds *all* candidates (filtered or full); the `pickerMaxVisible` cap is applied at render time, not in the slice. This keeps `cursor` math and the "X more" footer trivial.
- `all` is set once on open. The matcher runs per keystroke; `recent.Rank` does not.
- `cursor` is clamped to `[0, min(len(ranked), pickerMaxVisible) - 1]` on every reflow; `0` when `ranked` is empty.

Package constant:

```go
const pickerMaxVisible = 200
```

If the post-match list is longer, render the first 200 and append a faint `… N more` footer line. Cursor cannot scroll past 199; refining the query reaches hidden rows.

### Filtering pipeline (`refilter`)

```go
func (p *pickerState) refilter() {
    q := strings.ToLower(p.input.Value())
    if q == "" {
        p.ranked = p.all
        p.matches = nil
        p.cursor = 0
        p.refreshVP()
        return
    }

    // Build a parallel []string of lowercased paths to feed the matcher.
    // fuzzy.Find walks the slice; one match call covers the whole list.
    src := make([]string, len(p.all))
    for i, r := range p.all {
        src[i] = strings.ToLower(relativeTo(p.root, r.Path))
    }
    raw := fuzzy.Find(q, src)  // already sorted by score desc

    // Stable secondary sort by recency score (which is the order in p.all).
    // raw[i].Index points back into p.all, so we re-sort with a comparator
    // that uses fuzzy score primary, p.all-order (recency) secondary.
    sort.SliceStable(raw, func(i, j int) bool {
        if raw[i].Score != raw[j].Score {
            return raw[i].Score > raw[j].Score
        }
        return raw[i].Index < raw[j].Index  // earlier in p.all = more recent
    })

    p.ranked = make([]recent.Ranked, len(raw))
    p.matches = make([]fuzzy.Match, len(raw))
    for i, m := range raw {
        p.ranked[i] = p.all[m.Index]
        p.matches[i] = m
    }
    p.cursor = 0
    p.refreshVP()
}
```

Notes:

- Case handling: lowercased query against lowercased paths. (Decision: "always case-insensitive.")
- `fuzzy.Find` returns matches sorted by score descending. The stable resort preserves that order within a score tier and uses recency rank (`p.all` index) as the tiebreaker — `p.all` is already sorted by recency score, so its index is a faithful recency proxy.
- The lowercased-paths slice is rebuilt on every keystroke. For a 10K-path vault this is ~200KB of allocation per keystroke. If profiling shows this matters, cache it on open. Not premature.

### Key handling (`internal/tui/input.go`)

The modal-open key block already routes `modalPicker` to a dedicated `switch`. The new layout while picker is open:

| Key | Action |
|---|---|
| `Esc` (query non-empty) | Clear query; picker stays open |
| `Esc` (query empty) | Close picker |
| `Enter` | Open selected row (no-op if `ranked` empty) |
| `↑` / `↓` | Move cursor |
| `^j` / `^k` | Move cursor (vim muscle memory; `j`/`k` are now typing) |
| `^p` | Toggle picker closed (existing) |
| anything else | Forwarded to `textinput.Update` |

After forwarding to `textinput.Update`, the handler compares `input.Value()` before/after; on change, calls `pickerState.refilter()`. `cursor` is reset to 0 inside `refilter()`.

Modal-swap invariants from [unified-finder-recency](2026-05-12-unified-finder-recency-design.md) hold: `B`, `^l` while picker is open swap to those modals; `?` is a no-op (anchored). Those handlers run before the picker key block.

New `keyMap` entries:

```go
PickerCursorDown key.Binding  // ^j
PickerCursorUp   key.Binding  // ^k
```

These coexist with `keys.Down` / `keys.Up` (which still cover `↓`/`↑`); they're separate bindings so the help cheat sheet and `?` text can show both forms.

### Rendering

The picker view gets two new rows above the existing flat list: prompt + separator.

```
┌────────────────────────────────────────────────────┐
│ > projhyp_                                         │
│ ────────────────────────────────────────────────── │
│ notes/projects/hypogeum.md            2h ago       │
│ notes/projects/hypothesis.md          yesterday    │
│ docs/projects/hyperfocus.md           3d ago · ed… │
│ …                                                  │
│ … 47 more                                          │
└────────────────────────────────────────────────────┘
```

- **Prompt line:** `"> " + p.input.View()`. `textinput`'s built-in blinking cursor and style.
- **Separator:** one row of `─` repeated to viewport width.
- **Result rows:** existing `formatPickerRow` plus inline highlighting of `MatchedIndexes` bytes via a bold + bright-cyan lipgloss style. Selected-row reverse-video wraps the whole rendered line.
- **Overflow footer:** when `len(p.ranked) > pickerMaxVisible`, append a faint `… N more` row.
- **Empty-result state:** when `input.Value() != ""` and `len(p.ranked) == 0`, render a single faint row `(no match for "<query>")` in place of the list. Cursor and Enter become no-ops.

`resizePicker` shrinks the viewport height by 2 (one prompt row + one separator row) before setting `vp.Height`.

#### Highlight + truncation interaction

`MatchedIndexes` are byte offsets into the lowercased source path. The row formatter currently truncates with a leading ellipsis (`truncateLeadingEllipsis`) to fit the column. The byte indices won't survive an arbitrary truncation. Two approaches:

1. Translate indices through the truncated string by re-running `fuzzy.Find(query, []string{truncated})` for the one visible row and using the returned `MatchedIndexes`. Cheap (one match call per visible row, ≤200 rows). Simple. Always correct after truncation.
2. Carry the original string and the truncation offset together; remap indices arithmetically.

We pick **(1)**. The simplicity is worth the negligible cost. (Note: `sahilm/fuzzy` only exposes `Find` over a slice; single-string matching uses `Find(q, []string{s})`.)

### Rendering snippets

```go
// renderQueryPrompt returns the "> <query>" line for the modal header.
func (p *pickerState) renderQueryPrompt() string { … }

// renderSeparator returns a ─ row at the modal interior width.
func (p *pickerState) renderSeparator() string { … }

// highlightMatch wraps matched bytes in the bold/cyan style. Called per
// visible row after truncation; re-runs fuzzy.Find against the (possibly
// truncated) display string so indices map correctly. When the query is
// empty or the truncated row no longer matches, returns display unchanged.
func highlightMatch(display, query string, sty lipgloss.Style) string { … }
```

## Data flow

**Picker open (`^p`):**

1. `paths := m.allVaultMarkdownPaths()` (existing).
2. `m.modals.picker.all = m.recent.Rank(paths)` (existing call, stored on `all`).
3. `m.modals.picker.ranked = m.modals.picker.all` (full slice; render caps).
4. `m.modals.picker.matches = nil`.
5. `m.modals.picker.cursor = 0`.
6. `m.modals.picker.input.SetValue(""); m.modals.picker.input.Focus()`.
7. `m.modals.picker.refreshVP()`.

**Keystroke while picker open:**

1. Modal-swap keys (`B`, `^l`, `?`) — handled by existing block before picker dispatch.
2. `Esc` — clear query if non-empty (`SetValue("")`, `refilter()`); otherwise close picker.
3. `Enter` — `m.openFile(ranked[cursor].Path)` and close.
4. `↑` / `↓` / `^k` / `^j` — `cursorMoveAndRefresh`.
5. `^p` — toggle close.
6. Else — `m.modals.picker.input, _ = m.modals.picker.input.Update(msg)`; if the value changed, call `refilter()`.

**Picker selection:** unchanged from the recency spec — `openFile` records the visit, history updates, content refreshes.

## Error handling

| Failure | Behavior |
|---|---|
| `sahilm/fuzzy.Find` panics or errors | It doesn't (the API returns `Matches` for any input including empty). No defensive code. |
| `textinput.Update` returns an error | The widget's `Update` does not return errors. No new error surface. |
| Vault has zero markdown files | `all` is empty; query box accepts input; `ranked` stays empty. `(no match for …)` row appears on any non-empty query; Enter is a no-op. |
| Multibyte path | `sahilm/fuzzy` operates on runes; `MatchedIndexes` are byte offsets. The renderer walks the display string by rune while testing byte offsets against indices. Covered by test. |
| Paste of a multi-line string into the query | `textinput` strips newlines by default. No special handling. |
| Very long query (1000+ chars) | `Find` over 10K paths × 1000-char query is <100ms. Acceptable; no throttling. |
| Vault grows mid-session | `all` is captured at open time and not refreshed. A file added by an editor between two `^p` opens shows up on the next open. The watcher's `StructureChanged` does not reach into the picker — by design (the picker is short-lived). |

## Testing

A new `internal/tui/picker_fuzzy_test.go` (kept separate from `picker_test.go` so existing recency tests stay untouched on review):

- **Empty query shows full recency list.** Open picker, assert `ranked` equals (a prefix of) `all`.
- **One keystroke filters.** Fixture of three files; type `h`; only paths containing `h` appear; order is by match score, recency as tiebreaker.
- **Score tie + recency tiebreaker.** Two paths with identical fuzzy scores against a known query — the earlier-in-`all` (more-recent) one ranks first.
- **Cursor resets to 0 on query change.** Move cursor to row 2; type one more char; cursor is back to 0.
- **Esc with non-empty query clears query.** Query empties, picker stays open, full list restored.
- **Esc with empty query closes picker.** `modals.kind == modalNone`.
- **Enter on filtered list opens correct path.** Query narrows to one row; Enter calls `openFile` with that path.
- **`^j` / `^k` move cursor.** Even with textinput focused.
- **No-match query.** Query `xyzqq` shows `(no match for "xyzqq")`; Enter is a no-op.
- **Overflow.** 250 paths all matching the query; only 200 rendered; footer reads `… 50 more`; cursor clamps to 199.
- **Highlight bytes.** With a known query and path, the rendered row contains the bold-style ANSI for the expected character positions. (One test, not exhaustive.)
- **Multibyte basename.** A file named `日本語.md` filters on the query `日`. Confirms rune/byte handling in the renderer.

Existing tests in `picker_test.go` and `dispatch_test.go` must continue to pass — the new code is additive at the picker level and adds a refilter-on-input branch in dispatch.

## Documentation updates

- `CLAUDE.md`: extend the `^p` gotcha to mention type-to-filter, `^j`/`^k` for navigation, Esc-clears-then-closes.
- `README.md`: update the `^p` row in the keys table; add a one-liner about typing to filter.
- `docs/index.md`: add this spec under "Active feature work."
- `docs/packages/tui.md`: extend the finder section with the filter + matcher.
- `docs/superpowers/specs/2026-05-12-unified-finder-recency-design.md`: add a "Extended by: [finder-fuzzy-filter](2026-05-12-finder-fuzzy-filter-design.md)" line near the top of the Phase 2 list.

## Open questions / accepted risks

- **No fuzzy-score weights configurable.** `sahilm/fuzzy`'s scoring is "Smith-Waterman-ish" with bonuses for word boundaries and consecutive runs. If those bonuses don't match preference (e.g. you'd want basename matches to outweigh deep-folder matches more aggressively), the fix is a post-`Find` re-scoring layer in the TUI, not a fork of the matcher. Acceptable.
- **No async filtering.** The matcher runs synchronously on every keystroke. Worst case ~50ms for a 10K-vault — slower than ideal but well under the "feels laggy" threshold. If it ever becomes a problem, debounce via a Bubble Tea `tea.Tick` with a 30ms delay.
- **Highlight + truncation cost.** Re-running `fuzzy.Match` per visible row for index remapping is O(N×Q) where N≤200 and Q is query length. Cheap. If it shows up in a profile, switch to arithmetic remapping.
- **No multi-word AND queries.** `"hyp design"` matches the literal string `"hyp design"` (with a space), which won't appear in paths. Standard fzf splits on spaces and treats each token as an independent subsequence requirement. We accept this limitation in v1 — paths rarely benefit from multi-word queries.
- **`^p` is still picker-toggle.** Doesn't become "next match" (a common fzf-style binding). The toggle behavior is established; `^j` covers vim-next.
- **No query persistence across opens.** Each `^p` starts with an empty query. Persisting the previous query is plausible but introduces a "what does an old query mean for a refreshed file list?" question we'd rather not answer in v1.

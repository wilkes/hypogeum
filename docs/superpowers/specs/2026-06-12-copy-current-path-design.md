# Copy current file path — design

## Goal

Add a keybinding that copies the absolute path of the currently-viewed
file (or directory) to the clipboard, so users can paste it into a shell,
editor, or note.

## Behavior

- **Keys:** `y` (pager dialect) / `Ctrl+Y` (modern dialect).
- **What is copied:** the absolute path of the current view, taken from
  `m.history.Current()` (history entries are already absolute).
- **Feedback:** on success, toast `Copied path: <path>` via `m.diag.Info`.
  This reuses the existing transient footer that drag-to-select copy uses;
  it is cleared by the perpetual `clearTransientAfter` loop started in
  `Init`, so no new tick command is needed.
- **Empty view:** if `m.history.Current()` is `""` (nothing open), the
  keypress is a no-op and shows no toast.
- **Clipboard mechanism:** reuses `m.copyToClipboard`, which writes both
  the OS clipboard (pbcopy/xclip/wl-copy via `atotto/clipboard`) and an
  OSC 52 escape (`termenv.Copy`). It is injectable, so tests capture the
  copied string without touching a real clipboard.

Out of scope (YAGNI): copying a relative or vault-rooted path, copying
just the filename, and any config option to choose the format. The
request is "full path" → absolute only.

## Wiring

Follows the "adding a new action" recipe documented in CLAUDE.md
(keybinding-dialects gotcha):

1. **`internal/tui/keys.go`** — add `CopyPath key.Binding` to `keyMap`.
2. Bind it in **both** dialect factories:
   - `pagerKeys()`: `key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy path"))`
   - `modernKeys()`: `key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("^y", "copy path"))`
3. **`internal/tui/input.go`** — dispatch in the global key handler:
   `key.Matches(msg, m.keys.CopyPath)` → copy `m.history.Current()` (when
   non-empty) and toast.

### Dispatch placement and edge cases

- The handler lives in the **global key block**, not gated on content
  focus.
- The picker grabs printable runes *before* the global switch (documented
  CLAUDE.md invariant), so typing `y` into the picker's fuzzy-filter query
  still types `y` — copy-path only fires when the picker isn't
  intercepting runes. `Ctrl+Y` is a non-rune chord and flows through
  normally.
- `y` is currently unbound in the pager dialect, so there is no collision.

## Testing

Model-level tests only (no TTY required), per CLAUDE.md:

1. **Copies the current absolute path.** Inject a fake `copyToClipboard`
   that captures its argument; open a file; send the `CopyPath` keypress;
   assert the captured string equals the current absolute path and the
   footer shows the `Copied path:` toast.
2. **No-op on empty view.** With no current file, send the keypress and
   assert nothing was copied and no toast appears.
3. **Dialect coverage.** The new `keyMap` field must be bound in both
   factories so the existing all-actions-bound dialect tests stay green.

## Files touched

- `internal/tui/keys.go` — new field + two bindings.
- `internal/tui/input.go` — dispatch + copy/toast logic.
- `internal/tui/model_test.go` (or a sibling `_test.go`) — the tests above.
- `CLAUDE.md` — note the new keybinding in the feature summary if
  appropriate.

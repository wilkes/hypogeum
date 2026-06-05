# Copy document path to clipboard

Status: shipped — `y` (pager) / `alt+c` (modern) copies the open document's
absolute path to the system clipboard.

## What

A keybinding copies the absolute path of the currently-open document to the
system clipboard, so it can be pasted into another tool (e.g. a Claude
session) to point that tool at the file.

- **Pager dialect:** `y` (vim yank idiom).
- **Modern dialect:** `alt+c` (`ctrl+c` is taken by quit).
- Path format is **absolute** — directly usable when pasted into a Claude
  session pointed at the repo.
- Confirmation is a footer transient ("copied path: …") via the diagnostics
  stream, so it auto-clears and also lands in the `^l` log. Failures surface
  as a footer error.

## Implementation

- `internal/tui/clipboard.go` — `clipboardWriter` (injectable, mirrors
  `externalOpener`) and the default `copyToClipboard`, which shells out to the
  platform tool: `pbcopy` (macOS), `clip` (Windows), or, on Linux/BSD, the
  first of `wl-copy` / `xclip` / `xsel` found on `PATH` (actionable error if
  none). `copyCurrentPath` resolves `m.history.Current()` to an absolute path
  and writes it, reporting via `m.diag`.
- `internal/tui/keys.go` — `CopyPath` binding in both dialect factories.
- `internal/tui/input.go` — dispatched from `handleContentKey`.
- `internal/tui/help.go` — listed under a new "Clipboard" section in the `?`
  cheat sheet.
- `internal/tui/clipboard_test.go` — covers the pager + modern chords, the
  absolute-path guarantee, writer-error surfacing, and the no-document no-op.

## Possible follow-ups

- A second binding for the vault-relative path.
- Copying the path of the tree-modal cursor row or a selected link, not just
  the open document.
- OSC 52 clipboard escape as a fallback when no helper binary is installed
  (works over SSH); not pursued now since `bubbletea` v1.3.4 exposes no
  clipboard command and shell-out matches the existing external-open pattern.

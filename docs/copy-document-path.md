# Copy document path to clipboard

Status: idea — not yet designed or built.

## What

Add a keybinding to copy the path of the currently-open document to the system
clipboard, so it can be pasted into another tool (e.g. a Claude session) to
point that tool at the file.

## Why

When reading a note in hypogeum and wanting to hand it off to a Claude session
(or any other tool), there's currently no way to grab the file's path without
leaving the app and reconstructing it by hand. A one-keystroke "copy path"
closes that gap.

## Open questions

- **Absolute vs. vault-relative path?** Pasting into a Claude session usually
  wants an absolute path (or one resolvable from the repo root). Possibly offer
  both — e.g. one key for absolute, another for vault-relative.
- **Which path is "current"?** The open document is the obvious target. Should
  the tree-modal cursor row or a selected link also be copyable?
- **Clipboard mechanism.** No clipboard dependency exists today. Options:
  pull in a cross-platform clipboard library, or shell out to
  `pbcopy` / `xclip` / `wl-copy` / `clip.exe` per-platform (matches the
  existing `open` / `xdg-open` / `cmd start` external-handoff pattern in
  `internal/tui/external.go`).
- **Keybinding.** Needs a binding in both `pagerKeys()` and `modernKeys()`
  dialects (see `internal/tui/keys.go`) plus dispatch wiring.
- **Feedback.** Show a footer transient ("copied <path>") via the existing
  diagnostics stream so the user knows it worked.

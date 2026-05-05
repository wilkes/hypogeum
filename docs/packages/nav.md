# `internal/nav`

Browser-style back/forward stack of opaque path strings. No filesystem awareness, no UI, no I/O — just a cursor over a slice.

See also: [architecture overview](../architecture.md), [`internal/tui`](tui.md) (the only consumer).

## Purpose

Models the user's reading trail. Every time they open a file, that path joins history; `h` moves the cursor backward, `l` moves it forward. Familiar from any web browser.

## Types

```go
type History struct {
    entries []string
    cursor  int // -1 when empty
}
```

Construct with `nav.New()`. The cursor is the *current* entry; everything before is "back," everything after is "forward."

## Public surface

- `Visit(path string)` — record a new current entry. Truncates forward history (browser semantics). Visiting the same path as the current entry is a no-op so re-rendering doesn't pollute history.
- `Current() string` — the current path, or `""` when empty.
- `CanBack() bool`, `CanForward() bool` — predicates for the UI.
- `Back() (string, bool)`, `Forward() (string, bool)` — move the cursor and return the new current.

## Key invariants

- **Visit-the-same is a no-op.** Without this, a window resize (which calls `refreshContent`) would push a duplicate entry every time. The TUI relies on this — don't add a "force re-visit" variant without thinking about that path.
- **Visiting truncates forward.** Matches browser behavior. If you went back three pages and then clicked a link, the three forward pages are gone.
- **Strings are opaque.** `nav` doesn't care if the string is a path, a URL, an anchor — it's just a label. The TUI happens to use absolute file paths.

## Why it's in its own package

Small, pure, and frequently the place reviewers want to confirm correctness without being distracted by Bubble Tea or Glamour. Tests live next to the code (`history_test.go`) and run in microseconds.

Resist adding filesystem awareness here ("does this file still exist?") or UI awareness ("which entry is highlighted?"). Those belong in `tui` or `markdown` respectively. The package being boring is a feature.

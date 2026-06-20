# diagnostics

The single internal stream of `info`/`warn`/`error` events that surfaces non-fatal issues to the user via three observers: a transient footer status, an append-only log file, and an in-app log viewer modal (`^l`).

See also: [architecture](../architecture.md), [docs index](../index.md). Used primarily by [`internal/tui`](../packages/tui.md) and the [wikilinks-and-backlinks design](../superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md); press `b` for the full backlinks list.

## Why it exists

Several subsystems can fail non-fatally — a parse failure on one vault file, a `RefreshFile` race when a file is deleted between event and read, a watcher that exhausts inotify limits. The TUI must surface these without aborting and without forcing the user to know to look. Three observers cover the three usage modes:

- **Real-time.** Footer transient: the most recent diagnostic appears for ~3 seconds, then clears.
- **Audit.** JSON-line log file the user can `tail` from another terminal.
- **In-session review.** `^l` opens a modal showing the last 200 entries from an in-memory ring buffer.

A single severity-tagged stream means new diagnostics added later (render times, rebuild durations) land at `info` without any plumbing changes.

## How it works

The TUI owns the diagnostic sink. It implements a `vault.Diagnostics` interface (`Info(string)`, `Warn(string)`, `Error(string)`) and passes itself to `vault.Build`. The vault calls back through the interface; `internal/tui` adds UI-side issues directly. All three calls fan out to the three observers.

**Footer transient:** the latest diagnostic populates `m.status`. A `tea.Tick` clears it after ~3s. Severity is shown via color cue (warn = yellow, error = red, info = dim).

**Log file:** appended to `$XDG_STATE_HOME/hypogeum/hypogeum.log` (Linux) or `~/Library/Logs/hypogeum/hypogeum.log` (macOS). One JSON line per entry: `{ts, severity, message}` (the `diagEntry` struct in `internal/tui/diagnostics.go` has no `source` field). Path resolution falls back to `~/.local/state/hypogeum/` if `XDG_STATE_HOME` isn't set. If no path is writable, file logging silently disables — the in-memory buffer and footer still work.

**In-app log viewer modal (`^l`):** reuses the modal infrastructure built for backlinks. The 200-entry ring buffer is the source. Severity color cues match the footer. `Esc` closes; `j`/`k` scroll. See [[modal-geometry]] for layout rules.

## Invariants / gotchas

- **Phase 1 emits only `warn` and `error` (and one `info` for `RefreshFile` races).** Severity is plumbed through so future diagnostics can land at `info` without API changes.
- **The log file is unbounded.** No rotation in Phase 1. Volume is low (one warn per parse failure, etc.); long-running sessions over many days could grow the file. If this becomes a problem, add a 10MB cap with single-file rotation. The user can `rm` the log file at any time without affecting the running session — the in-memory ring buffer is independent.
- **The diag sink is required by `vault.Build`.** Tests pass a no-op implementation. Don't make `Diagnostics` optional or nil-handling will leak into vault code.
- **Single-modal invariant.** Pressing `^l` while the backlinks modal is open swaps content — the two modals share one viewport. The help modal (`?`) is anchored and does *not* participate in the swap. See [[modal-geometry]].
- **Don't log secrets.** No content of vault files goes through diagnostics — only paths and short error messages. The log file is plain-text on disk.

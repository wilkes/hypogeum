// Package vault indexes wikilinks and standard markdown links across a
// directory of markdown files, supporting backlink queries.
package vault

// Diagnostics is the sink vault uses to surface non-fatal issues
// (parse failures, refresh races, etc.) to the user. The TUI implements
// this interface; tests can pass a no-op or recording implementation.
//
// Severity contract:
//   - Info: incidental events (e.g. a file vanished between watcher event
//     and re-read). Not surfaced unless the user opens the log viewer.
//   - Warn: degraded but recoverable (e.g. one file in the vault failed
//     to parse — its references are missing but the rest of the index
//     is still usable).
//   - Error: a vault operation hit something that prevents the requested
//     work. Phase 1 doesn't emit any Error from vault — fatal errors
//     are returned from Build/RefreshFile/Rebuild instead.
type Diagnostics interface {
	Info(msg string)
	Warn(msg string)
	Error(msg string)
}

// NopDiagnostics drops all messages. Useful as a default and in tests
// that don't assert on diagnostic emission.
type NopDiagnostics struct{}

func (NopDiagnostics) Info(string)  {}
func (NopDiagnostics) Warn(string)  {}
func (NopDiagnostics) Error(string) {}

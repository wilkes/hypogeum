# Wikilinks and Backlinks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Phase 1 of the wikilinks and backlinks feature in `hypogeum`: a new `internal/vault` package that indexes both `[[wikilinks]]` and standard markdown links, surfaced via a persistent backlinks pane, a backlinks modal, and a log viewer modal — all driven by a shared diagnostics stream.

**Architecture:** A new `internal/vault` package becomes a peer of `tree`/`markdown`/`nav`/`watch`. It owns a goldmark extension that parses `[[...]]`, a basename + forward + on-demand reverse index covering both syntaxes, and watcher-driven incremental refresh. `internal/markdown` gains a small `Resolver` interface (defined locally; implemented by `vault.Vault`) that the renderer uses to turn wikilink AST nodes into either standard markdown links (resolved) or styled placeholders (unresolved). `internal/tui` consumes the vault for backlinks UI and owns a diagnostics ring buffer + log viewer modal.

**Tech Stack:** Go, Bubble Tea (TUI), Bubbles (viewport, key bindings), Lip Gloss (styling), Glamour (markdown → ANSI), goldmark (markdown AST), fsnotify (filesystem watching), bubblezone (mouse hit-testing).

**Spec:** [`docs/superpowers/specs/2026-05-07-wikilinks-and-backlinks-design.md`](../specs/2026-05-07-wikilinks-and-backlinks-design.md)

---

## Stages

The plan is grouped into stages. Each stage leaves the tree green and ships something testable. Stages are sequential.

1. **Diagnostics foundation** — `Diagnostics` interface, ring buffer, file logging. No UI yet.
2. **Vault scaffolding** — package skeleton with empty `Build`, `RefreshFile`, `Rebuild`, `Resolve`, `Backlinks`. Wire into `tui.New` so it builds at startup but doesn't change visible behavior.
3. **Wikilink parser** — goldmark extension producing `wikilinkNode` AST nodes. Standalone tests; not yet wired into rendering or indexing.
4. **Indexer** — populate the forward and reverse indexes from both wikilinks and standard markdown links.
5. **Resolver + markdown integration** — `Resolver` interface in `markdown`; renderer emits resolved/unresolved wikilinks correctly.
6. **Watcher integration** — `RefreshFile` and `Rebuild` driven by `watch.Event`s.
7. **Snippet extraction** — smallest enclosing block as plain text with the link's display text highlighted via SGR.
8. **Backlinks persistent pane** (`b`).
9. **Modal infrastructure + backlinks modal** (`B`).
10. **Log viewer modal + transient footer** (`?`).
11. **Polish: footer transient timing, broken-link footer message, diagnostics call-sites in vault and tui.**

---

## File Structure

### Created files

| Path | Responsibility |
|---|---|
| `internal/vault/vault.go` | `Vault` struct, `Build`, `RefreshFile`, `Rebuild`, `Backlinks`, indexes, internal types |
| `internal/vault/resolver.go` | `Resolve` method (basename lookup, proximity tiebreaking) |
| `internal/vault/parser.go` | Goldmark extension: `[[...]]` → `wikilinkNode` |
| `internal/vault/snippet.go` | AST → smallest-enclosing-block plain-text extraction with highlight |
| `internal/vault/diagnostics.go` | `Diagnostics` interface |
| `internal/vault/vault_test.go` | Build / RefreshFile / Rebuild / Backlinks behavior |
| `internal/vault/resolver_test.go` | Resolution rules, case insensitivity, proximity tiebreaker |
| `internal/vault/parser_test.go` | Each wikilink form parses to expected node |
| `internal/vault/snippet_test.go` | Snippet extraction across paragraph / list / blockquote |
| `internal/tui/diagnostics.go` | Ring buffer, file logger, transient status driver |
| `internal/tui/diagnostics_test.go` | Footer transient, ring buffer, file fallback |
| `internal/tui/backlinks.go` | Persistent pane + modal rendering, `Backlink` row formatter |
| `internal/tui/backlinks_test.go` | `b`/`B` keys, geometry, navigation |
| `internal/tui/modal.go` | `modalKind` enum, modal viewport, geometry helpers shared by backlinks + logs |
| `internal/tui/logs.go` | Log viewer modal — formats ring buffer entries |
| `internal/tui/logs_test.go` | `?` opens modal, listing matches buffer, modal swap |

### Modified files

| Path | Responsibility |
|---|---|
| `internal/markdown/render.go` | Add `Resolver` interface, options pattern (`NewRenderer(width, opts...)`), `WithResolver`, `WithFromFile` |
| `internal/markdown/links.go` | Optional — only if `ExtractLinks` needs to skip the new wikilink AST node |
| `internal/markdown/links_render.go` | Render `wikilinkNode` via the resolver: resolved → standard-link bytes, unresolved → broken-style bytes |
| `internal/tui/model.go` | New state fields, vault construction, diagnostics initialization |
| `internal/tui/keys.go` | New bindings: `b`, `B`, `?` |
| `internal/tui/input.go` | Handle new keys; modal-priority routing in `handleKey` |
| `internal/tui/view.go` | Geometry: bottom split when backlinks open; modal overlay; transient status row |
| `internal/tui/content.go` | `refreshContent` updates backlinks viewport too |
| `internal/tui/model_test.go`, `internal/tui/links_test.go` | Update existing tests if APIs shift |
| `internal/tui/keys.go` | Help footer key list |

---

## Stage 1 — Diagnostics foundation

The diagnostic stream has three observers (transient footer status, JSON-line log file, in-memory ring buffer). It must work before the vault uses it.

### Task 1: `vault.Diagnostics` interface

**Files:**
- Create: `internal/vault/diagnostics.go`

- [ ] **Step 1: Write the file**

```go
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
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./internal/vault/...`
Expected: Empty output, exit code 0.

- [ ] **Step 3: Commit**

```bash
git add internal/vault/diagnostics.go
git commit -m "feat(vault): add Diagnostics interface and Nop default"
```

### Task 2: TUI diagnostics ring buffer (recording sink)

The TUI owns the diagnostics state. The ring buffer is in-memory; the file log is appended to disk. Both are populated by the same Push call.

**Files:**
- Create: `internal/tui/diagnostics.go`
- Create: `internal/tui/diagnostics_test.go`

- [ ] **Step 1: Write the failing test for ring buffer behavior**

```go
package tui

import (
	"testing"
	"time"
)

func TestDiagnosticsRingBufferBoundedAndOrdered(t *testing.T) {
	d := newDiagnostics(testDiagOpts(t, ""))
	defer d.close()

	for i := 0; i < diagnosticsRingCap+50; i++ {
		d.Warn("entry")
	}

	entries := d.snapshot()
	if got, want := len(entries), diagnosticsRingCap; got != want {
		t.Fatalf("ring length: got %d want %d", got, want)
	}
	if entries[0].Severity != sevWarn {
		t.Fatalf("entry severity: got %v want warn", entries[0].Severity)
	}
}

func TestDiagnosticsRecordsTimestamp(t *testing.T) {
	d := newDiagnostics(testDiagOpts(t, ""))
	defer d.close()

	before := time.Now()
	d.Info("hello")
	after := time.Now()

	entries := d.snapshot()
	if len(entries) != 1 {
		t.Fatalf("snapshot len: got %d want 1", len(entries))
	}
	ts := entries[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Fatalf("timestamp %v not between %v and %v", ts, before, after)
	}
}

// testDiagOpts returns options pointing the file log at a temp file under
// the test's TempDir. If logPath is empty, file logging is disabled
// (covers the "no writable path" branch).
func testDiagOpts(t *testing.T, logPath string) diagOpts {
	t.Helper()
	return diagOpts{
		LogPath: logPath,
		Now:     time.Now,
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestDiagnostics -v`
Expected: FAIL — `newDiagnostics`, `diagOpts`, `diagnosticsRingCap`, `sevWarn`, `sevInfo`, etc. are undefined.

- [ ] **Step 3: Implement `internal/tui/diagnostics.go`**

```go
package tui

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// diagnosticsRingCap is the in-memory ring buffer size — what the log
// viewer modal displays. The on-disk log is unbounded and accepts
// everything that passes through Push.
const diagnosticsRingCap = 200

// severity is plumbed through so future diagnostics (render times,
// rebuild durations) can land at sevInfo without changing the API.
type severity int

const (
	sevInfo severity = iota
	sevWarn
	sevError
)

func (s severity) String() string {
	switch s {
	case sevInfo:
		return "info"
	case sevWarn:
		return "warn"
	case sevError:
		return "error"
	}
	return "unknown"
}

// diagEntry is one record in both the ring buffer and the JSON log.
type diagEntry struct {
	Timestamp time.Time `json:"ts"`
	Severity  severity  `json:"severity"`
	Message   string    `json:"message"`
}

// diagOpts configures the diagnostics sink.
type diagOpts struct {
	// LogPath is the file to append JSON-line records to. Empty disables
	// file logging without otherwise affecting the sink.
	LogPath string
	// Now is the time source — injected for tests.
	Now func() time.Time
}

// diagnostics is the TUI's diagnostic sink. It satisfies vault.Diagnostics
// (in spirit — the TUI imports vault, so the type assertion happens there).
type diagnostics struct {
	mu        sync.Mutex
	ring      []diagEntry
	ringStart int // index of oldest entry when ring is full
	ringFull  bool
	logFile   *os.File
	now       func() time.Time
	transient diagEntry // most recent — read by the footer
	hasTrans  bool
}

func newDiagnostics(opts diagOpts) *diagnostics {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	d := &diagnostics{
		ring: make([]diagEntry, 0, diagnosticsRingCap),
		now:  opts.Now,
	}
	if opts.LogPath != "" {
		// Best-effort file open. Failure leaves logFile nil — diagnostics
		// continue working in-memory. This is the "no writable path"
		// branch from the spec.
		if f, err := os.OpenFile(opts.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			d.logFile = f
		}
	}
	return d
}

func (d *diagnostics) Info(msg string)  { d.push(sevInfo, msg) }
func (d *diagnostics) Warn(msg string)  { d.push(sevWarn, msg) }
func (d *diagnostics) Error(msg string) { d.push(sevError, msg) }

func (d *diagnostics) push(sev severity, msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	entry := diagEntry{Timestamp: d.now(), Severity: sev, Message: msg}

	if len(d.ring) < diagnosticsRingCap {
		d.ring = append(d.ring, entry)
	} else {
		d.ring[d.ringStart] = entry
		d.ringStart = (d.ringStart + 1) % diagnosticsRingCap
		d.ringFull = true
	}

	d.transient = entry
	d.hasTrans = true

	if d.logFile != nil {
		if data, err := json.Marshal(entry); err == nil {
			d.logFile.Write(data)
			d.logFile.Write([]byte("\n"))
		}
	}
}

// snapshot returns a copy of the ring buffer in oldest-to-newest order.
func (d *diagnostics) snapshot() []diagEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]diagEntry, 0, len(d.ring))
	if !d.ringFull {
		out = append(out, d.ring...)
		return out
	}
	out = append(out, d.ring[d.ringStart:]...)
	out = append(out, d.ring[:d.ringStart]...)
	return out
}

// transientStatus returns the most recent entry, if any, for footer display.
// The caller decides when to clear it.
func (d *diagnostics) transientStatus() (diagEntry, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.transient, d.hasTrans
}

func (d *diagnostics) clearTransient() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hasTrans = false
}

func (d *diagnostics) close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.logFile != nil {
		err := d.logFile.Close()
		d.logFile = nil
		return err
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestDiagnostics -v`
Expected: PASS for both ring buffer and timestamp tests.

- [ ] **Step 5: Add file-logging test**

Append to `internal/tui/diagnostics_test.go`:

```go
import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func TestDiagnosticsWritesJSONLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	d := newDiagnostics(testDiagOpts(t, path))

	d.Warn("hello")
	d.Info("world")
	if err := d.close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	var entries []diagEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e diagEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("unmarshal: %v line=%q", err, scanner.Text())
		}
		entries = append(entries, e)
	}
	if got, want := len(entries), 2; got != want {
		t.Fatalf("entries: got %d want %d", got, want)
	}
	if !strings.HasPrefix(entries[0].Message, "hello") {
		t.Fatalf("first message: got %q", entries[0].Message)
	}
}

func TestDiagnosticsHandlesUnwritablePath(t *testing.T) {
	// A path under a non-existent directory should fail to open and
	// the sink should still record in memory.
	d := newDiagnostics(testDiagOpts(t, "/nonexistent-path-xyz/no/such/file.log"))
	defer d.close()

	d.Warn("still works")

	if got := len(d.snapshot()); got != 1 {
		t.Fatalf("ring entries with unwritable path: got %d want 1", got)
	}
}
```

- [ ] **Step 6: Run all diagnostics tests**

Run: `go test ./internal/tui/ -run TestDiagnostics -v`
Expected: PASS for all four tests.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/diagnostics.go internal/tui/diagnostics_test.go
git commit -m "feat(tui): add diagnostics ring buffer and JSON-line file logger"
```

### Task 3: Diagnostics log path resolver

Resolves the platform-appropriate log file path. Lives in TUI rather than vault because it's a UI concern.

**Files:**
- Modify: `internal/tui/diagnostics.go`
- Modify: `internal/tui/diagnostics_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/diagnostics_test.go`:

```go
import "runtime"

func TestDefaultLogPathHonorsXDG(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("XDG path is not the default on macOS")
	}
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-test-state")
	t.Setenv("HOME", "/tmp/home-test")

	got := defaultLogPath()
	want := "/tmp/xdg-test-state/hypogeum/hypogeum.log"
	if got != want {
		t.Fatalf("XDG path: got %q want %q", got, want)
	}
}

func TestDefaultLogPathFallsBackToLocalState(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Linux fallback is not used on macOS")
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/tmp/home-test")

	got := defaultLogPath()
	want := "/tmp/home-test/.local/state/hypogeum/hypogeum.log"
	if got != want {
		t.Fatalf("fallback path: got %q want %q", got, want)
	}
}

func TestDefaultLogPathMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}
	t.Setenv("HOME", "/tmp/home-test")

	got := defaultLogPath()
	want := "/tmp/home-test/Library/Logs/hypogeum/hypogeum.log"
	if got != want {
		t.Fatalf("macOS path: got %q want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run TestDefaultLogPath -v`
Expected: FAIL — `defaultLogPath` is undefined.

- [ ] **Step 3: Implement `defaultLogPath`**

Add to `internal/tui/diagnostics.go`:

```go
import (
	"path/filepath"
	"runtime"
)

// defaultLogPath returns the platform-conventional path for the
// hypogeum log file. The directory is *not* created here — the file
// open will create the directory tree if needed (see ensureLogDir).
//
// The path is best-effort: callers should treat any open failure as
// "file logging disabled" rather than fatal.
func defaultLogPath() string {
	home := os.Getenv("HOME")
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Logs", "hypogeum", "hypogeum.log")
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "hypogeum", "hypogeum.log")
	}
	return filepath.Join(home, ".local", "state", "hypogeum", "hypogeum.log")
}

// ensureLogDir creates the directory containing path if it doesn't
// exist. Errors are returned to the caller, which treats them as
// "disable file logging."
func ensureLogDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
```

Update `newDiagnostics` to call `ensureLogDir` before opening:

```go
if opts.LogPath != "" {
	if err := ensureLogDir(opts.LogPath); err == nil {
		if f, err := os.OpenFile(opts.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			d.logFile = f
		}
	}
}
```

- [ ] **Step 4: Run all diagnostics tests**

Run: `go test ./internal/tui/ -run TestDefaultLogPath -v && go test ./internal/tui/ -run TestDiagnostics -v`
Expected: PASS for all.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/diagnostics.go internal/tui/diagnostics_test.go
git commit -m "feat(tui): resolve platform-conventional log path with XDG fallback"
```

---

## Stage 2 — Vault scaffolding

Empty package skeleton with the public surface from the spec. No real indexing yet — just enough for the TUI to wire it in.

### Task 4: `Vault` struct and stub methods

**Files:**
- Create: `internal/vault/vault.go`
- Create: `internal/vault/vault_test.go`

- [ ] **Step 1: Write the failing test**

```go
package vault

import (
	"path/filepath"
	"testing"
)

func TestBuildEmptyVault(t *testing.T) {
	dir := t.TempDir()
	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if v == nil {
		t.Fatalf("Build returned nil vault")
	}
	if got := v.Backlinks(filepath.Join(dir, "anything.md")); len(got) != 0 {
		t.Fatalf("empty vault Backlinks: got %d want 0", len(got))
	}
}

func TestResolveOnEmptyVault(t *testing.T) {
	dir := t.TempDir()
	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if path, ok := v.Resolve(filepath.Join(dir, "from.md"), "missing", "", ""); ok {
		t.Fatalf("expected unresolved, got %q", path)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/vault/ -v`
Expected: FAIL — `Build`, `Vault`, `Backlinks`, `Resolve` undefined.

- [ ] **Step 3: Write `internal/vault/vault.go`**

```go
package vault

import (
	"path/filepath"
	"sync"
)

// Vault is the in-memory index of a directory of markdown files.
//
// State:
//   - files: forward index, keyed by absolute path. Each entry's refs
//     are this file's outgoing references (wikilinks + standard links).
//   - names: name index, keyed by lowercased basename without extension.
//     Used by Resolve to find a file matching a wikilink target.
//
// The reverse index (which files link *to* a given path) is computed
// on demand from `files` — see Backlinks.
type Vault struct {
	root  string
	mu    sync.RWMutex
	files map[string]*fileEntry
	names map[string][]string
	diag  Diagnostics
}

type fileEntry struct {
	path string
	refs []reference
}

type referenceKind int

const (
	refWikilink referenceKind = iota
	refStdLink
)

type reference struct {
	kind        referenceKind
	target      string // raw [[Target]] (wikilink) or href (stdlink)
	resolved    string // absolute path of the target file, "" if unresolved
	heading     string
	block       string
	alias       string
	displayText string
	snippet     string
	line        int
}

// Backlink is one cross-reference *to* a given file. Returned by Backlinks
// for the TUI to render in the persistent pane and modal.
type Backlink struct {
	SourceFile  string
	DisplayText string
	Snippet     string
	Line        int
	Kind        BacklinkKind
}

type BacklinkKind int

const (
	BacklinkWikilink BacklinkKind = iota
	BacklinkStdLink
)

// Build walks root and indexes every .md file's wikilinks and standard
// markdown links. The diag sink receives non-fatal issues; pass
// NopDiagnostics for tests that don't care.
//
// Returns a Vault even when individual files fail to parse — those
// emit a diagnostic and are skipped. A fatal error (root unreadable)
// returns (nil, err).
func Build(root string, diag Diagnostics) (*Vault, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	v := &Vault{
		root:  abs,
		files: make(map[string]*fileEntry),
		names: make(map[string][]string),
		diag:  diag,
	}
	// Indexing populated in a later task. For now, Build returns an
	// empty vault — the TUI can wire it in without behavior changes.
	return v, nil
}

// RefreshFile re-parses one file's outgoing references and updates
// both indexes. Called on watch.FileModified.
func (v *Vault) RefreshFile(path string) error {
	// Implementation in Stage 6.
	return nil
}

// Rebuild re-walks the entire root. Called on watch.StructureChanged.
func (v *Vault) Rebuild() error {
	// Implementation in Stage 6.
	return nil
}

// Resolve returns the absolute path the wikilink target resolves to,
// or ("", false) if no file matches. fromFile is the file containing
// the wikilink (used for proximity tiebreaking when multiple files
// share a basename).
func (v *Vault) Resolve(fromFile, name, heading, block string) (path string, ok bool) {
	// Implementation in Stage 4.
	return "", false
}

// Backlinks returns every reference *to* path in document order across
// files. Includes both wikilink and standard-markdown-link references.
func (v *Vault) Backlinks(path string) []Backlink {
	// Implementation in Stage 4.
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/vault/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/vault/vault.go internal/vault/vault_test.go
git commit -m "feat(vault): scaffold Vault struct and public API surface"
```

### Task 5: Wire vault into `tui.New`

The vault is built at startup. Construction is best-effort — a build error makes vault `nil` and the TUI continues without backlinks (mirroring the watcher's graceful degradation).

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/content.go` (no changes needed yet, but verify it compiles)

- [ ] **Step 1: Write a failing model test**

Append to `internal/tui/model_test.go`:

```go
func TestNewBuildsVault(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.vault == nil {
		t.Fatalf("expected vault to be constructed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestNewBuildsVault -v`
Expected: FAIL — `m.vault` undefined.

- [ ] **Step 3: Add fields and construction**

In `internal/tui/model.go`, inside the `Model` struct, add (next to `watcher`):

```go
vault *vault.Vault
diag  *diagnostics
```

Add the import:

```go
"github.com/wilkes/hypogeum/internal/vault"
```

In `New`, after the watcher initialization, add:

```go
m.diag = newDiagnostics(diagOpts{LogPath: defaultLogPath()})
if v, err := vault.Build(root, m.diag); err == nil {
	m.vault = v
} else {
	m.diag.Warn("vault build failed: " + err.Error())
}
if m.watcher == nil {
	m.diag.Warn("filesystem watcher unavailable; live updates disabled")
}
```

The watcher block already exists; the `if m.watcher == nil` block is added *after* the existing `if w, err := watch.New(root); err == nil { m.watcher = w }` block.

- [ ] **Step 4: Run all TUI tests**

Run: `go test ./internal/tui/ -v`
Expected: PASS — including the new `TestNewBuildsVault`.

- [ ] **Step 5: Build the binary to confirm wiring compiles end-to-end**

Run: `go build ./cmd/hypogeum`
Expected: Empty output, exit code 0.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat(tui): construct vault and diagnostics in tui.New"
```

---

## Stage 3 — Wikilink parser

Goldmark inline parser extension that recognizes `[[Name]]`, `[[Name|alias]]`, `[[Name#heading]]`, `[[Name^block]]`. The output is a private `wikilinkNode` AST type with the parsed components on it.

### Task 6: `wikilinkNode` AST type

**Files:**
- Create: `internal/vault/parser.go`
- Create: `internal/vault/parser_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package vault

import (
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func parseWith(src string) ast.Node {
	md := goldmark.New(goldmark.WithExtensions(WikilinkExtension))
	return md.Parser().Parse(text.NewReader([]byte(src)))
}

func findFirstWikilink(n ast.Node) *wikilinkNode {
	var found *wikilinkNode
	_ = ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if w, ok := n.(*wikilinkNode); ok {
			found = w
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

func TestWikilinkParse_Bare(t *testing.T) {
	doc := parseWith("see [[Foo]] for more.")
	w := findFirstWikilink(doc)
	if w == nil {
		t.Fatalf("no wikilink parsed")
	}
	if w.Name != "Foo" || w.Alias != "" || w.Heading != "" || w.Block != "" {
		t.Fatalf("got %+v", w)
	}
}

func TestWikilinkParse_Aliased(t *testing.T) {
	doc := parseWith("see [[Foo|the foo]] for more.")
	w := findFirstWikilink(doc)
	if w == nil {
		t.Fatalf("no wikilink parsed")
	}
	if w.Name != "Foo" || w.Alias != "the foo" {
		t.Fatalf("got %+v", w)
	}
}

func TestWikilinkParse_Heading(t *testing.T) {
	doc := parseWith("see [[Foo#Section Two]] for more.")
	w := findFirstWikilink(doc)
	if w == nil {
		t.Fatalf("no wikilink parsed")
	}
	if w.Name != "Foo" || w.Heading != "Section Two" {
		t.Fatalf("got %+v", w)
	}
}

func TestWikilinkParse_Block(t *testing.T) {
	doc := parseWith("see [[Foo^abc123]] for more.")
	w := findFirstWikilink(doc)
	if w == nil {
		t.Fatalf("no wikilink parsed")
	}
	if w.Name != "Foo" || w.Block != "abc123" {
		t.Fatalf("got %+v", w)
	}
}

func TestWikilinkParse_HeadingWithAlias(t *testing.T) {
	doc := parseWith("see [[Foo#Section|that section]] for more.")
	w := findFirstWikilink(doc)
	if w == nil {
		t.Fatalf("no wikilink parsed")
	}
	if w.Name != "Foo" || w.Heading != "Section" || w.Alias != "that section" {
		t.Fatalf("got %+v", w)
	}
}

func TestWikilinkParse_NotAWikilink(t *testing.T) {
	// A single-bracket link must not be parsed as a wikilink.
	doc := parseWith("see [Foo](bar.md) for more.")
	if w := findFirstWikilink(doc); w != nil {
		t.Fatalf("standard link parsed as wikilink: %+v", w)
	}
}

func TestWikilinkParse_UnclosedNotConsumed(t *testing.T) {
	// "[[Foo" with no closing brackets is left to the standard parser
	// (which renders it as text). The wikilink parser must not consume
	// it greedily.
	doc := parseWith("see [[Foo for more.")
	if w := findFirstWikilink(doc); w != nil {
		t.Fatalf("unclosed wikilink parsed: %+v", w)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/vault/ -run TestWikilinkParse -v`
Expected: FAIL — `WikilinkExtension`, `wikilinkNode` undefined.

- [ ] **Step 3: Implement `internal/vault/parser.go`**

```go
package vault

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// wikilinkNode is the AST type produced by the wikilink inline parser.
// It carries the raw components of a [[Name#Heading^Block|Alias]]
// syntax as parsed; resolution happens later in the renderer/indexer.
type wikilinkNode struct {
	ast.BaseInline
	Name    string
	Heading string
	Block   string
	Alias   string
}

var kindWikilink = ast.NewNodeKind("Wikilink")

func (w *wikilinkNode) Kind() ast.NodeKind { return kindWikilink }
func (w *wikilinkNode) Dump(source []byte, level int) {
	ast.DumpHelper(w, source, level, map[string]string{
		"Name":    w.Name,
		"Heading": w.Heading,
		"Block":   w.Block,
		"Alias":   w.Alias,
	}, nil)
}

// wikilinkParser is a goldmark inline parser that triggers on '[' and
// matches the full [[...]] form. Goldmark's standard link parser also
// triggers on '[' but only for a single bracket; by registering this
// parser at a higher priority, ours runs first and consumes [[ ... ]]
// before the standard parser can mistake it for two adjacent links.
type wikilinkParser struct{}

// Trigger returns the bytes that activate this parser.
func (wikilinkParser) Trigger() []byte { return []byte{'['} }

// Parse implements the inline parser interface. It returns nil if the
// input at the current position isn't a [[...]] — leaving the standard
// link parser free to handle a normal [link].
func (wikilinkParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, segment := block.PeekLine()
	if len(line) < 4 || line[0] != '[' || line[1] != '[' {
		return nil
	}
	// Find closing ]]. Don't cross newlines — wikilinks are inline only.
	end := -1
	for i := 2; i+1 < len(line); i++ {
		if line[i] == '\n' {
			break
		}
		if line[i] == ']' && line[i+1] == ']' {
			end = i
			break
		}
	}
	if end < 0 {
		return nil
	}

	body := string(line[2:end])
	w := parseWikilinkBody(body)
	if w == nil {
		return nil
	}

	// Advance the reader past the closing ]].
	block.Advance(end + 2)
	_ = segment
	return w
}

// parseWikilinkBody splits the inside of [[...]] into its components.
// Returns nil if the body is empty or otherwise malformed.
func parseWikilinkBody(body string) *wikilinkNode {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}

	w := &wikilinkNode{}

	// Pipe splits "name-with-extras|alias" first.
	if i := strings.Index(body, "|"); i >= 0 {
		w.Alias = strings.TrimSpace(body[i+1:])
		body = body[:i]
	}

	// "^block" is allowed after the name, with or without a heading.
	if i := strings.Index(body, "^"); i >= 0 {
		w.Block = strings.TrimSpace(body[i+1:])
		body = body[:i]
	}

	// "#heading" is everything between name and ^/| boundaries.
	if i := strings.Index(body, "#"); i >= 0 {
		w.Heading = strings.TrimSpace(body[i+1:])
		body = body[:i]
	}

	w.Name = strings.TrimSpace(body)
	if w.Name == "" {
		return nil
	}
	return w
}

// wikilinkExt registers wikilinkParser with the parser at a high priority.
// The priority value is chosen to run *before* goldmark's built-in link
// parser (priority 200) so [[...]] is consumed before [link].
type wikilinkExt struct{}

func (wikilinkExt) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(wikilinkParser{}, 102),
	))
}

// WikilinkExtension is the goldmark.Extender that adds [[wikilink]]
// support to a goldmark instance.
var WikilinkExtension = wikilinkExt{}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/vault/ -run TestWikilinkParse -v`
Expected: PASS for all seven cases.

- [ ] **Step 5: Commit**

```bash
git add internal/vault/parser.go internal/vault/parser_test.go
git commit -m "feat(vault): add goldmark wikilink inline parser"
```

### Task 7: Verify standard link parsing is not regressed

A goldmark extension at the wrong priority can break `[link]` parsing. This task captures golden output for a representative document before/after.

**Files:**
- Modify: `internal/vault/parser_test.go`

- [ ] **Step 1: Add a regression test**

Append:

```go
func TestStandardLinksUnchangedByWikilinkExtension(t *testing.T) {
	src := `# Title

A paragraph with a [normal link](other.md) and a [link with title](x.md "title").

- list with [a link](y.md)
- list with [[Wikilink]]

[autolink test](https://example.com).
`
	withoutExt := goldmark.New().Parser().Parse(text.NewReader([]byte(src)))
	withExt := goldmark.New(goldmark.WithExtensions(WikilinkExtension)).Parser().Parse(text.NewReader([]byte(src)))

	stdLinks := func(n ast.Node) []string {
		var out []string
		_ = ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			if l, ok := n.(*ast.Link); ok {
				out = append(out, string(l.Destination))
			}
			return ast.WalkContinue, nil
		})
		return out
	}

	got := stdLinks(withExt)
	want := stdLinks(withoutExt)
	if len(got) != len(want) {
		t.Fatalf("standard link count changed: got %d want %d (got=%v want=%v)", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("link %d destination changed: got %q want %q", i, got[i], want[i])
		}
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/vault/ -run TestStandardLinksUnchangedByWikilinkExtension -v`
Expected: PASS — the wikilink extension must not consume or alter standard links.

- [ ] **Step 3: Commit**

```bash
git add internal/vault/parser_test.go
git commit -m "test(vault): assert wikilink extension doesn't change standard link parsing"
```

---

## Stage 4 — Indexer

`Build` walks the tree, parses each file with the wikilink extension, collects both wikilink and standard-link references into the forward index, and populates the basename index. `Backlinks` and `Resolve` then read from those indexes.

### Task 8: File walk and reference collection

**Files:**
- Modify: `internal/vault/vault.go`
- Modify: `internal/vault/vault_test.go`

- [ ] **Step 1: Write the failing test**

Append to `vault_test.go`:

```go
import (
	"os"
	"path/filepath"
)

func writeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	return full
}

func TestBuildIndexesFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "links to [[b]] and [c](c.md).")
	writeFile(t, dir, "b.md", "i am b.")
	writeFile(t, dir, "c.md", "i am c.")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := v.fileCount(); got != 3 {
		t.Fatalf("fileCount: got %d want 3", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/vault/ -run TestBuildIndexesFiles -v`
Expected: FAIL — `fileCount` undefined and Build doesn't actually populate.

- [ ] **Step 3: Add file walk to `Build`**

In `internal/vault/vault.go`, replace `Build` with:

```go
import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func Build(root string, diag Diagnostics) (*Vault, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	v := &Vault{
		root:  abs,
		files: make(map[string]*fileEntry),
		names: make(map[string][]string),
		diag:  diag,
	}
	if err := v.walkAndIndex(); err != nil {
		return nil, err
	}
	return v, nil
}

// walkAndIndex populates v.files and v.names by walking v.root.
// Per-file parse failures emit a Warn diagnostic and are skipped.
// A walk-level error (root unreadable) is fatal.
func (v *Vault) walkAndIndex() error {
	return filepath.WalkDir(v.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != v.root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || !isMarkdownExt(d.Name()) {
			return nil
		}
		v.indexFile(path)
		return nil
	})
}

// indexFile parses one file and stores its outgoing references and
// name-index entry. Errors emit a Warn diagnostic and leave the file
// out of the index. The caller (walkAndIndex / RefreshFile) decides
// whether to drop or replace the existing entry.
func (v *Vault) indexFile(path string) {
	src, err := os.ReadFile(path)
	if err != nil {
		v.diag.Warn("vault: read " + path + ": " + err.Error())
		return
	}
	refs := extractReferences(string(src), path)

	v.mu.Lock()
	defer v.mu.Unlock()

	v.files[path] = &fileEntry{path: path, refs: refs}

	key := nameKey(path)
	// Keep names index unique-by-path: drop any prior occurrence of this
	// path under this key (in case of rename-in-place edge cases).
	existing := v.names[key]
	deduped := existing[:0]
	for _, p := range existing {
		if p != path {
			deduped = append(deduped, p)
		}
	}
	v.names[key] = append(deduped, path)
}

// extractReferences parses src as markdown (with the wikilink extension)
// and returns one reference per outgoing link, in document order.
// Standard ast.Link nodes become refStdLink entries; wikilinkNode
// instances become refWikilink entries.
//
// Snippet extraction and resolution against the vault are *not* done
// here — they happen in later stages once those subsystems exist.
func extractReferences(src, fromPath string) []reference {
	md := goldmark.New(goldmark.WithExtensions(WikilinkExtension))
	doc := md.Parser().Parse(text.NewReader([]byte(src)))

	var refs []reference
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *wikilinkNode:
			disp := v.Alias
			if disp == "" {
				disp = v.Name
			}
			refs = append(refs, reference{
				kind:        refWikilink,
				target:      v.Name,
				heading:     v.Heading,
				block:       v.Block,
				alias:       v.Alias,
				displayText: disp,
			})
			return ast.WalkSkipChildren, nil
		case *ast.Link:
			refs = append(refs, reference{
				kind:        refStdLink,
				target:      string(v.Destination),
				displayText: linkText(v, []byte(src)),
			})
			return ast.WalkSkipChildren, nil
		case *ast.Image:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return refs
}

// linkText returns the visible text under a *ast.Link.
func linkText(n ast.Node, source []byte) string {
	var out []byte
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			out = append(out, t.Segment.Value(source)...)
			continue
		}
		out = append(out, []byte(linkText(c, source))...)
	}
	return string(out)
}

// nameKey is the basename without extension, lowercased — the key used
// in v.names for wikilink lookups.
func nameKey(path string) string {
	name := filepath.Base(path)
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[:i]
	}
	return strings.ToLower(name)
}

// isMarkdownExt mirrors internal/tree's set; duplicated to keep vault
// independent of tree.
func isMarkdownExt(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	}
	return false
}

// fileCount is exposed for tests. Not part of the public API.
func (v *Vault) fileCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.files)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/vault/ -run TestBuildIndexesFiles -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/vault/vault.go internal/vault/vault_test.go
git commit -m "feat(vault): walk root and index outgoing references per file"
```

### Task 9: `Backlinks` query

The reverse index is computed on demand: iterate `files`, find references whose resolved target equals the queried path, and emit a `Backlink` per occurrence.

**Files:**
- Modify: `internal/vault/vault.go`
- Modify: `internal/vault/vault_test.go`

- [ ] **Step 1: Write the failing test**

Append to `vault_test.go`:

```go
func TestBacklinksFromStandardAndWikilinks(t *testing.T) {
	dir := t.TempDir()
	bAbs, _ := filepath.Abs(filepath.Join(dir, "b.md"))
	writeFile(t, dir, "a.md", "links to [[b]] and [b again](b.md).")
	writeFile(t, dir, "b.md", "i am b.")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// We expect 2 backlinks to b.md from a.md (one wikilink, one stdlink),
	// even though Resolve isn't fully implemented yet — the indexer
	// must pre-resolve standard-link hrefs to absolute paths during
	// indexFile so backlinks queries work without re-parsing.
	got := v.Backlinks(bAbs)
	if len(got) != 2 {
		t.Fatalf("Backlinks(b): got %d want 2 (%+v)", len(got), got)
	}
	kinds := []BacklinkKind{got[0].Kind, got[1].Kind}
	hasWiki, hasStd := false, false
	for _, k := range kinds {
		if k == BacklinkWikilink {
			hasWiki = true
		}
		if k == BacklinkStdLink {
			hasStd = true
		}
	}
	if !hasWiki || !hasStd {
		t.Fatalf("expected both wikilink and stdlink backlinks, got %v", kinds)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/vault/ -run TestBacklinksFromStandardAndWikilinks -v`
Expected: FAIL — `Backlinks` returns nil.

- [ ] **Step 3: Resolve standard links during indexing**

Standard markdown links carry an explicit href. Resolve them to absolute paths during `indexFile` so `Backlinks` is a simple loop. Wikilinks need the basename index, which only exists after the full walk — so we resolve those in a second pass.

In `internal/vault/vault.go`, replace `Build` with:

```go
func Build(root string, diag Diagnostics) (*Vault, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	v := &Vault{
		root:  abs,
		files: make(map[string]*fileEntry),
		names: make(map[string][]string),
		diag:  diag,
	}
	if err := v.walkAndIndex(); err != nil {
		return nil, err
	}
	v.resolveAllRefs()
	return v, nil
}
```

In `extractReferences`, populate `resolved` for stdlinks immediately:

```go
case *ast.Link:
	href := string(v.Destination)
	resolved := resolveStdLink(fromPath, href)
	refs = append(refs, reference{
		kind:        refStdLink,
		target:      href,
		resolved:    resolved,
		displayText: linkText(v, []byte(src)),
	})
	return ast.WalkSkipChildren, nil
```

Add `resolveStdLink`:

```go
import "net/url"

// resolveStdLink resolves a standard markdown link's href against
// the file containing it. Returns "" if the href is empty, an
// external URL, or a same-document anchor — none of which produce
// a backlink to another file in the vault.
func resolveStdLink(fromPath, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return ""
	}
	u, err := url.Parse(href)
	if err == nil && u.Scheme != "" && u.Scheme != "file" {
		return ""
	}
	target := href
	if u != nil && u.Path != "" {
		target = u.Path
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(fromPath), target)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return ""
	}
	return abs
}
```

Add `resolveAllRefs` (a no-op for now — full implementation comes with Resolve in Task 10):

```go
// resolveAllRefs fills in the `resolved` field for every wikilink
// reference now that the names index is fully populated. Standard
// links are already resolved during indexFile.
//
// Implemented in Task 10 once Resolve is available.
func (v *Vault) resolveAllRefs() {
	// Stub. Wikilink resolution lands in Task 10.
}
```

Add `Backlinks`:

```go
func (v *Vault) Backlinks(path string) []Backlink {
	v.mu.RLock()
	defer v.mu.RUnlock()

	abs, _ := filepath.Abs(path)
	var out []Backlink
	for src, entry := range v.files {
		for _, ref := range entry.refs {
			if ref.resolved == "" || ref.resolved != abs {
				continue
			}
			kind := BacklinkStdLink
			if ref.kind == refWikilink {
				kind = BacklinkWikilink
			}
			out = append(out, Backlink{
				SourceFile:  src,
				DisplayText: ref.displayText,
				Snippet:     ref.snippet,
				Line:        ref.line,
				Kind:        kind,
			})
		}
	}
	// Stable order: sort by source file, then by line, so test fixtures
	// don't depend on map iteration order.
	sortBacklinks(out)
	return out
}
```

Add the import and helper:

```go
import "sort"

func sortBacklinks(b []Backlink) {
	sort.Slice(b, func(i, j int) bool {
		if b[i].SourceFile != b[j].SourceFile {
			return b[i].SourceFile < b[j].SourceFile
		}
		return b[i].Line < b[j].Line
	})
}
```

- [ ] **Step 4: Run test to verify it passes (stdlink half)**

Run: `go test ./internal/vault/ -run TestBacklinksFromStandardAndWikilinks -v`
Expected: FAIL still — wikilink backlinks not yet resolved (covered in Task 10), but the stdlink half should work. If the test fails because *only* the stdlink shows up (count 1 instead of 2), that's expected and the next task fixes it.

- [ ] **Step 5: Commit (stdlink half complete)**

```bash
git add internal/vault/vault.go
git commit -m "feat(vault): index standard markdown links and add Backlinks query"
```

### Task 10: `Resolve` and second-pass wikilink resolution

The basename index lookup with proximity tiebreaker.

**Files:**
- Create: `internal/vault/resolver.go`
- Create: `internal/vault/resolver_test.go`
- Modify: `internal/vault/vault.go`

- [ ] **Step 1: Write the failing tests**

```go
package vault

import (
	"path/filepath"
	"testing"
)

func TestResolve_ExactBasenameCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Foo.md", "i am foo")
	writeFile(t, dir, "from.md", "links to [[FOO]]")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	from, _ := filepath.Abs(filepath.Join(dir, "from.md"))
	want, _ := filepath.Abs(filepath.Join(dir, "Foo.md"))

	got, ok := v.Resolve(from, "FOO", "", "")
	if !ok {
		t.Fatalf("Resolve returned ok=false")
	}
	if got != want {
		t.Fatalf("Resolve: got %q want %q", got, want)
	}
}

func TestResolve_MissReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "from.md", "links to [[Nonexistent]]")
	v, _ := Build(dir, NopDiagnostics{})
	from, _ := filepath.Abs(filepath.Join(dir, "from.md"))

	if _, ok := v.Resolve(from, "Nonexistent", "", ""); ok {
		t.Fatalf("expected unresolved")
	}
}

func TestResolve_ProximityTiebreaker(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "near/from.md", "links to [[shared]]")
	writeFile(t, dir, "near/shared.md", "near version")
	writeFile(t, dir, "far/away/shared.md", "far version")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	from, _ := filepath.Abs(filepath.Join(dir, "near/from.md"))
	want, _ := filepath.Abs(filepath.Join(dir, "near/shared.md"))

	got, ok := v.Resolve(from, "shared", "", "")
	if !ok {
		t.Fatalf("ok=false")
	}
	if got != want {
		t.Fatalf("expected near version %q, got %q", want, got)
	}
}

func TestBacklinksFromWikilinksAfterResolve(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "links to [[b]] and [b again](b.md).")
	writeFile(t, dir, "b.md", "i am b.")
	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	bAbs, _ := filepath.Abs(filepath.Join(dir, "b.md"))
	got := v.Backlinks(bAbs)
	if len(got) != 2 {
		t.Fatalf("Backlinks: got %d want 2 (%+v)", len(got), got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/vault/ -run "TestResolve_|TestBacklinksFromWikilinksAfterResolve" -v`
Expected: FAIL — Resolve returns "", false.

- [ ] **Step 3: Implement `internal/vault/resolver.go`**

```go
package vault

import (
	"path/filepath"
	"sort"
	"strings"
)

// Resolve returns the absolute path the wikilink target resolves to,
// or ("", false) if no file matches. fromFile is the file containing
// the wikilink; it's used as the proximity reference when multiple
// files share a basename.
//
// The lookup is case-insensitive on basename (without extension).
// The block argument is recorded in references but not used for
// resolution in Phase 1 — the caller still gets the file path.
func (v *Vault) Resolve(fromFile, name, heading, block string) (string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	candidates := v.names[strings.ToLower(name)]
	if len(candidates) == 0 {
		return "", false
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}

	// Proximity tiebreaker: prefer the candidate whose path is shortest
	// relative to fromFile. Falls back to lexical order on ties.
	type scored struct {
		path string
		dist int
	}
	scoredCands := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		rel, err := filepath.Rel(filepath.Dir(fromFile), c)
		if err != nil {
			rel = c
		}
		scoredCands = append(scoredCands, scored{path: c, dist: len(rel)})
	}
	sort.Slice(scoredCands, func(i, j int) bool {
		if scoredCands[i].dist != scoredCands[j].dist {
			return scoredCands[i].dist < scoredCands[j].dist
		}
		return scoredCands[i].path < scoredCands[j].path
	})
	return scoredCands[0].path, true
}
```

Replace the stub `resolveAllRefs` in `internal/vault/vault.go`:

```go
// resolveAllRefs fills in the `resolved` field for every wikilink
// reference now that the names index is fully populated. Standard
// links are already resolved during indexFile.
func (v *Vault) resolveAllRefs() {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, entry := range v.files {
		for i := range entry.refs {
			if entry.refs[i].kind != refWikilink {
				continue
			}
			// resolveLocked: same algorithm as Resolve, but assumes
			// the lock is already held (we're under v.mu.Lock).
			path, ok := v.resolveLocked(entry.path, entry.refs[i].target)
			if ok {
				entry.refs[i].resolved = path
			}
		}
	}
}

// resolveLocked is Resolve without the read lock — used by
// resolveAllRefs which already holds the write lock.
func (v *Vault) resolveLocked(fromFile, name string) (string, bool) {
	candidates := v.names[strings.ToLower(name)]
	if len(candidates) == 0 {
		return "", false
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	type scored struct {
		path string
		dist int
	}
	scoredCands := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		rel, err := filepath.Rel(filepath.Dir(fromFile), c)
		if err != nil {
			rel = c
		}
		scoredCands = append(scoredCands, scored{path: c, dist: len(rel)})
	}
	sort.Slice(scoredCands, func(i, j int) bool {
		if scoredCands[i].dist != scoredCands[j].dist {
			return scoredCands[i].dist < scoredCands[j].dist
		}
		return scoredCands[i].path < scoredCands[j].path
	})
	return scoredCands[0].path, true
}
```

- [ ] **Step 4: Run all vault tests**

Run: `go test ./internal/vault/ -v`
Expected: PASS — including the proximity, miss, and after-resolve backlinks tests.

- [ ] **Step 5: Commit**

```bash
git add internal/vault/resolver.go internal/vault/resolver_test.go internal/vault/vault.go
git commit -m "feat(vault): resolve wikilinks via basename index with proximity tiebreaker"
```

### Task 11: Per-file parse failure emits a Warn diagnostic

**Files:**
- Modify: `internal/vault/vault.go`
- Modify: `internal/vault/vault_test.go`

- [ ] **Step 1: Write the failing test**

Append to `vault_test.go`:

```go
type recordingDiag struct {
	infos, warns, errors []string
}

func (r *recordingDiag) Info(m string)  { r.infos = append(r.infos, m) }
func (r *recordingDiag) Warn(m string)  { r.warns = append(r.warns, m) }
func (r *recordingDiag) Error(m string) { r.errors = append(r.errors, m) }

func TestBuildEmitsWarnOnUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ok.md", "fine")
	bad := writeFile(t, dir, "bad.md", "fine")
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Skipf("chmod 000 not supported: %v", err)
	}
	defer os.Chmod(bad, 0o644)

	r := &recordingDiag{}
	v, err := Build(dir, r)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if v == nil {
		t.Fatalf("Build returned nil")
	}
	if len(r.warns) == 0 {
		t.Fatalf("expected a Warn diagnostic for unreadable file, got none")
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/vault/ -run TestBuildEmitsWarnOnUnreadableFile -v`
Expected: PASS — the existing `indexFile` already emits a Warn for read errors.

If skipped (root user / unusual filesystem), that's fine — the path is exercised by the existing indexFile code.

- [ ] **Step 3: Commit**

```bash
git add internal/vault/vault_test.go
git commit -m "test(vault): assert per-file read failures emit Warn diagnostic"
```

---

## Stage 5 — Resolver + markdown integration

The renderer now turns wikilink AST nodes into either standard markdown link bytes (resolved) or styled placeholders (unresolved). Standard links keep working unchanged.

### Task 12: Add `Resolver` interface to `internal/markdown`

**Files:**
- Modify: `internal/markdown/render.go`
- Create: `internal/markdown/resolver.go`

- [ ] **Step 1: Add the `Resolver` interface**

Create `internal/markdown/resolver.go`:

```go
package markdown

// Resolver looks up a wikilink target by name and returns the
// absolute path of the file it resolves to.
//
// `internal/vault.Vault` implements this. The interface is defined
// here so this package can be tested with a fake — it does not
// import vault.
type Resolver interface {
	Resolve(fromFile, name, heading, block string) (path string, ok bool)
}

// nopResolver returns ("", false) for every lookup. Used when no
// resolver is configured — wikilinks then render as broken.
type nopResolver struct{}

func (nopResolver) Resolve(string, string, string, string) (string, bool) {
	return "", false
}
```

- [ ] **Step 2: Build to verify it compiles**

Run: `go build ./internal/markdown/...`
Expected: Empty output, exit code 0.

- [ ] **Step 3: Commit**

```bash
git add internal/markdown/resolver.go
git commit -m "feat(markdown): add Resolver interface for wikilink lookup"
```

### Task 13: `Renderer` accepts options including `WithResolver`

We change `NewRenderer(width int)` to `NewRenderer(width int, opts ...Option)`. Existing callers pass no options and get unchanged behavior.

**Files:**
- Modify: `internal/markdown/render.go`

- [ ] **Step 1: Add Option type and update NewRenderer**

Replace the `NewRenderer` and `Renderer` definitions in `internal/markdown/render.go` with:

```go
// Option configures a Renderer.
type Option func(*Renderer)

// WithResolver makes wikilink AST nodes resolve via r. If unset,
// wikilinks always render as broken (which is fine for unit tests
// of the markdown package alone).
func WithResolver(r Resolver) Option {
	return func(rr *Renderer) { rr.resolver = r }
}

// Renderer renders markdown to ANSI-styled terminal output.
type Renderer struct {
	g            *glamour.TermRenderer
	instrumented *glamour.TermRenderer

	resolver Resolver
	fromFile string // mutated per-render via SetFromFile; not safe across goroutines
}

// NewRenderer constructs a Renderer with the given output width.
func NewRenderer(width int, opts ...Option) (*Renderer, error) {
	if width < 20 {
		width = 80
	}
	g, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithStyles(hypogeumStyle(width)),
	)
	if err != nil {
		return nil, fmt.Errorf("init glamour: %w", err)
	}

	instrumented, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithStyles(linkInstrumentationStyles(width)),
	)
	if err != nil {
		return nil, fmt.Errorf("init instrumented glamour: %w", err)
	}

	r := &Renderer{
		g:            g,
		instrumented: instrumented,
		resolver:     nopResolver{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

// SetFromFile sets the file path used to resolve wikilink targets
// for the next render. Must be called before RenderWithLinks for
// each new file. The renderer is not safe for concurrent use across
// files; one renderer per goroutine.
func (r *Renderer) SetFromFile(path string) {
	r.fromFile = path
}
```

- [ ] **Step 2: Build to verify nothing else broke**

Run: `go build ./...`
Expected: Empty output, exit code 0.

- [ ] **Step 3: Run all tests**

Run: `go test ./...`
Expected: PASS — existing tests must keep passing.

- [ ] **Step 4: Commit**

```bash
git add internal/markdown/render.go
git commit -m "feat(markdown): NewRenderer accepts options; add WithResolver and SetFromFile"
```

### Task 14: Pre-process wikilinks before Glamour

The cleanest integration point is to *transform* the source markdown before Glamour sees it: walk the AST, for each `wikilinkNode`, splice in the equivalent standard markdown link (resolved) or a styled placeholder (unresolved). Glamour then renders standard markdown unchanged.

**Files:**
- Modify: `internal/markdown/links_render.go`
- Create: `internal/markdown/wikilink_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/markdown/wikilink_test.go`:

```go
package markdown

import (
	"strings"
	"testing"
)

type fakeResolver struct {
	answers map[string]string
}

func (f fakeResolver) Resolve(fromFile, name, heading, block string) (string, bool) {
	v, ok := f.answers[name]
	return v, ok
}

func TestRenderWithLinks_ResolvedWikilinkBecomesLink(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(fakeResolver{
		answers: map[string]string{"Foo": "/abs/foo.md"},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	r.SetFromFile("/abs/source.md")

	out, links, err := r.RenderWithLinks("see [[Foo]] above.", "/abs/source.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("links: got %d want 1 (%v)", len(links), links)
	}
	if links[0].Resolved.Kind != LinkLocalFile {
		t.Fatalf("expected LinkLocalFile, got %v", links[0].Resolved.Kind)
	}
	if links[0].Resolved.Target != "/abs/foo.md" {
		t.Fatalf("target: got %q want /abs/foo.md", links[0].Resolved.Target)
	}
	if !strings.Contains(out, "Foo") {
		t.Fatalf("rendered output missing display text: %q", out)
	}
}

func TestRenderWithLinks_UnresolvedWikilinkIsBroken(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(fakeResolver{}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	r.SetFromFile("/abs/source.md")

	out, links, err := r.RenderWithLinks("see [[Missing]] above.", "/abs/source.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("links: got %d want 1", len(links))
	}
	if links[0].Resolved.Kind != LinkInvalid {
		t.Fatalf("expected LinkInvalid for unresolved wikilink, got %v", links[0].Resolved.Kind)
	}
	if !strings.Contains(out, "Missing") {
		t.Fatalf("rendered output missing display text: %q", out)
	}
	if !strings.Contains(out, "?") {
		t.Fatalf("expected '?' suffix on broken wikilink: %q", out)
	}
}

func TestRenderWithLinks_AliasUsedAsDisplayText(t *testing.T) {
	r, err := NewRenderer(80, WithResolver(fakeResolver{
		answers: map[string]string{"Foo": "/abs/foo.md"},
	}))
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	out, links, err := r.RenderWithLinks("see [[Foo|the foo]] above.", "/abs/source.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(links) != 1 || links[0].Text != "the foo" {
		t.Fatalf("link text: got %v want 'the foo'", links)
	}
	if !strings.Contains(out, "the foo") {
		t.Fatalf("rendered output missing alias: %q", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/markdown/ -run TestRenderWithLinks_ -v`
Expected: FAIL — wikilinks not yet handled by the renderer.

- [ ] **Step 3: Implement source pre-processing**

Add to `internal/markdown/links_render.go`:

```go
import (
	"regexp"
)

// wikilinkRegex matches the wikilink syntax for the source-rewrite pass.
// We use a regex rather than goldmark for the rewrite because goldmark's
// AST → source round-trip is lossy; rewriting strings preserves
// everything else about the source unchanged.
//
// Capture groups:
//   1. raw body inside [[...]]
//
// The body is parsed by parseWikilinkBody (same logic as the indexer).
var wikilinkRegex = regexp.MustCompile(`\[\[([^\]\n]+)\]\]`)

// preprocessWikilinks rewrites [[...]] occurrences in src into either
// standard markdown links (resolved) or styled placeholder text
// (unresolved). The resulting string is then handed to Glamour as
// normal markdown.
func (r *Renderer) preprocessWikilinks(src string) string {
	if r.resolver == nil {
		return src
	}
	return wikilinkRegex.ReplaceAllStringFunc(src, func(match string) string {
		body := match[2 : len(match)-2]
		w := parseWikilinkBodyForRender(body)
		if w == nil {
			return match
		}
		display := w.alias
		if display == "" {
			display = w.name
			if w.heading != "" {
				display = w.name + " > " + w.heading
			}
		}
		path, ok := r.resolver.Resolve(r.fromFile, w.name, w.heading, w.block)
		if !ok {
			return display + "?"
		}
		href := path
		if w.heading != "" {
			href = path + "#" + slugify(w.heading)
		}
		return "[" + display + "](" + href + ")"
	})
}

// parsedWikilink mirrors the vault's wikilinkNode without depending on
// it (markdown does not import vault). Names are kept lowercase here
// to make the source-rewrite logic readable.
type parsedWikilink struct {
	name    string
	heading string
	block   string
	alias   string
}

func parseWikilinkBodyForRender(body string) *parsedWikilink {
	body = trimSpace(body)
	if body == "" {
		return nil
	}
	w := &parsedWikilink{}
	if i := indexByte(body, '|'); i >= 0 {
		w.alias = trimSpace(body[i+1:])
		body = body[:i]
	}
	if i := indexByte(body, '^'); i >= 0 {
		w.block = trimSpace(body[i+1:])
		body = body[:i]
	}
	if i := indexByte(body, '#'); i >= 0 {
		w.heading = trimSpace(body[i+1:])
		body = body[:i]
	}
	w.name = trimSpace(body)
	if w.name == "" {
		return nil
	}
	return w
}

func trimSpace(s string) string  { return strings.TrimSpace(s) }
func indexByte(s string, b byte) int { return strings.IndexByte(s, b) }

// slugify is the same heading-slug rule used by anchor-style links.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	return b.String()
}
```

Update `RenderWithLinks` to apply the pre-processor:

```go
func (r *Renderer) RenderWithLinks(src, base string, marker LinkMarker) (string, []Link, error) {
	src = r.preprocessWikilinks(src)
	raw, err := r.instrumented.Render(src)
	// ... rest unchanged
```

Add `import "strings"` if not already present in the file.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/markdown/ -run TestRenderWithLinks_ -v`
Expected: PASS for all three tests.

- [ ] **Step 5: Run all tests to verify no regressions**

Run: `go test ./...`
Expected: PASS for everything.

- [ ] **Step 6: Commit**

```bash
git add internal/markdown/links_render.go internal/markdown/wikilink_test.go
git commit -m "feat(markdown): rewrite wikilinks to standard links before Glamour render"
```

### Task 15: Wire the resolver into the TUI

The TUI now passes its vault to the renderer.

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/content.go`

- [ ] **Step 1: Update `New` to construct the renderer with the resolver**

In `internal/tui/model.go`, find the line:

```go
r, err := markdown.NewRenderer(80)
```

Replace with:

```go
r, err := markdown.NewRenderer(80)
if err != nil {
	return Model{}, err
}
// Renderer is created without a resolver here; we attach the vault
// after Build below. WithResolver as an option works at construction;
// for runtime updates we rebuild the renderer.
_ = r
```

Actually, the cleanest fix is to construct the vault first, then the renderer with the vault. Replace the section:

```go
r, err := markdown.NewRenderer(80)
if err != nil {
	return Model{}, err
}

m := Model{
	root:       root,
	rootNode:   rootNode,
	viewport:   viewport.New(0, 0),
	renderer:   r,
	history:    nav.New(),
	focus:      focusTree,
	keys:       defaultKeys(),
	linkCursor: -1,
}
```

with:

```go
diag := newDiagnostics(diagOpts{LogPath: defaultLogPath()})
var v *vault.Vault
if vv, err := vault.Build(root, diag); err == nil {
	v = vv
} else {
	diag.Warn("vault build failed: " + err.Error())
}

var rOpts []markdown.Option
if v != nil {
	rOpts = append(rOpts, markdown.WithResolver(v))
}
r, err := markdown.NewRenderer(80, rOpts...)
if err != nil {
	return Model{}, err
}

m := Model{
	root:       root,
	rootNode:   rootNode,
	viewport:   viewport.New(0, 0),
	renderer:   r,
	history:    nav.New(),
	focus:      focusTree,
	keys:       defaultKeys(),
	linkCursor: -1,
	vault:      v,
	diag:       diag,
}
```

Remove the duplicated diag/vault wiring from later in `New` (left over from Task 5).

- [ ] **Step 2: Update WindowSizeMsg renderer rebuild**

In `Update`, the `WindowSizeMsg` case calls `markdown.NewRenderer(renderWidth)`. Update to pass the resolver:

```go
var rOpts []markdown.Option
if m.vault != nil {
	rOpts = append(rOpts, markdown.WithResolver(m.vault))
}
if r, err := markdown.NewRenderer(renderWidth, rOpts...); err == nil {
	m.renderer = r
}
```

- [ ] **Step 3: Set the file path before each render**

In `internal/tui/content.go`, in `refreshContent`, before calling `RenderWithLinks`:

```go
m.renderer.SetFromFile(path)
out, links, err := m.renderer.RenderWithLinks(string(src), path, linkZoneMarker)
```

- [ ] **Step 4: Run tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Build and run as a smoke test**

Run: `go build ./cmd/hypogeum`
Expected: Empty output. Don't run the TUI here (no terminal in test harness), but the binary must build clean.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/model.go internal/tui/content.go
git commit -m "feat(tui): wire vault into renderer so wikilinks resolve"
```

---

## Stage 6 — Watcher integration

`RefreshFile` and `Rebuild` driven by `watch.Event`s. The fsEventMsg handler calls `vault.RefreshFile` (FileModified) or `vault.Rebuild` (StructureChanged) before refreshing the content pane.

### Task 16: `RefreshFile` and `Rebuild` implementations

**Files:**
- Modify: `internal/vault/vault.go`
- Modify: `internal/vault/vault_test.go`

- [ ] **Step 1: Write the failing test**

Append to `vault_test.go`:

```go
func TestRefreshFileUpdatesIndex(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.md", "links to [[b]].")
	writeFile(t, dir, "b.md", "i am b")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	bAbs, _ := filepath.Abs(filepath.Join(dir, "b.md"))
	if got := len(v.Backlinks(bAbs)); got != 1 {
		t.Fatalf("initial Backlinks: got %d want 1", got)
	}

	if err := os.WriteFile(a, []byte("no more links."), 0o644); err != nil {
		t.Fatalf("rewrite a: %v", err)
	}
	if err := v.RefreshFile(a); err != nil {
		t.Fatalf("RefreshFile: %v", err)
	}
	if got := len(v.Backlinks(bAbs)); got != 0 {
		t.Fatalf("post-refresh Backlinks: got %d want 0", got)
	}
}

func TestRefreshFileDeletedFileDropsEntry(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.md", "links to [[b]].")
	writeFile(t, dir, "b.md", "i am b")

	v, _ := Build(dir, NopDiagnostics{})
	bAbs, _ := filepath.Abs(filepath.Join(dir, "b.md"))
	os.Remove(a)
	if err := v.RefreshFile(a); err != nil {
		t.Fatalf("RefreshFile on deleted: %v", err)
	}
	if got := len(v.Backlinks(bAbs)); got != 0 {
		t.Fatalf("after delete-and-refresh: got %d want 0", got)
	}
}

func TestRebuildPicksUpNewFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "links to [[b]].")
	v, _ := Build(dir, NopDiagnostics{})

	from, _ := filepath.Abs(filepath.Join(dir, "a.md"))
	if _, ok := v.Resolve(from, "b", "", ""); ok {
		t.Fatalf("b should not resolve before it exists")
	}

	writeFile(t, dir, "b.md", "i am b")
	if err := v.Rebuild(); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if _, ok := v.Resolve(from, "b", "", ""); !ok {
		t.Fatalf("b should resolve after Rebuild")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/vault/ -run "TestRefreshFile|TestRebuild" -v`
Expected: FAIL — both methods are stubs.

- [ ] **Step 3: Implement RefreshFile and Rebuild**

In `internal/vault/vault.go`, replace the stub methods:

```go
func (v *Vault) RefreshFile(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	// If the file is gone, drop its entry. The watcher will follow
	// up with a StructureChanged for the directory, which calls
	// Rebuild — so getting Resolve right here is best-effort.
	if _, statErr := os.Stat(abs); statErr != nil {
		v.mu.Lock()
		delete(v.files, abs)
		key := nameKey(abs)
		v.names[key] = removePath(v.names[key], abs)
		if len(v.names[key]) == 0 {
			delete(v.names, key)
		}
		v.mu.Unlock()
		v.diag.Info("vault: file vanished before refresh: " + abs)
		return nil
	}

	v.indexFile(abs)
	// Re-resolve all wikilinks in case this file's appearance/change
	// affects resolution (e.g. it newly satisfies a name lookup).
	v.resolveAllRefs()
	return nil
}

func (v *Vault) Rebuild() error {
	v.mu.Lock()
	v.files = make(map[string]*fileEntry)
	v.names = make(map[string][]string)
	v.mu.Unlock()
	if err := v.walkAndIndex(); err != nil {
		return err
	}
	v.resolveAllRefs()
	return nil
}

func removePath(s []string, p string) []string {
	out := s[:0]
	for _, x := range s {
		if x != p {
			out = append(out, x)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/vault/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/vault/vault.go internal/vault/vault_test.go
git commit -m "feat(vault): implement RefreshFile and Rebuild for watcher integration"
```

### Task 17: TUI calls vault on filesystem events

**Files:**
- Modify: `internal/tui/content.go`

- [ ] **Step 1: Update `handleFSEvent` to call vault**

In `handleFSEvent`, add vault refresh calls. The current method:

```go
func (m *Model) handleFSEvent(ev watch.Event) {
	switch ev.Kind {
	case watch.StructureChanged:
		// ... existing logic
	case watch.FileModified:
		// ... existing logic
	}
}
```

becomes:

```go
func (m *Model) handleFSEvent(ev watch.Event) {
	switch ev.Kind {
	case watch.StructureChanged:
		if m.vault != nil {
			if err := m.vault.Rebuild(); err != nil {
				m.diag.Warn("vault rebuild failed: " + err.Error())
			}
		}
		// ... existing tree-rewalk logic continues here

	case watch.FileModified:
		if m.vault != nil {
			for _, p := range ev.Paths {
				if err := m.vault.RefreshFile(p); err != nil {
					m.diag.Warn("vault refresh failed: " + err.Error())
				}
			}
		}
		// ... existing content-refresh logic continues here
	}
}
```

The simplest change is to add the two vault calls at the *top* of each case branch, leaving the existing code untouched below.

- [ ] **Step 2: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/content.go
git commit -m "feat(tui): refresh vault on watcher events"
```

---

## Stage 7 — Snippet extraction

The smallest enclosing block of plain text around each reference, with the link's display text wrapped in SGR for highlight.

### Task 18: Snippet extraction

**Files:**
- Create: `internal/vault/snippet.go`
- Create: `internal/vault/snippet_test.go`
- Modify: `internal/vault/vault.go` (call snippet extraction in `extractReferences`)

- [ ] **Step 1: Write the failing tests**

Create `internal/vault/snippet_test.go`:

```go
package vault

import (
	"strings"
	"testing"
)

func TestSnippet_ParagraphContext(t *testing.T) {
	src := "before. Here we link to [[Foo]] in a sentence. after."
	refs := extractReferences(src, "/x.md")
	if len(refs) != 1 {
		t.Fatalf("refs: got %d want 1", len(refs))
	}
	if !strings.Contains(refs[0].snippet, "Here we link to") {
		t.Fatalf("snippet missing surrounding text: %q", refs[0].snippet)
	}
	if !strings.Contains(refs[0].snippet, "Foo") {
		t.Fatalf("snippet missing display text: %q", refs[0].snippet)
	}
}

func TestSnippet_ListItemContext(t *testing.T) {
	src := `- first item
- item with [[Foo]] inside
- last item`
	refs := extractReferences(src, "/x.md")
	if len(refs) != 1 {
		t.Fatalf("refs: got %d want 1", len(refs))
	}
	// The snippet should be the list item's text only — not the whole list.
	if strings.Contains(refs[0].snippet, "first item") {
		t.Fatalf("snippet leaked sibling list items: %q", refs[0].snippet)
	}
	if !strings.Contains(refs[0].snippet, "item with") {
		t.Fatalf("snippet missing item text: %q", refs[0].snippet)
	}
}

func TestSnippet_HighlightWrapping(t *testing.T) {
	src := "see [[Foo]] now."
	refs := extractReferences(src, "/x.md")
	if len(refs) != 1 {
		t.Fatalf("refs: got %d want 1", len(refs))
	}
	// The snippet wraps the display text in the snippet highlight markers.
	if !strings.Contains(refs[0].snippet, snippetHighlightOpen+"Foo"+snippetHighlightClose) {
		t.Fatalf("snippet not wrapped with highlight markers: %q", refs[0].snippet)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/vault/ -run TestSnippet -v`
Expected: FAIL — snippet is empty, highlight markers undefined.

- [ ] **Step 3: Implement snippet extraction**

Create `internal/vault/snippet.go`:

```go
package vault

import (
	"strings"

	"github.com/yuin/goldmark/ast"
)

// snippetHighlightOpen / Close bracket the display text of a reference
// inside its snippet. The TUI applies SGR around these markers when
// rendering snippets so the wikilink target stands out.
//
// Using ASCII unit-separator characters keeps the markers invisible
// to plain-text processing while distinguishable from anything in
// user content.
const (
	snippetHighlightOpen  = "\x11" // DC1
	snippetHighlightClose = "\x12" // DC2
)

// snippetForNode walks up from n to the smallest enclosing block-level
// node, then renders that subtree as plain text. Within the result,
// the original n's text is wrapped in highlight markers.
func snippetForNode(n ast.Node, source []byte, displayText string) string {
	block := enclosingBlock(n)
	if block == nil {
		return wrapHighlight(displayText)
	}
	plain := nodeText(block, source)
	plain = strings.TrimSpace(plain)

	// Replace the first occurrence of displayText with the wrapped form.
	// Using First only — multiple occurrences in one block would need a
	// position-aware splice, which Phase 1 doesn't require (rare in
	// practice).
	if displayText != "" {
		i := strings.Index(plain, displayText)
		if i >= 0 {
			plain = plain[:i] + wrapHighlight(displayText) + plain[i+len(displayText):]
		}
	}
	return plain
}

func wrapHighlight(s string) string {
	return snippetHighlightOpen + s + snippetHighlightClose
}

// enclosingBlock returns the smallest block-level ancestor of n.
// Block-level means: paragraph, heading, list item, blockquote, fenced
// code (etc.). Returns nil if n is itself the document root.
func enclosingBlock(n ast.Node) ast.Node {
	for cur := n.Parent(); cur != nil; cur = cur.Parent() {
		if cur.Type() == ast.TypeBlock {
			return cur
		}
	}
	return nil
}

// nodeText recursively concatenates every Text segment under n.
// Block-level structure (list bullets, blockquote markers) is not
// included — snippets are plain text by design.
func nodeText(n ast.Node, source []byte) string {
	var out []byte
	switch v := n.(type) {
	case *ast.Text:
		return string(v.Segment.Value(source))
	case *wikilinkNode:
		// Render the display text — the alias if set, else the name.
		disp := v.Alias
		if disp == "" {
			disp = v.Name
		}
		return disp
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		piece := nodeText(c, source)
		if len(out) > 0 && len(piece) > 0 {
			// Insert a space between sibling block-level pieces so
			// "Heading\nparagraph" doesn't render as "Headingparagraph".
			if c.Type() == ast.TypeBlock {
				out = append(out, ' ')
			}
		}
		out = append(out, piece...)
	}
	return string(out)
}
```

- [ ] **Step 4: Update `extractReferences` to populate snippet and line**

In `internal/vault/vault.go`, modify `extractReferences` to compute snippet for each reference:

```go
func extractReferences(src, fromPath string) []reference {
	source := []byte(src)
	md := goldmark.New(goldmark.WithExtensions(WikilinkExtension))
	doc := md.Parser().Parse(text.NewReader(source))

	var refs []reference
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch nn := n.(type) {
		case *wikilinkNode:
			disp := nn.Alias
			if disp == "" {
				disp = nn.Name
			}
			refs = append(refs, reference{
				kind:        refWikilink,
				target:      nn.Name,
				heading:     nn.Heading,
				block:       nn.Block,
				alias:       nn.Alias,
				displayText: disp,
				snippet:     snippetForNode(nn, source, disp),
				line:        lineForNode(nn, source),
			})
			return ast.WalkSkipChildren, nil
		case *ast.Link:
			href := string(nn.Destination)
			disp := linkText(nn, source)
			refs = append(refs, reference{
				kind:        refStdLink,
				target:      href,
				resolved:    resolveStdLink(fromPath, href),
				displayText: disp,
				snippet:     snippetForNode(nn, source, disp),
				line:        lineForNode(nn, source),
			})
			return ast.WalkSkipChildren, nil
		case *ast.Image:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return refs
}

// lineForNode returns the 1-indexed line of the first segment of n
// within source. Returns 0 if no segment is found (rare — defensive).
func lineForNode(n ast.Node, source []byte) int {
	var seg *ast.Text
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := c.(*ast.Text); ok {
			seg = t
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if seg == nil {
		return 0
	}
	// Count newlines from start to seg.Segment.Start.
	stop := seg.Segment.Start
	if stop > len(source) {
		stop = len(source)
	}
	line := 1
	for i := 0; i < stop; i++ {
		if source[i] == '\n' {
			line++
		}
	}
	return line
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/vault/ -v`
Expected: PASS — including the new snippet tests.

- [ ] **Step 6: Commit**

```bash
git add internal/vault/snippet.go internal/vault/snippet_test.go internal/vault/vault.go
git commit -m "feat(vault): extract enclosing-block snippets with display-text highlight"
```

---

## Stage 8 — Backlinks persistent pane

The `b` key toggles a bottom split that shows backlinks for the current file.

### Task 19: `keys.go` adds `Backlinks` binding; `b` toggles state

**Files:**
- Modify: `internal/tui/keys.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/input.go`
- Modify: `internal/tui/model_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/model_test.go`:

```go
import (
	tea "github.com/charmbracelet/bubbletea"
)

func TestKeyBTogglesBacklinksOpen(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.backlinksOpen {
		t.Fatalf("expected backlinksOpen=false initially")
	}
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if !out.(Model).backlinksOpen {
		t.Fatalf("after b: expected backlinksOpen=true")
	}
	out2, _ := out.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if out2.(Model).backlinksOpen {
		t.Fatalf("after second b: expected backlinksOpen=false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestKeyBTogglesBacklinksOpen -v`
Expected: FAIL — `backlinksOpen` undefined.

- [ ] **Step 3: Add fields and binding**

In `internal/tui/model.go`, add fields to `Model`:

```go
backlinksOpen   bool
backlinksVP     viewport.Model
backlinkCursor  int
```

In `internal/tui/keys.go`, add to `keyMap`:

```go
ToggleBacklinks key.Binding
```

In `defaultKeys`:

```go
ToggleBacklinks: key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "backlinks pane")),
```

In `internal/tui/input.go`, in `handleContentKey` (or `handleKey` if we want it global; per spec it's content-pane scoped — but the simplest is global since the persistent pane shows backlinks for whichever file is open regardless of focus):

Actually, per Section 4 spec — "`b` toggles persistent bottom split" — there's no scoping clause. Make it global by adding the case to `handleKey` before pane dispatch:

```go
case key.Matches(msg, m.keys.ToggleBacklinks):
	m.backlinksOpen = !m.backlinksOpen
	return *m, nil
```

- [ ] **Step 4: Run test**

Run: `go test ./internal/tui/ -run TestKeyBTogglesBacklinksOpen -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/keys.go internal/tui/model.go internal/tui/input.go internal/tui/model_test.go
git commit -m "feat(tui): add b binding to toggle backlinks pane state"
```

### Task 20: Backlinks pane geometry and rendering

**Files:**
- Create: `internal/tui/backlinks.go`
- Create: `internal/tui/backlinks_test.go`
- Modify: `internal/tui/view.go`
- Modify: `internal/tui/model.go` (WindowSizeMsg height calc)
- Modify: `internal/tui/content.go` (refreshContent populates backlinks viewport)

- [ ] **Step 1: Write the failing test**

Create `internal/tui/backlinks_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBacklinksPaneShowsLinkers(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[b]] for more.")
	writeTUITestFile(t, dir, "b.md", "i am b.")

	m, err := New(dir, "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Window-size message so geometry is initialized.
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)
	// Open file b.md so backlinks pane shows links from a.md.
	bAbs := filepath.Join(dir, "b.md")
	m.openFile(bAbs)
	// Toggle backlinks pane.
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = mm.(Model)

	rendered := m.renderBacklinks()
	if !strings.Contains(rendered, "a.md") {
		t.Fatalf("expected a.md in backlinks pane, got %q", rendered)
	}
}

func TestBacklinksPaneAutoCollapsesBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	m.backlinksOpen = true
	m.height = 15 // below threshold
	if m.shouldShowBacklinks() {
		t.Fatalf("expected backlinks suppressed at height %d", m.height)
	}
	m.height = 25
	if !m.shouldShowBacklinks() {
		t.Fatalf("expected backlinks visible at height %d", m.height)
	}
}

func writeTUITestFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}
```

Add to imports: `"os"`, `"path/filepath"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestBacklinksPane -v`
Expected: FAIL — `renderBacklinks`, `shouldShowBacklinks` undefined.

- [ ] **Step 3: Create `internal/tui/backlinks.go`**

```go
package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/wilkes/hypogeum/internal/vault"
)

// backlinksHeight is the row count of the persistent bottom-split pane,
// including its border. Two visible rows per backlink × 4 backlinks
// visible at a time + border (2) = 10 — but per spec the pane is 8 rows,
// scrollable internally. So with border it's 8.
const backlinksHeight = 8

// backlinksMinTotalHeight is the minimum terminal height at which the
// persistent backlinks pane is shown. Below this, the pane state is
// preserved but the pane is suppressed in View().
const backlinksMinTotalHeight = 20

// shouldShowBacklinks returns true when the persistent pane is open
// AND the terminal is tall enough for it.
func (m Model) shouldShowBacklinks() bool {
	return m.backlinksOpen && m.height >= backlinksMinTotalHeight
}

// refreshBacklinks repopulates m.backlinksVP from the vault for the
// currently-open file. Called on file change and on toggle.
func (m *Model) refreshBacklinks(currentPath string) {
	if m.vault == nil || currentPath == "" {
		m.backlinksVP.SetContent("")
		return
	}
	links := m.vault.Backlinks(currentPath)
	m.backlinksVP.SetContent(formatBacklinks(links, m.root, m.viewport.Width))
}

// renderBacklinks returns the rendered string of the persistent pane,
// styled to match the rest of the UI. Empty string when the pane is
// suppressed.
func (m Model) renderBacklinks() string {
	if !m.shouldShowBacklinks() {
		return ""
	}
	return paneStyle(false).
		Width(m.viewport.Width).
		Height(backlinksHeight - 2). // -2 for top/bottom border
		Render(m.backlinksVP.View())
}

// formatBacklinks renders a slice of vault.Backlink as the two-row-per-
// entry text used in both the persistent pane and the modal.
func formatBacklinks(links []vault.Backlink, root string, width int) string {
	if len(links) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("(no backlinks)")
	}
	var b strings.Builder
	for _, l := range links {
		rel, err := filepath.Rel(root, l.SourceFile)
		if err != nil {
			rel = l.SourceFile
		}
		fmt.Fprintf(&b, "%s:%d\n", rel, l.Line)
		fmt.Fprintf(&b, "  %s\n", truncateOneLine(applyHighlight(l.Snippet), width-2))
	}
	return b.String()
}

// applyHighlight replaces snippetHighlightOpen/Close markers with SGR
// codes for visible bold/yellow display.
func applyHighlight(s string) string {
	hi := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	out := s
	for {
		i := strings.Index(out, "\x11")
		j := strings.Index(out, "\x12")
		if i < 0 || j < 0 || j < i {
			break
		}
		out = out[:i] + hi.Render(out[i+1:j]) + out[j+1:]
	}
	return out
}

// truncateOneLine collapses internal newlines into spaces and clips
// to width with an ellipsis if needed.
func truncateOneLine(s string, width int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if width <= 0 || len(s) <= width {
		return s
	}
	if width <= 1 {
		return s[:width]
	}
	return s[:width-1] + "…"
}
```

The string constants `\x11` / `\x12` are imported from `internal/vault` via duplication (not import — keeps the layering clean). Let me make that explicit by exporting them from vault. Update `internal/vault/snippet.go` to make them public:

Wait — exporting `snippetHighlightOpen` requires renaming. Cleaner: redefine the constants here in tui with the same values:

```go
// Mirror of vault's snippet highlight markers. Defined here so the TUI
// doesn't import internal vault constants — markers are part of the
// data contract on the Backlink.Snippet string.
const (
	snippetHighlightOpenChar  = "\x11"
	snippetHighlightCloseChar = "\x12"
)
```

And replace `"\x11"` and `"\x12"` in `applyHighlight` with these constants.

- [ ] **Step 4: Wire into view geometry**

In `internal/tui/view.go`, update `View` to subtract backlinks height from content area when shown, and append the pane:

Replace:

```go
contentStyled := zone.Mark(zoneContentPane, paneStyle(m.focus == focusContent).
	Width(m.viewport.Width).
	Height(m.height-4).
	Render(content))

body := lipgloss.JoinHorizontal(lipgloss.Top, treeStyled, contentStyled)
footer := m.renderFooter()
```

with:

```go
contentHeight := m.height - 4
if m.shouldShowBacklinks() {
	contentHeight -= backlinksHeight
}
contentStyled := zone.Mark(zoneContentPane, paneStyle(m.focus == focusContent).
	Width(m.viewport.Width).
	Height(contentHeight).
	Render(content))

contentColumn := contentStyled
if bl := m.renderBacklinks(); bl != "" {
	contentColumn = lipgloss.JoinVertical(lipgloss.Left, contentStyled, bl)
}

body := lipgloss.JoinHorizontal(lipgloss.Top, treeStyled, contentColumn)
footer := m.renderFooter()
```

In `internal/tui/model.go`'s `WindowSizeMsg` handler, also size the backlinks viewport:

```go
m.backlinksVP.Width = contentWidth
m.backlinksVP.Height = backlinksHeight - 2
```

Add to `New` (after the viewport init):

```go
m.backlinksVP = viewport.New(0, 0)
```

- [ ] **Step 5: Refresh backlinks when content refreshes**

In `internal/tui/content.go`, at the bottom of `refreshContent`:

```go
m.refreshBacklinks(path)
```

Also on toggle, refresh: in `handleKey` where `b` is processed, after toggling:

```go
if m.backlinksOpen {
	m.refreshBacklinks(m.history.Current())
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/tui/ -run TestBacklinksPane -v`
Expected: PASS.

- [ ] **Step 7: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/backlinks.go internal/tui/backlinks_test.go internal/tui/view.go internal/tui/model.go internal/tui/content.go internal/tui/input.go
git commit -m "feat(tui): render persistent backlinks pane below content"
```

---

## Stage 9 — Modal infrastructure + backlinks modal

### Task 21: `modalKind` enum and shared modal viewport

**Files:**
- Create: `internal/tui/modal.go`
- Modify: `internal/tui/model.go`

- [ ] **Step 1: Create `internal/tui/modal.go`**

```go
package tui

import (
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// modalKind enumerates which modal (if any) is currently visible.
// The single-modal invariant means at most one is open at a time:
// pressing the toggle key for one while another is open swaps content.
type modalKind int

const (
	modalNone modalKind = iota
	modalBacklinks
	modalLogs
)

// modalGeometry returns the (x, y, w, h) of the modal frame given the
// current terminal dimensions. The modal is 60% × 60% clamped to a
// minimum of 40×12 and a maximum of 120×40.
func modalGeometry(termW, termH int) (x, y, w, h int) {
	w = termW * 60 / 100
	h = termH * 60 / 100
	if w < 40 {
		w = 40
	}
	if h < 12 {
		h = 12
	}
	if w > 120 {
		w = 120
	}
	if h > 40 {
		h = 40
	}
	if w > termW {
		w = termW
	}
	if h > termH {
		h = termH
	}
	x = (termW - w) / 2
	y = (termH - h) / 2
	return
}

// renderModal styles `body` with a border at the modal geometry.
// The caller is responsible for clipping `body` to modal interior size.
func (m Model) renderModal(body string) string {
	_, _, w, h := modalGeometry(m.width, m.height)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Width(w - 2).
		Height(h - 2).
		Render(body)
}

// resizeModalVP resizes the shared modal viewport to fit the modal interior.
func (m *Model) resizeModalVP() {
	_, _, w, h := modalGeometry(m.width, m.height)
	m.modalVP.Width = w - 2
	m.modalVP.Height = h - 2
}

// newModalViewport returns an empty viewport sized 0,0 — resized later.
func newModalViewport() viewport.Model {
	return viewport.New(0, 0)
}
```

- [ ] **Step 2: Add fields to Model**

In `internal/tui/model.go`:

```go
modalOpen modalKind
modalVP   viewport.Model
```

In `New`:

```go
m.modalVP = newModalViewport()
```

In `WindowSizeMsg`:

```go
m.resizeModalVP()
```

- [ ] **Step 3: Build to confirm**

Run: `go build ./...`
Expected: Empty output, exit code 0.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/modal.go internal/tui/model.go
git commit -m "feat(tui): add modalKind enum and shared modal viewport"
```

### Task 22: Backlinks modal toggle (`B`) and rendering

**Files:**
- Modify: `internal/tui/keys.go`
- Modify: `internal/tui/input.go`
- Modify: `internal/tui/view.go`
- Modify: `internal/tui/backlinks.go`
- Modify: `internal/tui/backlinks_test.go`

- [ ] **Step 1: Write the failing test**

Append to `backlinks_test.go`:

```go
func TestBacklinksModalToggleAndEsc(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[b]].")
	writeTUITestFile(t, dir, "b.md", "i am b.")

	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)
	m.openFile(filepath.Join(dir, "b.md"))

	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	if out.(Model).modalOpen != modalBacklinks {
		t.Fatalf("after B: expected modalBacklinks, got %v", out.(Model).modalOpen)
	}

	out2, _ := out.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if out2.(Model).modalOpen != modalNone {
		t.Fatalf("after Esc: expected modalNone, got %v", out2.(Model).modalOpen)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/tui/ -run TestBacklinksModalToggleAndEsc -v`
Expected: FAIL — `modalOpen`, `modalBacklinks`, capital `B` binding undefined.

- [ ] **Step 3: Add the binding and handler**

In `keys.go`:

```go
OpenBacklinksModal key.Binding
```

In `defaultKeys`:

```go
OpenBacklinksModal: key.NewBinding(key.WithKeys("B"), key.WithHelp("B", "backlinks modal")),
```

In `input.go`, `handleKey`, before pane dispatch — but *after* the back/forward/focus handlers — add modal-priority handling:

```go
// Modal opens take precedence; Esc closes modal first.
if m.modalOpen != modalNone {
	switch {
	case key.Matches(msg, m.keys.ClearLink): // Esc
		m.modalOpen = modalNone
		return *m, nil
	}
	// Forward j/k/etc. to the modal viewport.
	var cmd tea.Cmd
	m.modalVP, cmd = m.modalVP.Update(msg)
	return *m, cmd
}

if key.Matches(msg, m.keys.OpenBacklinksModal) {
	m.modalOpen = modalBacklinks
	m.refreshBacklinksModal(m.history.Current())
	return *m, nil
}
```

Add `refreshBacklinksModal` to `backlinks.go`:

```go
func (m *Model) refreshBacklinksModal(currentPath string) {
	if m.vault == nil || currentPath == "" {
		m.modalVP.SetContent("")
		return
	}
	m.resizeModalVP()
	links := m.vault.Backlinks(currentPath)
	m.modalVP.SetContent(formatBacklinks(links, m.root, m.modalVP.Width))
}
```

- [ ] **Step 4: Render the modal in View**

In `view.go`, update `View` so that when a modal is open, it overlays:

```go
base := zone.Scan(lipgloss.JoinVertical(lipgloss.Left, body, footer))
if m.modalOpen != modalNone {
	return overlayModal(base, m.renderModal(m.modalVP.View()), m.width, m.height)
}
return base
```

Add `overlayModal` to `modal.go`:

```go
import "strings"

// overlayModal places `modal` in the center of `base`. Both are full
// width/height strings; this implementation renders `modal` rendered
// over the corresponding rows of `base`.
//
// Lip Gloss has Place/PlaceHorizontal but they don't preserve the
// underlying content; we want the modal *over* the body, not
// replacing it. Simplest implementation: split base into lines, splice
// modal lines in at the right offset.
func overlayModal(base, modal string, termW, termH int) string {
	x, y, _, _ := modalGeometry(termW, termH)

	baseLines := strings.Split(base, "\n")
	modalLines := strings.Split(modal, "\n")

	for i, ml := range modalLines {
		row := y + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		baseLines[row] = spliceLine(baseLines[row], ml, x)
	}
	return strings.Join(baseLines, "\n")
}

// spliceLine overlays `over` onto `base` starting at column x.
// Naive ASCII-aware version (does not handle ANSI escapes inside
// `base` — modal is opaque so this is acceptable for Phase 1).
func spliceLine(base, over string, x int) string {
	if x >= len(base) {
		// pad and append
		return base + strings.Repeat(" ", x-len(base)) + over
	}
	end := x + len(over)
	if end > len(base) {
		return base[:x] + over
	}
	return base[:x] + over + base[end:]
}
```

> Note: spliceLine is naive about ANSI escapes — Phase 1 modal is opaque enough that this works in practice, but it's the kind of thing that would be revisited if we added partially-transparent modals later.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tui/ -run TestBacklinks -v`
Expected: PASS for both pane and modal tests.

- [ ] **Step 6: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/keys.go internal/tui/input.go internal/tui/view.go internal/tui/backlinks.go internal/tui/modal.go internal/tui/backlinks_test.go
git commit -m "feat(tui): add B modal for backlinks with overlay rendering"
```

---

## Stage 10 — Log viewer modal + transient footer

### Task 23: `?` opens the log viewer modal; `B` and `?` swap content

**Files:**
- Create: `internal/tui/logs.go`
- Create: `internal/tui/logs_test.go`
- Modify: `internal/tui/keys.go`
- Modify: `internal/tui/input.go`

- [ ] **Step 1: Write the failing test**

Create `internal/tui/logs_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLogsModalShowsRingBuffer(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	m.diag.Warn("a problem occurred")

	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	mm2 := out.(Model)
	if mm2.modalOpen != modalLogs {
		t.Fatalf("after ?: expected modalLogs, got %v", mm2.modalOpen)
	}
	rendered := mm2.modalVP.View()
	if !strings.Contains(rendered, "a problem occurred") {
		t.Fatalf("expected log entry in modal: %q", rendered)
	}
}

func TestLogsModalReplacesBacklinksModal(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	// Open backlinks modal.
	out1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'B'}})
	if out1.(Model).modalOpen != modalBacklinks {
		t.Fatalf("expected modalBacklinks")
	}
	// Press ?: backlinks should be replaced by logs.
	out2, _ := out1.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if out2.(Model).modalOpen != modalLogs {
		t.Fatalf("expected modalLogs after ?, got %v", out2.(Model).modalOpen)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/tui/ -run TestLogsModal -v`
Expected: FAIL — `?` does nothing, `modalLogs` not handled.

- [ ] **Step 3: Implement `internal/tui/logs.go`**

```go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// refreshLogsModal repopulates m.modalVP with the diagnostic ring
// buffer formatted for display.
func (m *Model) refreshLogsModal() {
	m.resizeModalVP()
	entries := m.diag.snapshot()
	m.modalVP.SetContent(formatLogEntries(entries))
}

// formatLogEntries renders the ring buffer as one line per entry,
// with severity-colored prefix and timestamp.
func formatLogEntries(entries []diagEntry) string {
	if len(entries) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("(no entries)")
	}
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s %s %s\n",
			e.Timestamp.Format("15:04:05"),
			styleSeverity(e.Severity),
			e.Message,
		)
	}
	return b.String()
}

func styleSeverity(s severity) string {
	switch s {
	case sevError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("ERR ")
	case sevWarn:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("WARN")
	default:
		return lipgloss.NewStyle().Faint(true).Render("INFO")
	}
}
```

- [ ] **Step 4: Add binding and handler**

In `keys.go`:

```go
OpenLogsModal key.Binding
```

In `defaultKeys`:

```go
OpenLogsModal: key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "logs")),
```

In `input.go`, `handleKey`, alongside the `OpenBacklinksModal` handler:

```go
if key.Matches(msg, m.keys.OpenLogsModal) {
	if m.modalOpen == modalLogs {
		m.modalOpen = modalNone
	} else {
		m.modalOpen = modalLogs
		m.refreshLogsModal()
	}
	return *m, nil
}
```

Make sure `OpenBacklinksModal` is similarly toggle-aware:

```go
if key.Matches(msg, m.keys.OpenBacklinksModal) {
	if m.modalOpen == modalBacklinks {
		m.modalOpen = modalNone
	} else {
		m.modalOpen = modalBacklinks
		m.refreshBacklinksModal(m.history.Current())
	}
	return *m, nil
}
```

These two handlers must run *before* the existing "if modalOpen != modalNone" forwarding block — otherwise `?` while modalBacklinks is open gets eaten by the viewport. Reorder to:

```go
// Toggle modals (priority over modal forwarding).
if key.Matches(msg, m.keys.OpenBacklinksModal) {
	// (toggle logic)
}
if key.Matches(msg, m.keys.OpenLogsModal) {
	// (toggle logic)
}

// Modal-open forwarding for everything else.
if m.modalOpen != modalNone {
	// ...
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tui/ -run TestLogs -v`
Expected: PASS.

- [ ] **Step 6: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/logs.go internal/tui/logs_test.go internal/tui/keys.go internal/tui/input.go
git commit -m "feat(tui): add ? log viewer modal with single-modal-swap invariant"
```

### Task 24: Footer transient status

The most recent diagnostic shows in the footer for ~3 seconds.

**Files:**
- Modify: `internal/tui/view.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/diagnostics_test.go`

- [ ] **Step 1: Write the failing test**

Append to `diagnostics_test.go`:

```go
func TestFooterShowsTransientDiagnostic(t *testing.T) {
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)
	m.diag.Warn("transient warning here")

	rendered := m.renderFooter()
	if !strings.Contains(rendered, "transient warning here") {
		t.Fatalf("expected transient warning in footer, got: %q", rendered)
	}
}
```

Add imports if missing.

- [ ] **Step 2: Run test**

Run: `go test ./internal/tui/ -run TestFooterShowsTransientDiagnostic -v`
Expected: FAIL — footer doesn't include transient.

- [ ] **Step 3: Update renderFooter**

In `view.go`, modify `renderFooter` to prefer the transient status when present:

```go
func (m Model) renderFooter() string {
	keys := []string{
		"tab: switch", "↑↓/jk: move", "enter: open",
		"n/p: link", "esc: clear",
		"b: backlinks", "B: modal", "? logs",
		"h/←: back", "l/→: forward", "q: quit",
	}
	help := strings.Join(keys, "  ")

	loc := m.status
	if loc != "" {
		if rel, err := filepath.Rel(m.root, loc); err == nil {
			loc = rel
		}
	}

	transientStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	if m.diag != nil {
		if e, ok := m.diag.transientStatus(); ok {
			loc = transientStyle.Render(string(e.Severity.String()) + ": " + e.Message)
		}
	}

	hasLink := false
	if sel := m.selectedLink(); sel != nil {
		loc = fmt.Sprintf("%s%s [%d/%d] %s", linkFooterMarker, loc, m.linkCursor+1, len(m.links), linkLabel(*sel, m.root))
		hasLink = true
	}
	helpStyle := lipgloss.NewStyle().Faint(true)
	locStyle := helpStyle
	if hasLink {
		locStyle = lipgloss.NewStyle()
	}
	return fmt.Sprintf("%s\n%s", locStyle.Render(loc), helpStyle.Render(help))
}
```

- [ ] **Step 4: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/view.go internal/tui/diagnostics_test.go
git commit -m "feat(tui): show transient diagnostic in footer location row"
```

### Task 25: Auto-clear transient after ~3 seconds

This is mostly aesthetic — without auto-clear, every diagnostic stays in the footer until another fires. Use a `tea.Tick`.

**Files:**
- Modify: `internal/tui/diagnostics.go`
- Modify: `internal/tui/model.go`

- [ ] **Step 1: Add a tick command**

In `internal/tui/model.go`, add a new message type and command:

```go
import "time"

// transientClearMsg is delivered ~3s after a diagnostic is emitted,
// asking the model to clear the footer transient if it's still the
// same entry.
type transientClearMsg struct{ at time.Time }

func clearTransientAfter(d time.Duration, at time.Time) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return transientClearMsg{at: at} })
}
```

In `Update`, handle the message:

```go
case transientClearMsg:
	if m.diag != nil {
		// Only clear if the most recent entry is the one that scheduled
		// this tick. Otherwise a newer diagnostic should keep showing.
		if e, ok := m.diag.transientStatus(); ok && e.Timestamp.Equal(msg.at) {
			m.diag.clearTransient()
		}
	}
	return m, nil
```

- [ ] **Step 2: Schedule the tick on every diagnostic**

The simplest approach: have the diagnostics sink also produce a `tea.Cmd` we can return. But the sink is currently fire-and-forget. Instead, schedule the tick on every render of the footer — if there's a transient, schedule a clear:

In `Update`'s `WindowSizeMsg` case (and any other point where we might transition), or simpler: schedule on diagnostic emit. The cleanest fix is to have `diagnostics.push` return an at-time, and have call sites schedule the tick. But call sites are in vault, which has no Bubble Tea awareness.

Alternative: schedule the tick once at startup and have the tick handler inspect, clear if stale, and re-schedule:

Replace the message handler with a self-rescheduling one:

```go
case transientClearMsg:
	if m.diag != nil {
		if e, ok := m.diag.transientStatus(); ok && time.Since(e.Timestamp) > 3*time.Second {
			m.diag.clearTransient()
		}
	}
	return m, clearTransientAfter(time.Second, time.Now())
```

And kick it off in `Init`:

```go
func (m Model) Init() tea.Cmd {
	cmd := m.waitForFSEvent()
	tick := clearTransientAfter(time.Second, time.Now())
	if cmd == nil {
		return tick
	}
	return tea.Batch(cmd, tick)
}
```

Adjust `clearTransientAfter` to ignore the `at` argument:

```go
func clearTransientAfter(d time.Duration, _ time.Time) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return transientClearMsg{} })
}
```

And `transientClearMsg` becomes empty struct.

- [ ] **Step 3: Verify build and tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/diagnostics.go internal/tui/model.go
git commit -m "feat(tui): clear transient diagnostic from footer after 3s"
```

---

## Stage 11 — Polish

### Task 26: Broken-link footer message when an unresolved wikilink is selected

The link list already classifies the broken wikilink as `LinkInvalid`. The footer should show "broken: [[Name]]" rather than the path.

**Files:**
- Modify: `internal/tui/view.go` (or wherever `linkLabel` lives)

- [ ] **Step 1: Find linkLabel**

Run: `rg "linkLabel" internal/tui`
Expected: returns the file/line where `linkLabel` is defined.

- [ ] **Step 2: Update the function**

Wherever `linkLabel` is defined, branch on `link.Resolved.Kind`:

```go
func linkLabel(l markdown.Link, root string) string {
	if l.Resolved.Kind == markdown.LinkInvalid {
		return "broken: " + l.Href
	}
	// ... existing logic
}
```

If the existing logic is already a switch on `Kind`, just add the `LinkInvalid` arm.

- [ ] **Step 3: Add a test**

Append to `links_test.go` (or wherever similar tests live):

```go
func TestSelectedBrokenWikilinkShowsInFooter(t *testing.T) {
	dir := t.TempDir()
	writeTUITestFile(t, dir, "a.md", "see [[Missing]] here.")
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)
	m.openFile(filepath.Join(dir, "a.md"))
	// Cycle to first link.
	m.cycleLink(+1)
	rendered := m.renderFooter()
	if !strings.Contains(rendered, "broken") {
		t.Fatalf("expected 'broken' in footer, got %q", rendered)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/view.go internal/tui/links_test.go
git commit -m "feat(tui): show 'broken: [[name]]' in footer for unresolved wikilinks"
```

### Task 27: Update README and CLAUDE.md

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update README key bindings**

Add rows to the `## Keys` table in `README.md`:

```markdown
| `b` | Toggle backlinks pane |
| `B` | Backlinks modal |
| `?` | Log viewer |
```

Also update Status to reflect that wikilinks ship.

- [ ] **Step 2: Update CLAUDE.md gotchas**

Append to `## Gotchas` (the file's section):

```markdown
- **Vault is best-effort.** If `vault.Build` fails, `tui.New` continues with a nil vault — wikilinks render as broken, backlinks pane stays empty. Same graceful-degradation rule as the watcher.
- **`?`, `B`, and `b` are mutually aware.** `b` toggles the persistent pane. `B` and `?` open modals; opening one while another is open swaps content (single-modal invariant). `Esc` closes whichever modal is up before falling through to the link cursor's clear behavior.
- **Snippet highlight uses ASCII control chars (`\x11` / `\x12`).** Don't use these bytes in user content (extremely unlikely) and don't rewrite snippets through any pipeline that would strip control chars.
```

Update the "What's not built yet" section to remove the wikilinks/backlinks line if it was there, and replace with a Phase 2 hint.

- [ ] **Step 3: Update docs/index.md**

In `docs/index.md`, move the wikilinks/backlinks entry from "Active feature work" to a new "Shipped" section, or strike it through.

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md docs/index.md
git commit -m "docs: document wikilinks/backlinks/log viewer key bindings and gotchas"
```

### Task 28: End-to-end smoke build

- [ ] **Step 1: Full build**

Run: `go build ./...`
Expected: Empty output, exit code 0.

- [ ] **Step 2: Full vet**

Run: `go vet ./...`
Expected: No warnings.

- [ ] **Step 3: Full test**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Manual TUI smoke test**

The TUI requires a real terminal. Run interactively against `docs/`:

```sh
go run ./cmd/hypogeum docs/
```

Verify:
- Tree renders, content renders.
- Press `b` — backlinks pane appears below content (or doesn't, if the file has no backlinks).
- Press `B` — backlinks modal appears centered.
- Press `?` — log viewer modal appears centered (replacing backlinks if open).
- Press `Esc` — modal closes.
- Open a file containing a `[[wikilink]]` — verify it's followable with `n`/`Enter` if resolved, or shows broken-style if not.

This step is for the human reviewer; agents don't run it.

- [ ] **Step 5: Commit any final fixes from smoke testing**

```bash
git add -p
git commit -m "fix: address issues found during manual smoke test"
```

---

## Self-Review Checklist

Run through this against the spec:

- [x] Goldmark wikilink extension parsing all four forms — Tasks 6, 7
- [x] Standard markdown links in the same index — Task 8
- [x] `Resolver` interface in `markdown`, `vault.Vault` implements — Tasks 12, 14
- [x] Renderer integration: resolved → standard link bytes; unresolved → broken — Task 14
- [x] Vault build at startup — Task 5
- [x] Watcher-driven `RefreshFile` / `Rebuild` — Tasks 16, 17
- [x] Backlinks persistent pane (`b`) — Tasks 19, 20
- [x] Backlinks modal (`B`) — Task 22
- [x] Snippet extraction with highlight — Task 18
- [x] `Diagnostics` interface, ring buffer, JSON-line file log — Tasks 1–3
- [x] Footer transient (~3s clear) — Tasks 24, 25
- [x] Log viewer modal (`?`) — Task 23
- [x] Single-modal invariant — Task 23
- [x] Modal geometry clamps (40×12 min, 120×40 max) — Task 21
- [x] Auto-collapse pane below height 20 — Task 20
- [x] Proximity tiebreaker — Task 10
- [x] Case-insensitive basename lookup — Task 10
- [x] Per-file parse failure emits Warn — Task 11
- [x] Vault unwritable log path falls back gracefully — Task 2
- [x] Broken-link footer message — Task 26
- [x] Documentation updates — Task 27

Type/method consistency check:
- `Vault` methods: `Build`, `RefreshFile`, `Rebuild`, `Resolve`, `Backlinks` — used consistently throughout.
- `Backlink` fields: `SourceFile`, `DisplayText`, `Snippet`, `Line`, `Kind` — consistent.
- `Diagnostics` methods: `Info`, `Warn`, `Error` — consistent.
- Modal kinds: `modalNone`, `modalBacklinks`, `modalLogs` — consistent.
- Severity: `sevInfo`, `sevWarn`, `sevError` — consistent.

No placeholders. Every code step has actual code; every test step shows the test.

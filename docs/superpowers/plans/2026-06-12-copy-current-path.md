# Copy current file path — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a keybinding (`y` in pager, `Ctrl+Y` in modern) that copies the absolute path of the currently-viewed file/directory to the clipboard with a footer toast.

**Architecture:** Compose three existing seams — `m.history.Current()` (absolute path source), `m.copyToClipboard` (injectable dual OS-clipboard + OSC 52 writer), and `m.diag.Info` (transient footer toast). Add one `keyMap` field, bind it in both dialect factories, and dispatch it in `handleContentKey` so it's naturally disabled while a modal is open.

**Tech Stack:** Go, Bubble Tea, `github.com/charmbracelet/bubbles/key`. Tests are model-level (no TTY) in `internal/tui/*_test.go`.

---

## Background for the implementer (read once)

- **Keybindings use a dialect factory.** `internal/tui/keys.go` defines a `keyMap` struct plus two factories, `pagerKeys()` and `modernKeys()`. A new action means: (1) a new `keyMap` field, (2) a binding in *both* factories, (3) dispatch wiring. The dispatch code never branches on dialect — it just calls `key.Matches(msg, m.keys.X)`.
- **Two dispatch switches exist.** In `internal/tui/input.go`: a global *modal-toggle* switch (`handleKey`, runs even while a modal is open) and `handleContentKey` (reached only when no modal is consuming input). We add `CopyPath` to `handleContentKey` so copy-path is disabled while a modal is open — consistent with drag-to-select copy.
- **Current path:** `m.history.Current()` returns the absolute path of the open file/dir, or `""` if nothing is open.
- **Clipboard:** `m.copyToClipboard` is a `clipboardWriter` (`func(string)`), defaulting to `defaultClipboardWriter` and injectable in tests: `m.copyToClipboard = func(s string){ copied = s }`.
- **Toast:** `m.diag.Info(msg)` pushes a transient footer message; `m.renderFooter()` renders the footer (used by tests to assert toast text). The transient is auto-cleared by the perpetual `clearTransientAfter` loop started in `Init` — no new tick command is needed.
- **Test helpers** (`internal/tui/helpers_test.go`): `sized(t, root, file)` builds a sized model in the default (pager) dialect; `sizedWithOptions(t, root, file, Options{Dialect:"modern"})` for modern; `pressRune(t, m, 'y')` sends a rune; `pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})` sends a chord; `writeFixture(t)` lays down `index.md`, `notes/first.md`, `notes/sub/deep.md`.
- **Reflection-driven dialect tests** in `internal/tui/keys_test.go` (`TestPagerKeys_AllActionsBound`, `TestModernKeys_AllActionsBound`, `TestKeys_HelpTextNonEmpty`, `TestKeys_NoOverlappingActions`) iterate every `keyMap` field. They cover `CopyPath` automatically once it is bound with non-empty keys + help in both factories. Do **not** add `CopyPath` to `modernZeroFields`.

---

## File structure

- **Modify** `internal/tui/keys.go` — add `CopyPath key.Binding` field + bind in both factories.
- **Modify** `internal/tui/input.go` — add a `CopyPath` case to `handleContentKey`'s switch + a small helper `copyCurrentPath()`.
- **Create** `internal/tui/copypath_test.go` — behavior tests for copy / no-op / modern chord.
- **Modify** `CLAUDE.md` — one-line mention of the new keybinding (docs only).

---

### Task 1: Add the `CopyPath` keybinding field and dialect bindings

**Files:**
- Modify: `internal/tui/keys.go`

This task is a prerequisite for the dispatch wiring and is verified by the existing reflection-based dialect tests (no new test needed in this task — Task 2 adds behavior tests).

- [ ] **Step 1: Add the field to `keyMap`**

In `internal/tui/keys.go`, add a `CopyPath` field to the `keyMap` struct. Place it right after the `ClearLink` field in the link-related group:

```go
	NextLink  key.Binding
	PrevLink  key.Binding
	ClearLink key.Binding

	CopyPath key.Binding
```

- [ ] **Step 2: Bind it in `pagerKeys()`**

In the `pagerKeys()` return literal, add after the `ClearLink` binding:

```go
		ClearLink: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear link")),

		CopyPath: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy path")),
```

- [ ] **Step 3: Bind it in `modernKeys()`**

In the `modernKeys()` return literal, add after the `ClearLink` binding:

```go
		ClearLink: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear link")),

		CopyPath: key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("^y", "copy path")),
```

- [ ] **Step 4: Run the dialect tests to verify the field is bound in both factories**

Run: `go test ./internal/tui/ -run 'TestPagerKeys_AllActionsBound|TestModernKeys_AllActionsBound|TestKeys_HelpTextNonEmpty|TestKeys_NoOverlappingActions' -v`
Expected: PASS. (These iterate every `keyMap` field; they fail if `CopyPath` is unbound in a factory, has empty help, or collides with another key.)

- [ ] **Step 5: Verify the whole package still builds and tests pass**

Run: `go build ./... && go test ./internal/tui/`
Expected: PASS (dispatch not wired yet, but nothing should regress).

- [ ] **Step 6: Commit**

```bash
git add internal/tui/keys.go
git commit -m "feat(tui): add CopyPath keybinding (y / ctrl+y) to both dialects"
```

---

### Task 2: Dispatch the keybinding — copy current path + toast

**Files:**
- Modify: `internal/tui/input.go`
- Create: `internal/tui/copypath_test.go`

- [ ] **Step 1: Write the failing test (pager: copies the absolute path and toasts)**

Create `internal/tui/copypath_test.go`:

```go
package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModel_CopyPath_CopiesCurrentAbsolutePath(t *testing.T) {
	root := writeFixture(t)
	want := filepath.Join(root, "index.md")
	m := sized(t, root, want)

	var copied string
	m.copyToClipboard = func(s string) { copied = s }

	m = pressRune(t, m, 'y')

	if copied != want {
		t.Errorf("copyToClipboard got %q, want %q", copied, want)
	}
	if !strings.Contains(m.renderFooter(), "Copied path") {
		t.Errorf("footer should show a copy-path toast; got %q", m.renderFooter())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestModel_CopyPath_CopiesCurrentAbsolutePath -v`
Expected: FAIL — `y` currently does nothing (it falls through to the viewport), so `copied` stays `""`.

- [ ] **Step 3: Add the `copyCurrentPath` helper**

In `internal/tui/input.go`, add this helper (place it just above `handleContentKey`):

```go
// copyCurrentPath copies the absolute path of the currently-viewed file
// (or directory) to the clipboard and toasts the result in the footer.
// No-op when nothing is open.
func (m *Model) copyCurrentPath() {
	path := m.history.Current()
	if path == "" {
		return
	}
	m.copyToClipboard(path)
	if m.diag != nil {
		m.diag.Info("Copied path: " + path)
	}
}
```

- [ ] **Step 4: Wire dispatch in `handleContentKey`'s switch**

In `internal/tui/input.go`, inside `handleContentKey`'s `switch` (the one starting `case key.Matches(msg, m.keys.NextLink):`), add a new case after the `PrevLink` case:

```go
	case key.Matches(msg, m.keys.PrevLink):
		m.cycleLink(-1)
		return *m, nil
	case key.Matches(msg, m.keys.CopyPath):
		m.copyCurrentPath()
		return *m, nil
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/tui/ -run TestModel_CopyPath_CopiesCurrentAbsolutePath -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/input.go internal/tui/copypath_test.go
git commit -m "feat(tui): copy current file path on y / ctrl+y"
```

---

### Task 3: Cover the no-op and modern-dialect cases

**Files:**
- Modify: `internal/tui/copypath_test.go`

- [ ] **Step 1: Write the no-op test (nothing open → no copy, no toast)**

`writeFixture` puts `index.md` at the top level, which the model auto-opens, so `Current()` would be non-empty. To get an empty `Current()`, drive auto-open to find nothing by using a fixture whose only markdown lives in a subdirectory — auto-open is top-level only (see CLAUDE.md). The `writeNoTopLevelFixture` helper added in Step 2 provides exactly that.

Append to `internal/tui/copypath_test.go`:

```go
func TestModel_CopyPath_NoOpWhenNothingOpen(t *testing.T) {
	root := writeNoTopLevelFixture(t)
	m := sized(t, root, "")

	if m.history.Current() != "" {
		t.Fatalf("precondition: expected no file open, got %q", m.history.Current())
	}

	var calls int
	m.copyToClipboard = func(string) { calls++ }

	m = pressRune(t, m, 'y')

	if calls != 0 {
		t.Errorf("copy should be a no-op when nothing is open; got %d calls", calls)
	}
	if strings.Contains(m.renderFooter(), "Copied path") {
		t.Errorf("no toast expected when nothing is open; got %q", m.renderFooter())
	}
}
```

- [ ] **Step 2: Add the `writeNoTopLevelFixture` helper**

Append to `internal/tui/copypath_test.go` (kept local to this test file; it lays down markdown only under a subdirectory so the top-level auto-open finds nothing):

```go
// writeNoTopLevelFixture lays down markdown only inside a subdirectory, so
// the model's top-level auto-open (firstTopLevelFile) finds nothing and
// history.Current() stays empty.
func writeNoTopLevelFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	sub := filepath.Join(root, "notes")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "only.md"), []byte("# Only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
```

Add `"os"` to the import block of `internal/tui/copypath_test.go`:

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)
```

- [ ] **Step 3: Write the modern-dialect test (`ctrl+y` copies)**

Append to `internal/tui/copypath_test.go`:

```go
func TestModel_CopyPath_ModernChord(t *testing.T) {
	root := writeFixture(t)
	want := filepath.Join(root, "index.md")
	m := sizedWithOptions(t, root, want, Options{Dialect: "modern"})

	var copied string
	m.copyToClipboard = func(s string) { copied = s }

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlY})

	if copied != want {
		t.Errorf("ctrl+y copyToClipboard got %q, want %q", copied, want)
	}
}
```

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `go test ./internal/tui/ -run 'TestModel_CopyPath' -v`
Expected: PASS for all three (`CopiesCurrentAbsolutePath`, `NoOpWhenNothingOpen`, `ModernChord`).

If `TestModel_CopyPath_NoOpWhenNothingOpen` fails its precondition (`Current()` not empty), it means auto-open behaved differently than expected — STOP and verify `firstTopLevelFile`'s top-level-only contract in `internal/tui/model.go` rather than weakening the assertion.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/copypath_test.go
git commit -m "test(tui): cover copy-path no-op and modern chord"
```

---

### Task 4: Document the keybinding in CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add a one-line mention in the "What this is" summary**

In `CLAUDE.md`, find the opening paragraph under `## What this is` that lists the key actions (`^p`, `/`, `Ctrl+F`, `^b`, `h`/`l`). Add copy-path to that sentence. Replace:

```
`h`/`l` navigate browser-style history (pager dialect; modern uses `Alt+←/→`).
```

with:

```
`h`/`l` navigate browser-style history (pager dialect; modern uses `Alt+←/→`), and `y` (pager) / `^y` (modern) copies the current file's absolute path to the clipboard.
```

- [ ] **Step 2: Verify the full suite is green and the tree is clean**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS across all packages.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: note y / ctrl+y copy-current-path keybinding"
```

---

## Final verification

- [ ] Run the full race-enabled suite (mirrors CI): `go test -race ./...` → PASS.
- [ ] Manual smoke (real terminal): `go run ./cmd/hypogeum .`, open a file, press `y`, paste elsewhere to confirm the absolute path landed on the clipboard and the footer showed `Copied path: …`.

## Self-review notes

- **Spec coverage:** keys (Task 1), copy + toast behavior (Task 2), absolute-path source / empty no-op / dual-dialect (Tasks 2–3), docs (Task 4). The spec's "reuse `copyToClipboard` / `diag.Info` / no new tick" all fall out of Task 2's helper.
- **Dispatch placement** matches the updated spec: `handleContentKey`, disabled while a modal is open.
- **No new dialect test** is required — the four reflection-based tests in `keys_test.go` cover the new field; Task 1 Step 4 runs them explicitly.

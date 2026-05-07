package tui

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// testDiagOpts returns options pointing the file log at logPath. If
// logPath is empty, file logging is disabled (covers the "no writable
// path" branch).
func testDiagOpts(t *testing.T, logPath string) diagOpts {
	t.Helper()
	return diagOpts{
		LogPath: logPath,
		Now:     time.Now,
	}
}

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

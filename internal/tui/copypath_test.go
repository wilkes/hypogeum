package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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


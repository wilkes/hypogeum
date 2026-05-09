package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// writePreselectFixture lays down a small vault where a.md links to b.md
// and b.md links back to a.md, so backlink-follow / Back / Forward all have
// reciprocal targets to match. Returns the root and absolute paths to a, b.
func writePreselectFixture(t *testing.T) (root, aAbs, bAbs string) {
	t.Helper()
	root = t.TempDir()
	files := map[string]string{
		"a.md": "# A\n\nLink to [b](b.md).\n",
		"b.md": "# B\n\nLink back to [a](a.md).\n",
	}
	for rel, body := range files {
		full := filepath.Join(root, rel)
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	aAbs = filepath.Join(root, "a.md")
	bAbs = filepath.Join(root, "b.md")
	return root, aAbs, bAbs
}

// TestPreselect_DefaultPathUnchanged confirms that a plain refreshContent
// (no navigation, no pending field) still resets linkCursor to -1. This
// guards against the new logic accidentally activating when the field is
// empty.
func TestPreselect_DefaultPathUnchanged(t *testing.T) {
	root, aAbs, _ := writePreselectFixture(t)
	m := sized(t, root, aAbs)
	if m.content.linkCursor != -1 {
		t.Fatalf("expected linkCursor=-1 on initial render, got %d", m.content.linkCursor)
	}
	if m.pendingPreselectTarget != "" {
		t.Fatalf("expected pendingPreselectTarget empty, got %q", m.pendingPreselectTarget)
	}
	// Drive a redundant refresh; cursor should still be -1.
	m.refreshContent(aAbs)
	if m.content.linkCursor != -1 {
		t.Fatalf("expected linkCursor=-1 after redundant refresh, got %d", m.content.linkCursor)
	}
	_ = tea.KeyMsg{} // silence unused import; tea is used by other tests in this file
}

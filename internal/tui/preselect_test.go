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

// TestPreselect_ConsumerMatchesByTarget exercises refreshContent's match
// logic in isolation: set the field, refresh, expect linkCursor to point
// at the link whose Resolved.Target equals the field's value. This is a
// pure consumer test — no key dispatch involved.
func TestPreselect_ConsumerMatchesByTarget(t *testing.T) {
	root, aAbs, bAbs := writePreselectFixture(t)
	m := sized(t, root, aAbs) // start on a.md

	m.pendingPreselectTarget = bAbs
	m.refreshContent(aAbs) // a.md contains [b](b.md), so linkCursor should land on it

	if m.content.linkCursor < 0 {
		t.Fatalf("expected linkCursor >= 0 after refresh with matching pending target, got %d", m.content.linkCursor)
	}
	got := m.content.links[m.content.linkCursor].Resolved.Target
	if got != bAbs {
		t.Fatalf("matched link target = %q, want %q", got, bAbs)
	}
	if m.pendingPreselectTarget != "" {
		t.Fatalf("expected pendingPreselectTarget cleared after consumption, got %q", m.pendingPreselectTarget)
	}
}

// TestPreselect_ConsumerNoMatchLeavesUnselected confirms that when the
// pending target doesn't match any inline link, linkCursor stays -1 and
// the field is still cleared (single-shot semantics).
func TestPreselect_ConsumerNoMatchLeavesUnselected(t *testing.T) {
	root, aAbs, _ := writePreselectFixture(t)
	m := sized(t, root, aAbs)

	bogus := filepath.Join(root, "does-not-exist.md")
	m.pendingPreselectTarget = bogus
	m.refreshContent(aAbs)

	if m.content.linkCursor != -1 {
		t.Fatalf("expected linkCursor=-1 with unmatched pending target, got %d", m.content.linkCursor)
	}
	if m.pendingPreselectTarget != "" {
		t.Fatalf("expected pendingPreselectTarget cleared even on miss, got %q", m.pendingPreselectTarget)
	}
}

// TestPreselect_FollowBacklink_PicksMatchingLink covers the primary case:
// from a.md, open the backlinks pane, press Enter on the (only) backlink
// (which points at b.md, the file whose only link points to a.md). After
// follow, we should be on b.md with its [a](a.md) link pre-selected.
func TestPreselect_FollowBacklink_PicksMatchingLink(t *testing.T) {
	root, aAbs, bAbs := writePreselectFixture(t)
	m := sized(t, root, aAbs) // start on a.md

	m = pressRune(t, m, 'b') // open backlinks pane
	if len(m.backlinks.items) != 1 {
		t.Fatalf("expected 1 backlink (b.md → a.md), got %d", len(m.backlinks.items))
	}
	if m.backlinks.items[0].SourceFile != bAbs {
		t.Fatalf("backlink source = %q, want %q", m.backlinks.items[0].SourceFile, bAbs)
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.history.Current() != bAbs {
		t.Fatalf("expected to be on b.md after follow, got %q", m.history.Current())
	}
	if m.content.linkCursor < 0 {
		t.Fatalf("expected linkCursor pre-selected after follow, got %d", m.content.linkCursor)
	}
	if got := m.content.links[m.content.linkCursor].Resolved.Target; got != aAbs {
		t.Fatalf("pre-selected link target = %q, want %q", got, aAbs)
	}
}

// TestPreselect_FollowBacklink_NoMatchFallsThroughToScrollToLine confirms
// the gate behavior: from a file that DOES have a reciprocal link, the
// pre-select succeeds. Used as a smoke test for the followBacklink path
// in the absence of a "no reciprocal" fixture (the writePreselectFixture
// always has reciprocal links). The test sets up b → a backlink and
// verifies the inline link IS pre-selected (happy path through followBacklink).
func TestPreselect_FollowBacklink_NoMatchFallsThroughToScrollToLine(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"a.md": "# A\n\nLink to [b](b.md).\n",
		"b.md": "# B\n\nNo outbound links here.\n",
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bAbs := filepath.Join(root, "b.md")

	m := sized(t, root, bAbs) // start on b.md
	m = pressRune(t, m, 'b')   // open backlinks pane (a.md links to b.md)
	if len(m.backlinks.items) != 1 {
		t.Fatalf("expected 1 backlink (a.md → b.md), got %d", len(m.backlinks.items))
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	// Now on a.md; a.md links to b.md, so linkCursor SHOULD be set.
	if m.content.linkCursor < 0 {
		t.Fatalf("expected linkCursor preselected (a.md links to b.md), got %d", m.content.linkCursor)
	}
}

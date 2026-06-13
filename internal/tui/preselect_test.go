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
	if m.pending.preselectTarget != "" {
		t.Fatalf("expected pending.preselectTarget empty, got %q", m.pending.preselectTarget)
	}
	// Drive a redundant refresh; cursor should still be -1.
	m.refreshContent(aAbs)
	if m.content.linkCursor != -1 {
		t.Fatalf("expected linkCursor=-1 after redundant refresh, got %d", m.content.linkCursor)
	}
}

// TestPreselect_ConsumerMatchesByTarget exercises refreshContent's match
// logic in isolation: set the field, refresh, expect linkCursor to point
// at the link whose Resolved.Target equals the field's value. This is a
// pure consumer test — no key dispatch involved.
func TestPreselect_ConsumerMatchesByTarget(t *testing.T) {
	root, aAbs, bAbs := writePreselectFixture(t)
	m := sized(t, root, aAbs) // start on a.md

	m.pending.preselectTarget = bAbs
	m.refreshContent(aAbs) // a.md contains [b](b.md), so linkCursor should land on it

	if m.content.linkCursor < 0 {
		t.Fatalf("expected linkCursor >= 0 after refresh with matching pending target, got %d", m.content.linkCursor)
	}
	got := m.content.links[m.content.linkCursor].Resolved.Target
	if got != bAbs {
		t.Fatalf("matched link target = %q, want %q", got, bAbs)
	}
	if m.pending.preselectTarget != "" {
		t.Fatalf("expected pending.preselectTarget cleared after consumption, got %q", m.pending.preselectTarget)
	}
}

// TestPreselect_ConsumerNoMatchLeavesUnselected confirms that when the
// pending target doesn't match any inline link, linkCursor stays -1 and
// the field is still cleared (single-shot semantics).
func TestPreselect_ConsumerNoMatchLeavesUnselected(t *testing.T) {
	root, aAbs, _ := writePreselectFixture(t)
	m := sized(t, root, aAbs)

	bogus := filepath.Join(root, "does-not-exist.md")
	m.pending.preselectTarget = bogus
	m.refreshContent(aAbs)

	if m.content.linkCursor != -1 {
		t.Fatalf("expected linkCursor=-1 with unmatched pending target, got %d", m.content.linkCursor)
	}
	if m.pending.preselectTarget != "" {
		t.Fatalf("expected pending.preselectTarget cleared even on miss, got %q", m.pending.preselectTarget)
	}
}

// TestPreselect_FollowBacklink_PicksMatchingLink covers the primary case:
// from a.md, open the backlinks modal, press Enter on the (only) backlink
// (which points at b.md, the file whose only link points to a.md). After
// follow, we should be on b.md with its [a](a.md) link pre-selected.
func TestPreselect_FollowBacklink_PicksMatchingLink(t *testing.T) {
	root, aAbs, bAbs := writePreselectFixture(t)
	m := sized(t, root, aAbs) // start on a.md

	m = pressRune(t, m, 'b') // open backlinks modal
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
	m = pressRune(t, m, 'b')  // open backlinks modal (a.md links to b.md)
	if len(m.backlinks.items) != 1 {
		t.Fatalf("expected 1 backlink (a.md → b.md), got %d", len(m.backlinks.items))
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	// Now on a.md; a.md links to b.md, so linkCursor SHOULD be set.
	if m.content.linkCursor < 0 {
		t.Fatalf("expected linkCursor preselected (a.md links to b.md), got %d", m.content.linkCursor)
	}
}

// TestPreselect_Back_RestoresLink: from a.md, follow [b](b.md), then press
// 'h' to Back. We should be on a.md again with the [b](b.md) link
// pre-selected.
func TestPreselect_Back_RestoresLink(t *testing.T) {
	root, aAbs, bAbs := writePreselectFixture(t)
	m := sized(t, root, aAbs)

	// Navigate a → b by following the inline link.
	m = switchToContent(t, m)
	m = pressRune(t, m, 'n') // select first link in a.md (which is [b](b.md))
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.history.Current() != bAbs {
		t.Fatalf("expected on b.md after follow, got %q", m.history.Current())
	}

	// Press 'h' to Back.
	m = pressRune(t, m, 'h')
	if m.history.Current() != aAbs {
		t.Fatalf("expected on a.md after Back, got %q", m.history.Current())
	}
	if m.content.linkCursor < 0 {
		t.Fatalf("expected linkCursor preselected after Back, got %d", m.content.linkCursor)
	}
	if got := m.content.links[m.content.linkCursor].Resolved.Target; got != bAbs {
		t.Fatalf("preselected link target = %q, want %q", got, bAbs)
	}
}

// TestPreselect_Forward_RestoresLink: a → b → Back to a → Forward to b.
// On Forward arrival at b.md, b.md's [a](a.md) link should be pre-selected.
func TestPreselect_Forward_RestoresLink(t *testing.T) {
	root, aAbs, bAbs := writePreselectFixture(t)
	m := sized(t, root, aAbs)

	m = switchToContent(t, m)
	m = pressRune(t, m, 'n')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // a → b
	m = pressRune(t, m, 'h')                           // back to a
	if m.history.Current() != aAbs {
		t.Fatalf("setup: expected on a.md, got %q", m.history.Current())
	}

	// Forward (default key binding is 'l').
	m = pressRune(t, m, 'l')
	if m.history.Current() != bAbs {
		t.Fatalf("expected on b.md after Forward, got %q", m.history.Current())
	}
	if m.content.linkCursor < 0 {
		t.Fatalf("expected linkCursor preselected after Forward, got %d", m.content.linkCursor)
	}
	if got := m.content.links[m.content.linkCursor].Resolved.Target; got != aAbs {
		t.Fatalf("preselected link target = %q, want %q", got, aAbs)
	}
}

// TestPreselect_FirstMatchWins covers the ambiguity rule: when a source
// file contains two inline links to the same target, the one with the
// lower index in m.content.links is selected.
func TestPreselect_FirstMatchWins(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		// Two distinct links to b.md, separated by some prose so they end
		// up as two separate Link entries.
		"a.md": "# A\n\nFirst [b](b.md), then more text, then [b again](b.md).\n",
		"b.md": "# B\n\nLink to [a](a.md).\n",
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	aAbs := filepath.Join(root, "a.md")
	bAbs := filepath.Join(root, "b.md")

	m := sized(t, root, bAbs)
	m = pressRune(t, m, 'b')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // follow the b→a backlink, lands on a

	if m.history.Current() != aAbs {
		t.Fatalf("expected on a.md, got %q", m.history.Current())
	}
	if m.content.linkCursor != 0 {
		t.Fatalf("expected first matching link (index 0), got %d", m.content.linkCursor)
	}
	// Sanity: a.md should have at least 2 links to b.md.
	matches := 0
	for _, l := range m.content.links {
		if l.Resolved.Target == bAbs {
			matches++
		}
	}
	if matches < 2 {
		t.Fatalf("fixture should have 2+ links to b.md, got %d", matches)
	}
}

// TestPreselect_ClearedAfterConsumption confirms two consecutive
// Back-then-Forward cycles each get an independent pre-select decision —
// no leakage from one navigation to the next.
func TestPreselect_ClearedAfterConsumption(t *testing.T) {
	root, aAbs, bAbs := writePreselectFixture(t)
	m := sized(t, root, aAbs)

	m = switchToContent(t, m)
	m = pressRune(t, m, 'n')
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // a → b
	m = pressRune(t, m, 'h')                           // back to a
	if m.pending.preselectTarget != "" {
		t.Fatalf("expected field cleared after Back consumed it, got %q", m.pending.preselectTarget)
	}

	// Back again: should be a no-op (no further history). Pre-select
	// field stays clear.
	m = pressRune(t, m, 'h')
	if m.pending.preselectTarget != "" {
		t.Fatalf("expected field cleared after no-op Back, got %q", m.pending.preselectTarget)
	}
	_ = bAbs
}

// TestPreselect_WatcherEventConsumesQuietly documents the accepted race:
// if a watcher refresh fires between leave and arrival, the field gets
// consumed (cleared) by the unrelated refresh, so the next intentional
// navigation gets no pre-select.
func TestPreselect_WatcherEventConsumesQuietly(t *testing.T) {
	root, aAbs, bAbs := writePreselectFixture(t)
	m := sized(t, root, aAbs)

	// Manually set the field to simulate "leave" without driving a navigation.
	m.pending.preselectTarget = bAbs

	// Fire a redundant refreshContent for the current file (simulating a
	// watcher FileModified). This consumes the field. a.md DOES contain
	// a link to b.md, so the cursor will end up selected — but the field
	// is now empty and won't fire on a subsequent intentional navigation.
	m.refreshContent(aAbs)
	if m.pending.preselectTarget != "" {
		t.Fatalf("expected field cleared after watcher refresh, got %q", m.pending.preselectTarget)
	}
}

// TestPreselect_OnlyLocalFileLinks confirms that links with non-local
// kinds (external URLs, anchors) are skipped during matching even if
// their Href happens to equal the pending target string.
func TestPreselect_OnlyLocalFileLinks(t *testing.T) {
	root := t.TempDir()
	// a.md contains an external URL whose string we'll use as the target.
	files := map[string]string{
		"a.md": "# A\n\nExternal: [example](https://example.test/).\n",
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	aAbs := filepath.Join(root, "a.md")

	m := sized(t, root, aAbs)
	// Set the pending target to the external URL string. Even if there
	// were a Link with Href == this URL, its Kind is LinkExternal not
	// LinkLocalFile, so the consumer should skip it.
	m.pending.preselectTarget = "https://example.test/"
	m.refreshContent(aAbs)
	if m.content.linkCursor != -1 {
		t.Fatalf("expected linkCursor=-1 (only LinkLocalFile is eligible), got %d", m.content.linkCursor)
	}
}

// TestPreselect_FieldClearedOnReadError confirms the single-shot invariant
// holds even on the read-failure early return: a target set before a
// refreshContent that fails to read the file must not leak into the next
// refreshContent.
func TestPreselect_FieldClearedOnReadError(t *testing.T) {
	root, aAbs, bAbs := writePreselectFixture(t)
	m := sized(t, root, aAbs)

	// Set the field, then trigger a read error by passing a non-existent
	// path. The consumer's clear must run before the early return.
	m.pending.preselectTarget = bAbs
	m.refreshContent(filepath.Join(root, "does-not-exist.md"))
	if m.pending.preselectTarget != "" {
		t.Fatalf("expected field cleared after read-failure refresh, got %q", m.pending.preselectTarget)
	}

	// Now refresh a real file that contains a link to b.md. With the
	// field correctly cleared above, no preselect should fire.
	m.refreshContent(aAbs)
	if m.content.linkCursor != -1 {
		t.Fatalf("expected linkCursor=-1 (field was cleared by prior failure), got %d", m.content.linkCursor)
	}
}

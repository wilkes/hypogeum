package tui

import (
	"testing"

	"github.com/sahilm/fuzzy"
)

// TestFuzzyDependencyAvailable is a sanity check that the matcher import
// works and the API shape matches what the picker code expects.
func TestFuzzyDependencyAvailable(t *testing.T) {
	matches := fuzzy.Find("hyp", []string{"hypogeum.md", "other.md"})
	if len(matches) != 1 {
		t.Fatalf("Find: got %d matches, want 1", len(matches))
	}
	if matches[0].Str != "hypogeum.md" {
		t.Errorf("Find: matched %q, want %q", matches[0].Str, "hypogeum.md")
	}
	if len(matches[0].MatchedIndexes) == 0 {
		t.Errorf("MatchedIndexes empty; expected positions in hypogeum.md")
	}
}

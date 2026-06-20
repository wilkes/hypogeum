package query

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestSearchDeterministic verifies that two consecutive calls to Search
// on the same vault with the same term produce identical results.
// This catches the race-dependent ordering that the old search.Search
// (with a cap) introduced when more hits existed than the cap.
func TestSearchDeterministic(t *testing.T) {
	dir := withTempStore(t)
	// Several files, each with the term on known lines, to give the
	// worker goroutines multiple files to race over.
	files := map[string]string{
		"alpha.md": "needle on line one\nno match\nneedle on line three\n",
		"beta.md":  "nothing\nneedle here\n",
		"gamma.md": "needle first\nneedle second\nneedle third\n",
		"delta.md": "no matches at all\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	first, err := Search(dir, "needle", 0) // 0 = no cap
	if err != nil {
		t.Fatalf("first Search: %v", err)
	}
	second, err := Search(dir, "needle", 0)
	if err != nil {
		t.Fatalf("second Search: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("Search results differ between runs:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

// TestSearchCapsToN verifies that a positive -n value caps the returned
// results without exceeding it.
func TestSearchCapsToN(t *testing.T) {
	dir := withTempStore(t)
	// 20 lines, all matching.
	var body string
	for i := 0; i < 20; i++ {
		body += "needle line\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "big.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Search(dir, "needle", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) > 5 {
		t.Errorf("got %d results with n=5, want ≤5", len(got))
	}
	if len(got) == 0 {
		t.Error("got 0 results, want >0")
	}
}

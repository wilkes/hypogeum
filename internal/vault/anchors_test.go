package vault

import (
	"testing"
)

func TestExtractAnchors_Headings(t *testing.T) {
	src := "# Top\n\nIntro paragraph.\n\n## Sub Section\n\nbody\n\n### Deep One\n"
	got := extractAnchors(src)

	want := map[string]int{
		"top":         1,
		"sub-section": 5,
		"deep-one":    9,
	}
	if len(got.headings) != len(want) {
		t.Fatalf("headings len = %d, want %d (%v)", len(got.headings), len(want), got.headings)
	}
	for slug, line := range want {
		if got.headings[slug] != line {
			t.Errorf("headings[%q] = %d, want %d", slug, got.headings[slug], line)
		}
	}
}

package embed

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseEmbedToken_WholeFile(t *testing.T) {
	got, err := ParseEmbedToken("main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := &Embed{Path: "main.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseEmbedToken_SingleLine(t *testing.T) {
	got, err := ParseEmbedToken("main.go#L5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := &Embed{Path: "main.go", Range: &LineRange{Start: 5, End: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseEmbedToken_Range(t *testing.T) {
	got, err := ParseEmbedToken("a/b/main.go#L10-L20")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := &Embed{Path: "a/b/main.go", Range: &LineRange{Start: 10, End: 20}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseEmbedToken_RangeWithContext(t *testing.T) {
	got, err := ParseEmbedToken("main.go#L10-L20+3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := &Embed{Path: "main.go", Range: &LineRange{Start: 10, End: 20}, ContextLines: 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseEmbedToken_TrimmedWhitespace(t *testing.T) {
	got, err := ParseEmbedToken("  main.go#L5  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Path != "main.go" || got.Range == nil || got.Range.Start != 5 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseEmbedToken_Errors(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantSubstr string // substring of the expected error message
	}{
		{"empty", "", "empty embed token"},
		{"whitespace_only", "  ", "empty embed token"},
		{"empty_line_spec", "main.go#L", "invalid line spec"},
		{"zero_start", "main.go#L0", "1-indexed"},
		{"non_numeric_line", "main.go#Labc", "invalid line spec"},
		{"inverted_range", "main.go#L10-L5", "inverted range"},
		{"missing_context_number", "main.go#L10-L20+", "invalid line spec"},
		{"non_numeric_context", "main.go#L10-L20+abc", "invalid line spec"},
		{"partial_range_trailing", "main.go#L10-", "invalid line spec"},
		{"partial_range_leading", "main.go#L-L20", "invalid line spec"},
		{"non_line_fragment", "main.go#XYZ", "invalid line spec"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseEmbedToken(tc.body)
			if err == nil {
				t.Fatalf("body %q: expected error, got %+v", tc.body, got)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("body %q: error %q does not contain %q", tc.body, err.Error(), tc.wantSubstr)
			}
		})
	}
}

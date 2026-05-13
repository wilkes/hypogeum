package embed

import (
	"reflect"
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
	cases := []string{
		"",                    // empty
		"  ",                  // whitespace only
		"main.go#L",           // empty line spec
		"main.go#L0",          // zero is invalid (1-indexed)
		"main.go#Labc",        // non-numeric
		"main.go#L10-L5",      // inverted range
		"main.go#L10-L20+",    // missing context number
		"main.go#L10-L20+abc", // non-numeric context
		"main.go#L10-",        // partial range
		"main.go#L-L20",       // partial range
		"main.go#XYZ",         // not a line spec at all (would route to wikilink/anchor elsewhere)
	}
	for _, body := range cases {
		if got, err := ParseEmbedToken(body); err == nil {
			t.Errorf("body %q: expected error, got %+v", body, got)
		}
	}
}

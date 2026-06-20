package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIsQueryVerb(t *testing.T) {
	for _, v := range []string{"search", "links", "recent", "neighbors"} {
		if !isQueryVerb(v) {
			t.Errorf("isQueryVerb(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"foo.md", "", "Search", "help"} {
		if isQueryVerb(v) {
			t.Errorf("isQueryVerb(%q) = true, want false", v)
		}
	}
}

func TestRunQueryLinks(t *testing.T) {
	dir := t.TempDir()
	foo := filepath.Join(dir, "foo.md")
	if err := os.WriteFile(foo, []byte("See [[bar]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.md"), []byte("# bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := runQuery([]string{"links", "--vault", dir, foo}, &out)
	if err != nil {
		t.Fatalf("runQuery: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0]["kind"] != "wikilink" {
		t.Errorf("unexpected links output: %s", out.String())
	}
}

func TestRunQueryUnknownFlag(t *testing.T) {
	var out bytes.Buffer
	if err := runQuery([]string{"links", "--bogus"}, &out); err == nil {
		t.Error("runQuery with unknown flag returned nil error, want non-nil")
	}
}

func TestRunQueryMissingArg(t *testing.T) {
	var out bytes.Buffer
	if err := runQuery([]string{"links"}, &out); err == nil {
		t.Error("runQuery links with no file returned nil error, want non-nil")
	}
}

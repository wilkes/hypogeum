package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wilkes/hypogeum/internal/query"
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

func TestRunQuerySearchFlagAfterPositional(t *testing.T) {
	dir := t.TempDir()
	// A single file with multiple lines matching the term, so an
	// uncapped search would return >1 hit.
	if err := os.WriteFile(filepath.Join(dir, "note.md"),
		[]byte("alpha needle\nbeta needle\ngamma needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	// -n placed AFTER the positional term must be honored.
	err := runQuery([]string{"search", "--vault", dir, "needle", "-n", "1"}, &out)
	if err != nil {
		t.Fatalf("runQuery: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
	}
	if len(got) != 1 {
		t.Errorf("expected exactly 1 hit with -n 1, got %d: %s", len(got), out.String())
	}
}

func TestRunQueryLinksVaultAfterFile(t *testing.T) {
	dir := t.TempDir()
	foo := filepath.Join(dir, "foo.md")
	if err := os.WriteFile(foo, []byte("See [[bar]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.md"), []byte("# bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	// --vault placed AFTER the file positional must be honored.
	err := runQuery([]string{"links", foo, "--vault", dir}, &out)
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

func TestRunQueryLinksVaultEqualsForm(t *testing.T) {
	dir := t.TempDir()
	foo := filepath.Join(dir, "foo.md")
	if err := os.WriteFile(foo, []byte("See [[bar]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.md"), []byte("# bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	// --vault=dir (equals form) after the positional must survive reordering.
	err := runQuery([]string{"links", foo, "--vault=" + dir}, &out)
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

func TestRunQueryLinksRejectsNFlag(t *testing.T) {
	var out bytes.Buffer
	// -n is not a valid flag for links; it must be a parse error,
	// not a silent no-op.
	if err := runQuery([]string{"links", "foo.md", "-n", "5"}, &out); err == nil {
		t.Error("runQuery links with -n returned nil error, want non-nil")
	}
}

func TestRunQueryNeighborsRejectsNFlag(t *testing.T) {
	var out bytes.Buffer
	if err := runQuery([]string{"neighbors", "foo.md", "-n", "5"}, &out); err == nil {
		t.Error("runQuery neighbors with -n returned nil error, want non-nil")
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

func TestRunQueryMissingArgSearch(t *testing.T) {
	var out bytes.Buffer
	if err := runQuery([]string{"search"}, &out); err == nil {
		t.Error("runQuery search with no term returned nil error, want non-nil")
	}
}

func TestRunQueryMissingArgNeighbors(t *testing.T) {
	var out bytes.Buffer
	if err := runQuery([]string{"neighbors"}, &out); err == nil {
		t.Error("runQuery neighbors with no file returned nil error, want non-nil")
	}
}

func TestRunQueryGraph(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("[[b]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("# b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runQuery([]string{"graph", "--vault", dir}, &buf); err != nil {
		t.Fatalf("runQuery graph: %v", err)
	}

	var g query.Graph
	if err := json.Unmarshal(buf.Bytes(), &g); err != nil {
		t.Fatalf("unmarshal: %v (output: %s)", err, buf.String())
	}
	if len(g.Nodes) != 2 {
		t.Errorf("got %d nodes, want 2: %+v", len(g.Nodes), g.Nodes)
	}
	if len(g.Edges) != 1 || g.Edges[0].Kind != "wikilink" || g.Edges[0].Broken {
		t.Errorf("edges = %+v, want one resolved wikilink", g.Edges)
	}
}

func TestGraphIsQueryVerb(t *testing.T) {
	if !isQueryVerb("graph") {
		t.Error("graph should be a reserved query verb")
	}
}

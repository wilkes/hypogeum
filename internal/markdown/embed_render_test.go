package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderWithLinks_EmbedFromSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("line1\nline2\nline3\nline4\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	mdPath := filepath.Join(dir, "notes.md")
	mdSrc := "Before.\n\n![[main.go#L2-L3]]\n\nAfter.\n"

	r, err := NewRenderer(80)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	out, links, deps, err := r.RenderWithLinks(mdSrc, mdPath, nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if !strings.Contains(out, "line2") || !strings.Contains(out, "line3") {
		t.Fatalf("rendered output missing embed body:\n%s", out)
	}
	if strings.Contains(out, "line1") || strings.Contains(out, "line4") {
		t.Fatalf("embed leaked lines outside range:\n%s", out)
	}
	if len(deps) != 1 || deps[0] != src {
		t.Fatalf("deps = %v, want [%q]", deps, src)
	}
	if len(links) == 0 {
		t.Fatalf("expected at least one embed-derived Link")
	}
	found := false
	for _, l := range links {
		if l.Resolved.Target == src && l.Resolved.Range != nil &&
			l.Resolved.Range.Start == 2 && l.Resolved.Range.End == 3 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no embed link with range 2-3 in links: %+v", links)
	}
}

func TestRenderWithLinks_EmbedMissingFile(t *testing.T) {
	r, err := NewRenderer(80)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	mdSrc := "![[no-such-file.go#L1-L2]]\n"
	out, _, _, err := r.RenderWithLinks(mdSrc, "/tmp/notes.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	// Strip ANSI escapes before substring-checking: Glamour styles the
	// warning text in chunks, interleaving SGR sequences with the literal
	// characters, so a raw strings.Contains on the styled output fails.
	if !strings.Contains(stripANSI(out), "file not found") {
		t.Fatalf("expected warning text in output:\n%s", out)
	}
}

func TestRenderWithLinks_NoEmbedsReturnsEmptyDeps(t *testing.T) {
	r, err := NewRenderer(80)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	out, _, deps, err := r.RenderWithLinks("just plain prose\n", "/tmp/notes.md", nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(deps) != 0 {
		t.Fatalf("deps = %v, want empty", deps)
	}
	if out == "" {
		t.Fatalf("output should not be empty")
	}
}

func TestRenderWithLinks_EmbedDedupSameFileTwice(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("a\nb\nc\nd\ne\nf\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	mdPath := filepath.Join(dir, "notes.md")
	mdSrc := "![[main.go#L1-L2]]\n\n![[main.go#L5-L6]]\n"

	r, err := NewRenderer(80)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	_, links, deps, err := r.RenderWithLinks(mdSrc, mdPath, nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	if len(deps) != 1 || deps[0] != src {
		t.Fatalf("deps = %v, want [%q]", deps, src)
	}
	// Count synthetic embed links (Row == -1) for this target.
	count := 0
	for _, l := range links {
		if l.Row == -1 && l.Resolved.Target == src {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("embed link count = %d, want 2", count)
	}
}

func TestRenderWithLinks_EmbedRangePastEOFShowsSoftWarning(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "short.go")
	if err := os.WriteFile(src, []byte("only-line\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	mdPath := filepath.Join(dir, "notes.md")
	mdSrc := "![[short.go#L1-L99]]\n"

	r, err := NewRenderer(80)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	out, _, deps, err := r.RenderWithLinks(mdSrc, mdPath, nil)
	if err != nil {
		t.Fatalf("RenderWithLinks: %v", err)
	}
	// Dep still tracked (the file existed and we got a usable slice).
	if len(deps) != 1 {
		t.Fatalf("deps = %v, want exactly one entry", deps)
	}
	// The "file ends at line N" annotation should appear in the rendered
	// output. As with TestRenderWithLinks_EmbedMissingFile, Glamour
	// interleaves SGR sequences inside text, so strip ANSI first.
	if !strings.Contains(stripANSI(out), "file ends at line 1") {
		t.Fatalf("expected 'file ends at line 1' in rendered output:\n%s", out)
	}
}

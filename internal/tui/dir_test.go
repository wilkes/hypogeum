package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderDirListing_EmptyDirectoryHasHeaderAndParent(t *testing.T) {
	dir := t.TempDir()
	out, err := renderDirListing(dir)
	if err != nil {
		t.Fatalf("renderDirListing: %v", err)
	}
	if !strings.Contains(out, "# "+filepath.Base(dir)) {
		t.Errorf("missing header for %q in: %q", filepath.Base(dir), out)
	}
	if !strings.Contains(out, "`"+dir+"`") {
		t.Errorf("missing inline-code absolute path in: %q", out)
	}
	if !strings.Contains(out, "- [..](") {
		t.Errorf("missing parent link in: %q", out)
	}
}

func TestRenderDirListing_MixedEntriesSortDirsFirstThenAlphabetical(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "beta"))
	mustMkdir(t, filepath.Join(dir, "alpha"))
	mustWrite(t, filepath.Join(dir, "z.md"), "")
	mustWrite(t, filepath.Join(dir, "a.txt"), "")

	out, err := renderDirListing(dir)
	if err != nil {
		t.Fatalf("renderDirListing: %v", err)
	}

	want := []string{"[..]", "[alpha/]", "[beta/]", "[a.txt]", "[z.md]"}
	pos := 0
	for _, w := range want {
		idx := strings.Index(out[pos:], w)
		if idx < 0 {
			t.Errorf("missing %q (or out of order) in:\n%s", w, out)
			return
		}
		pos += idx + len(w)
	}
}

func TestRenderDirListing_HiddenEntriesSkipped(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".hidden.md"), "")
	mustMkdir(t, filepath.Join(dir, ".secret"))
	mustWrite(t, filepath.Join(dir, "visible.md"), "")

	out, err := renderDirListing(dir)
	if err != nil {
		t.Fatalf("renderDirListing: %v", err)
	}
	if strings.Contains(out, ".hidden.md") {
		t.Errorf("hidden file leaked into listing: %q", out)
	}
	if strings.Contains(out, ".secret") {
		t.Errorf("hidden dir leaked into listing: %q", out)
	}
	if !strings.Contains(out, "visible.md") {
		t.Errorf("visible entry missing from listing: %q", out)
	}
}

func TestRenderDirListing_AbsoluteHrefs(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "note.md"), "")
	mustMkdir(t, filepath.Join(dir, "sub"))

	out, err := renderDirListing(dir)
	if err != nil {
		t.Fatalf("renderDirListing: %v", err)
	}
	if !strings.Contains(out, "("+filepath.Join(dir, "note.md")+")") {
		t.Errorf("expected absolute href for note.md in: %q", out)
	}
	if !strings.Contains(out, "("+filepath.Join(dir, "sub")+")") {
		t.Errorf("expected absolute href for sub/ in: %q", out)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildEmptyVault(t *testing.T) {
	dir := t.TempDir()
	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if v == nil {
		t.Fatalf("Build returned nil vault")
	}
	if got := v.Backlinks(filepath.Join(dir, "anything.md")); len(got) != 0 {
		t.Fatalf("empty vault Backlinks: got %d want 0", len(got))
	}
}

func TestResolveOnEmptyVault(t *testing.T) {
	dir := t.TempDir()
	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if path, ok := v.Resolve(filepath.Join(dir, "from.md"), "missing", "", ""); ok {
		t.Fatalf("expected unresolved, got %q", path)
	}
}

func writeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	return full
}

func TestBuildIndexesFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "links to [[b]] and [c](c.md).")
	writeFile(t, dir, "b.md", "i am b.")
	writeFile(t, dir, "c.md", "i am c.")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := v.fileCount(); got != 3 {
		t.Fatalf("fileCount: got %d want 3", got)
	}
}

func TestBacklinksFromStandardAndWikilinks(t *testing.T) {
	dir := t.TempDir()
	bAbs, _ := filepath.Abs(filepath.Join(dir, "b.md"))
	writeFile(t, dir, "a.md", "links to [[b]] and [b again](b.md).")
	writeFile(t, dir, "b.md", "i am b.")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// We expect 2 backlinks to b.md from a.md (one wikilink, one stdlink).
	// The stdlink resolves during indexFile; the wikilink resolves in
	// resolveAllRefs (Task 10).
	got := v.Backlinks(bAbs)
	if len(got) != 2 {
		t.Fatalf("Backlinks(b): got %d want 2 (%+v)", len(got), got)
	}
	hasWiki, hasStd := false, false
	for _, b := range got {
		if b.Kind == BacklinkWikilink {
			hasWiki = true
		}
		if b.Kind == BacklinkStdLink {
			hasStd = true
		}
	}
	if !hasWiki || !hasStd {
		t.Fatalf("expected both wikilink and stdlink backlinks, got %+v", got)
	}
}

type recordingDiag struct {
	infos, warns, errors []string
}

func (r *recordingDiag) Info(m string)  { r.infos = append(r.infos, m) }
func (r *recordingDiag) Warn(m string)  { r.warns = append(r.warns, m) }
func (r *recordingDiag) Error(m string) { r.errors = append(r.errors, m) }

func TestBuildEmitsWarnOnUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ok.md", "fine")
	bad := writeFile(t, dir, "bad.md", "fine")
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Skipf("chmod 000 not supported: %v", err)
	}
	defer os.Chmod(bad, 0o644)

	r := &recordingDiag{}
	v, err := Build(dir, r)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if v == nil {
		t.Fatalf("Build returned nil")
	}
	if len(r.warns) == 0 {
		t.Fatalf("expected a Warn diagnostic for unreadable file, got none")
	}
}

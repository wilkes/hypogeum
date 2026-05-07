package vault

import (
	"path/filepath"
	"testing"
)

func TestResolve_ExactBasenameCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Foo.md", "i am foo")
	writeFile(t, dir, "from.md", "links to [[FOO]]")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	from, _ := filepath.Abs(filepath.Join(dir, "from.md"))
	want, _ := filepath.Abs(filepath.Join(dir, "Foo.md"))

	got, ok := v.Resolve(from, "FOO", "", "")
	if !ok {
		t.Fatalf("Resolve returned ok=false")
	}
	if got != want {
		t.Fatalf("Resolve: got %q want %q", got, want)
	}
}

func TestResolve_MissReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "from.md", "links to [[Nonexistent]]")
	v, _ := Build(dir, NopDiagnostics{})
	from, _ := filepath.Abs(filepath.Join(dir, "from.md"))

	if _, ok := v.Resolve(from, "Nonexistent", "", ""); ok {
		t.Fatalf("expected unresolved")
	}
}

func TestResolve_ProximityTiebreaker(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "near/from.md", "links to [[shared]]")
	writeFile(t, dir, "near/shared.md", "near version")
	writeFile(t, dir, "far/away/shared.md", "far version")

	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	from, _ := filepath.Abs(filepath.Join(dir, "near/from.md"))
	want, _ := filepath.Abs(filepath.Join(dir, "near/shared.md"))

	got, ok := v.Resolve(from, "shared", "", "")
	if !ok {
		t.Fatalf("ok=false")
	}
	if got != want {
		t.Fatalf("expected near version %q, got %q", want, got)
	}
}

func TestBacklinksFromWikilinksAfterResolve(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "links to [[b]] and [b again](b.md).")
	writeFile(t, dir, "b.md", "i am b.")
	v, err := Build(dir, NopDiagnostics{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	bAbs, _ := filepath.Abs(filepath.Join(dir, "b.md"))
	got := v.Backlinks(bAbs)
	if len(got) != 2 {
		t.Fatalf("Backlinks: got %d want 2 (%+v)", len(got), got)
	}
}

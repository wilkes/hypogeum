package vault

import (
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

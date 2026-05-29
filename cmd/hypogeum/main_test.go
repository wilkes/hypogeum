package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFileForTest(path string) error {
	return os.WriteFile(path, []byte("# note\n"), 0o644)
}

func TestResolveTarget_NoArgsIsCwd(t *testing.T) {
	roots, initial, err := resolveTarget(nil)
	if err != nil {
		t.Fatalf("resolveTarget(nil): %v", err)
	}
	if len(roots) != 1 || initial != "" {
		t.Fatalf("resolveTarget(nil) = (%v, %q), want one root, no file", roots, initial)
	}
}

func TestResolveTarget_SingleDir(t *testing.T) {
	dir := t.TempDir()
	roots, initial, err := resolveTarget([]string{dir})
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	abs, _ := filepath.Abs(dir)
	if len(roots) != 1 || roots[0] != abs || initial != "" {
		t.Fatalf("resolveTarget(dir) = (%v, %q), want ([%q], \"\")", roots, initial, abs)
	}
}

func TestResolveTarget_SingleFileRootsAtParent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	if err := writeFileForTest(file); err != nil {
		t.Fatal(err)
	}
	roots, initial, err := resolveTarget([]string{file})
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	absDir, _ := filepath.Abs(dir)
	absFile, _ := filepath.Abs(file)
	if len(roots) != 1 || roots[0] != absDir || initial != absFile {
		t.Fatalf("resolveTarget(file) = (%v, %q), want ([%q], %q)", roots, initial, absDir, absFile)
	}
}

func TestResolveTarget_MultipleDirsBecomeRoots(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	roots, initial, err := resolveTarget([]string{a, b})
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	absA, _ := filepath.Abs(a)
	absB, _ := filepath.Abs(b)
	if len(roots) != 2 || roots[0] != absA || roots[1] != absB || initial != "" {
		t.Fatalf("resolveTarget(a,b) = (%v, %q), want ([%q %q], \"\")", roots, initial, absA, absB)
	}
}

func TestResolveTarget_FileAmongMultipleIsError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	if err := writeFileForTest(file); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if _, _, err := resolveTarget([]string{other, file}); err == nil {
		t.Fatalf("resolveTarget(dir, file) = nil error, want a usage error")
	}
}

func TestResolveTarget_NonexistentPathIsError(t *testing.T) {
	if _, _, err := resolveTarget([]string{"/no/such/dir", "/also/missing"}); err == nil {
		t.Fatalf("resolveTarget(missing) = nil error, want error")
	}
}

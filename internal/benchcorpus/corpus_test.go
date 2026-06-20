package benchcorpus

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestGenerate_Deterministic(t *testing.T) {
	a := Generate(t.TempDir(), 42, 20, 3)
	b := Generate(t.TempDir(), 42, 20, 3)
	if len(a.Files) != len(b.Files) {
		t.Fatalf("file count differs: %d vs %d", len(a.Files), len(b.Files))
	}
	for i := range a.Files {
		da, err := os.ReadFile(a.Files[i])
		if err != nil {
			t.Fatal(err)
		}
		db, err := os.ReadFile(b.Files[i])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(da, db) {
			t.Fatalf("file %d bytes differ between runs with same seed", i)
		}
	}
}

func TestGenerate_Invariants(t *testing.T) {
	c := Generate(t.TempDir(), 1, 15, 2)
	if len(c.Files) != 15 {
		t.Fatalf("want 15 files, got %d", len(c.Files))
	}
	if c.Target == "" {
		t.Fatal("Target unset")
	}
	for _, p := range c.Files {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), SearchToken) {
			t.Errorf("%s missing SearchToken", p)
		}
	}
}

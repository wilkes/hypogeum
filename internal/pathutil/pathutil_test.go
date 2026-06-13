package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRelativeTo(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	tests := []struct {
		name   string
		base   string
		target string
		want   string
	}{
		{
			name:   "relative target resolves against base dir",
			base:   "/notes/index.md",
			target: "sub/page.md",
			want:   "/notes/sub/page.md",
		},
		{
			name:   "relative target with parent traversal",
			base:   "/notes/sub/page.md",
			target: "../other.md",
			want:   "/notes/other.md",
		},
		{
			name:   "already-absolute target ignores base",
			base:   "/notes/index.md",
			target: "/elsewhere/file.md",
			want:   "/elsewhere/file.md",
		},
		{
			name:   "base with no directory component",
			base:   "index.md",
			target: "page.md",
			want:   filepath.Join(cwd, "page.md"),
		},
		{
			name:   "empty target resolves to base dir",
			base:   "/notes/index.md",
			target: "",
			want:   "/notes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveRelativeTo(tt.base, tt.target)
			if err != nil {
				t.Fatalf("ResolveRelativeTo(%q, %q) error = %v", tt.base, tt.target, err)
			}
			if got != tt.want {
				t.Errorf("ResolveRelativeTo(%q, %q) = %q, want %q", tt.base, tt.target, got, tt.want)
			}
		})
	}
}

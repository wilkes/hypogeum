package embed

import "testing"

func TestLanguageFromPath(t *testing.T) {
	cases := map[string]string{
		"main.go":        "go",
		"app.py":         "python",
		"index.ts":       "typescript",
		"index.tsx":      "tsx",
		"foo.js":         "javascript",
		"a.rs":           "rust",
		"a.rb":           "ruby",
		"a.sh":           "bash",
		"Makefile":       "makefile",
		"Dockerfile":     "dockerfile",
		"config.yaml":    "yaml",
		"config.yml":     "yaml",
		"data.json":      "json",
		"styles.css":     "css",
		"page.html":      "html",
		"q.sql":          "sql",
		"notes.md":       "markdown",
		"unknown.xyzqux": "",
		"":               "",
	}
	for path, want := range cases {
		if got := LanguageFromPath(path); got != want {
			t.Errorf("LanguageFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}

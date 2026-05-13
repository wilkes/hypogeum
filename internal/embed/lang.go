package embed

import (
	"path/filepath"
	"strings"
)

var extLang = map[string]string{
	".go":    "go",
	".py":    "python",
	".ts":    "typescript",
	".tsx":   "tsx",
	".js":    "javascript",
	".jsx":   "jsx",
	".rs":    "rust",
	".rb":    "ruby",
	".sh":    "bash",
	".bash":  "bash",
	".zsh":   "bash",
	".yaml":  "yaml",
	".yml":   "yaml",
	".json":  "json",
	".toml":  "toml",
	".css":   "css",
	".scss":  "scss",
	".html":  "html",
	".htm":   "html",
	".sql":   "sql",
	".md":    "markdown",
	".c":     "c",
	".h":     "c",
	".cpp":   "cpp",
	".cc":    "cpp",
	".hpp":   "cpp",
	".java":  "java",
	".kt":    "kotlin",
	".swift": "swift",
	".clj":   "clojure",
	".cljs":  "clojure",
	".ex":    "elixir",
	".exs":   "elixir",
	".lua":   "lua",
	".php":   "php",
}

var nameLang = map[string]string{
	"Makefile":   "makefile",
	"makefile":   "makefile",
	"Dockerfile": "dockerfile",
	"dockerfile": "dockerfile",
}

// LanguageFromPath returns the markdown fence-language tag for path's
// basename. Returns "" for unrecognized extensions; callers should
// render an untagged fence in that case.
func LanguageFromPath(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	if lang, ok := nameLang[base]; ok {
		return lang
	}
	ext := strings.ToLower(filepath.Ext(base))
	return extLang[ext]
}

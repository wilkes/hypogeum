package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	got := Default()
	if got.Dialect != "pager" {
		t.Errorf("Default().Dialect = %q, want %q", got.Dialect, "pager")
	}
}

func TestDefaultPath(t *testing.T) {
	p, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if p == "" {
		t.Fatal("DefaultPath returned empty string with nil error")
	}
	if !strings.HasSuffix(p, "config.toml") {
		t.Errorf("DefaultPath = %q, want suffix %q", p, "hypogeum/config.toml")
	}
	if !strings.Contains(p, "hypogeum") {
		t.Errorf("DefaultPath = %q, want to contain %q", p, "hypogeum")
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_Missing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "does-not-exist.toml")
	cfg, warnings, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if cfg != (Default()) {
		t.Errorf("cfg = %+v, want %+v", cfg, Default())
	}
}

func TestLoad_HappyPath(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantDialect string
	}{
		{"empty file uses default", "", "pager"},
		{"explicit pager", `dialect = "pager"` + "\n", "pager"},
		{"explicit modern", `dialect = "modern"` + "\n", "modern"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeConfig(t, tc.body)
			cfg, warnings, err := Load(p)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if len(warnings) != 0 {
				t.Errorf("warnings = %v, want none", warnings)
			}
			if cfg.Dialect != tc.wantDialect {
				t.Errorf("cfg.Dialect = %q, want %q", cfg.Dialect, tc.wantDialect)
			}
		})
	}
}

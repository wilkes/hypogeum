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

func TestLoad_DefaultDialect(t *testing.T) {
	p := writeConfig(t, "")
	cfg, warnings, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if cfg.Dialect != "pager" {
		t.Errorf("cfg.Dialect = %q, want %q", cfg.Dialect, "pager")
	}
}

func TestLoad_ValidPager(t *testing.T) {
	p := writeConfig(t, `dialect = "pager"`+"\n")
	cfg, warnings, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if cfg.Dialect != "pager" {
		t.Errorf("cfg.Dialect = %q, want %q", cfg.Dialect, "pager")
	}
}

func TestLoad_ValidModern(t *testing.T) {
	p := writeConfig(t, `dialect = "modern"`+"\n")
	cfg, warnings, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if cfg.Dialect != "modern" {
		t.Errorf("cfg.Dialect = %q, want %q", cfg.Dialect, "modern")
	}
}

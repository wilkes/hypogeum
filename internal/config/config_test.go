package config

import (
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

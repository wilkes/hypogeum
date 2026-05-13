package code

import (
	"strings"
	"testing"
)

func TestRender_GoSource_ContainsANSI(t *testing.T) {
	r := NewRenderer(80)
	out, err := r.Render("main.go", []byte("package main\n\nfunc main() {}\n"))
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if out == "" {
		t.Fatal("Render returned empty output")
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escape sequences in output, got:\n%q", out)
	}
}

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

func TestRender_BinaryBlob_ReturnsBinaryMessage(t *testing.T) {
	r := NewRenderer(80)
	src := []byte{'M', 'Z', 0x00, 0x00, 0xff, 0xff}
	out, err := r.Render("a.exe", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "binary file") {
		t.Errorf("expected binary-file message, got: %q", out)
	}
}

func TestRender_OversizedFile_ReturnsTooLargeMessage(t *testing.T) {
	r := NewRenderer(80)
	src := make([]byte, 6*1024*1024) // 6 MB, all zero bytes — but size check runs first
	out, err := r.Render("huge.txt", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "too large") {
		t.Errorf("expected too-large message, got: %q", out)
	}
}

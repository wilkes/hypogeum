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

func TestRender_GoSource_PrefixesGutter(t *testing.T) {
	r := NewRenderer(80)
	src := []byte("package main\n\nfunc main() {}\n")
	out, err := r.Render("main.go", src)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// Three source lines must produce exactly three gutter rows — no
	// phantom trailing row from Chroma's terminal256 trailing SGR reset.
	if len(lines) != 3 {
		t.Fatalf("expected exactly 3 output lines, got %d:\n%q", len(lines), out)
	}
	if !strings.Contains(stripANSI(lines[0]), "1") {
		t.Errorf("first line gutter missing '1': %q", lines[0])
	}
	if !strings.Contains(stripANSI(lines[2]), "3") {
		t.Errorf("third line gutter missing '3': %q", lines[2])
	}
}

// TestRender_NoTrailingNewline_StillCountsCorrectly covers the
// asymmetric case: source without a trailing '\n' still produces one
// gutter row per source line.
func TestRender_NoTrailingNewline_StillCountsCorrectly(t *testing.T) {
	r := NewRenderer(80)
	src := []byte("package main\n\nfunc main() {}") // no trailing newline
	out, err := r.Render("main.go", src)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%q", len(lines), out)
	}
	if !strings.Contains(stripANSI(lines[2]), "3") {
		t.Errorf("third line gutter missing '3': %q", lines[2])
	}
}

// stripANSI is a test-only helper that removes ANSI escape sequences so
// assertions can check the user-visible text without coupling to color
// codes.
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			// Skip to next 'm' or end of string.
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

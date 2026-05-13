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

// TestRender_LongComment_KeepsColorOnContinuationRows guards against an
// ansi.Wrap quirk: it won't split mid-escape but doesn't synthesize a
// state restore at the seam. Without explicit carry, a long colored
// token like a comment renders its continuation rows in terminal
// default. We re-inject the active SGR at the start of each
// continuation row so the body color stays consistent.
func TestRender_LongComment_KeepsColorOnContinuationRows(t *testing.T) {
	r := NewRenderer(40)
	src := []byte("// " + strings.Repeat("aaaaaaaaaa ", 10) + "\n")
	out, err := r.Render("a.go", src)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrap to produce multiple rows, got %d:\n%q", len(lines), out)
	}
	// First row should contain Chroma's comment color (38;5;242 in monokai).
	if !strings.Contains(lines[0], "\x1b[38;5;242m") {
		t.Fatalf("first row missing expected comment color SGR: %q", lines[0])
	}
	// Each continuation row should re-establish a color SGR before the
	// body text. Without the fix, lines[1+] would have no SGR at all
	// after the blank gutter.
	for i := 1; i < len(lines); i++ {
		// Strip the blank-gutter leading spaces; what follows must be an SGR.
		body := strings.TrimLeft(lines[i], " ")
		if !strings.HasPrefix(body, "\x1b[") {
			t.Errorf("continuation row %d missing SGR after gutter: %q", i, lines[i])
		}
	}
}

func TestRender_LongLine_WrapsWithBlankContinuationGutter(t *testing.T) {
	r := NewRenderer(40) // narrow terminal
	longLine := strings.Repeat("a", 100)
	src := []byte(longLine + "\n")
	out, err := r.Render("note.txt", src)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrap to produce >= 2 output rows, got %d:\n%q", len(lines), out)
	}

	// Continuation row(s) must NOT start with an SGR escape — that
	// would mean a color is leaking into the gutter column.
	for i := 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "\x1b[") {
			t.Errorf("continuation row %d starts with SGR escape (color leak into gutter): %q", i, lines[i])
		}
		stripped := stripANSI(lines[i])
		if len(stripped) > 0 && stripped[0] != ' ' {
			t.Errorf("continuation row %d has non-blank gutter: %q", i, stripped)
		}
	}
}

func TestRender_Dockerfile_HighlightedByFilename(t *testing.T) {
	r := NewRenderer(80)
	src := []byte("FROM alpine:3.18\nRUN apk add --no-cache git\n")
	out, err := r.Render("Dockerfile", src)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("Dockerfile should be highlighted by filename glob; got plain text:\n%q", out)
	}
}

func TestRender_UnknownExtension_FallsBackToPlainTextWithGutter(t *testing.T) {
	r := NewRenderer(80)
	src := []byte("hello\nworld\n")
	out, err := r.Render("note.xyz", src)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if out == "" {
		t.Fatal("Render returned empty output")
	}
	if !strings.Contains(stripANSI(out), "1") || !strings.Contains(stripANSI(out), "2") {
		t.Errorf("expected line numbers 1 and 2 in gutter, got:\n%q", stripANSI(out))
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

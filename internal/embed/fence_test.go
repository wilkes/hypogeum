package embed

import (
	"strings"
	"testing"
)

func TestRenderToFence_RangeWithGutter(t *testing.T) {
	lines := []string{"func parse(s string) Tree {", "    // build AST", "}"}
	got := RenderToFence("main.go", lines, 42, "42–44", 0, 0, "")

	if !strings.HasPrefix(got, "> `main.go:42–44`\n") {
		t.Fatalf("missing provenance header:\n%s", got)
	}
	if !strings.Contains(got, "```go\n") {
		t.Fatalf("missing language fence:\n%s", got)
	}
	if !strings.Contains(got, " 42 │ func parse(s string) Tree {\n") {
		t.Fatalf("missing first gutter line:\n%s", got)
	}
	if !strings.Contains(got, " 44 │ }\n") {
		t.Fatalf("missing last gutter line:\n%s", got)
	}
	if !strings.HasSuffix(got, "```\n") {
		t.Fatalf("missing closing fence:\n%s", got)
	}
}

func TestRenderToFence_ContextLinesMarkedFaint(t *testing.T) {
	lines := []string{"before", "primary1", "primary2", "after"}
	// Range is [primary1, primary2] = absolute lines 11-12;
	// startLine 10 means line 10 is "before" (context), line 13 is "after" (context).
	got := RenderToFence("main.go", lines, 10, "11–12", 1, 1, "")

	if !strings.Contains(got, "  ~ │ before\n") {
		t.Fatalf("context line should use ~ gutter, got:\n%s", got)
	}
	if !strings.Contains(got, " 11 │ primary1\n") {
		t.Fatalf("primary line 11 missing or wrong gutter:\n%s", got)
	}
	if !strings.Contains(got, " 12 │ primary2\n") {
		t.Fatalf("primary line 12 missing or wrong gutter:\n%s", got)
	}
	if !strings.Contains(got, "  ~ │ after\n") {
		t.Fatalf("trailing context line should use ~ gutter, got:\n%s", got)
	}
}

func TestRenderToFence_UnknownExtensionUntagged(t *testing.T) {
	got := RenderToFence("notes.zzz", []string{"hello"}, 1, "1", 0, 0, "")
	if !strings.Contains(got, "```\n  1 │ hello\n") {
		t.Fatalf("unknown extension should produce untagged fence:\n%s", got)
	}
}

func TestRenderToFence_WithSoftWarning(t *testing.T) {
	got := RenderToFence("main.go", []string{"x"}, 10, "10–20", 0, 0, "file ends at line 10")
	if !strings.Contains(got, "> `main.go:10–20 (file ends at line 10)`") {
		t.Fatalf("soft warning not in header:\n%s", got)
	}
}

func TestRenderToFence_GutterWidthFromMaxLineNumber(t *testing.T) {
	// 9 lines means single-digit gutter, but if startLine = 95, max = 103 → 3-wide.
	lines := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}
	got := RenderToFence("x.go", lines, 95, "95–103", 0, 0, "")
	if !strings.Contains(got, " 95 │ a\n") || !strings.Contains(got, "103 │ i\n") {
		t.Fatalf("gutter width should accommodate widest number:\n%s", got)
	}
}

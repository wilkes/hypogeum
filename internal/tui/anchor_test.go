package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFollowLink_HeadingAnchor_ScrollsToHeading(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	// Long preamble so scrolling to the deep heading produces a non-zero
	// viewport offset on a 30-row test viewport.
	var b strings.Builder
	b.WriteString("# Intro\n\n")
	for i := 0; i < 80; i++ {
		b.WriteString("filler line\n\n")
	}
	b.WriteString("\n## Deep Dive\n\nmore\n")
	src := b.String()
	if err := os.WriteFile(target, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.md")
	if err := os.WriteFile(source, []byte("[link](target.md#deep-dive)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestModelAtSize(t, dir, source, 100, 30)
	m.cycleLink(1)
	m.followCurrentLink()

	if got := m.history.Current(); got != target {
		t.Fatalf("history.Current() = %s, want %s", got, target)
	}
	if m.content.viewport.YOffset == 0 {
		t.Errorf("expected viewport scrolled to anchor; YOffset = 0 (TotalLineCount=%d)", m.content.viewport.TotalLineCount())
	}
}

func TestFollowLink_BlockAnchor_ScrollsToBlock(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	var b strings.Builder
	b.WriteString("# Intro\n\n")
	for i := 0; i < 80; i++ {
		b.WriteString("filler line\n\n")
	}
	b.WriteString("\nimportant fact ^key\n\ntail\n")
	if err := os.WriteFile(target, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.md")
	if err := os.WriteFile(source, []byte("[[target#^key]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestModelAtSize(t, dir, source, 100, 30)
	m.cycleLink(1)
	m.followCurrentLink()

	if got := m.history.Current(); got != target {
		t.Fatalf("history.Current() = %s, want %s", got, target)
	}
	if m.content.viewport.YOffset == 0 {
		t.Errorf("expected viewport scrolled to block; YOffset = 0 (TotalLineCount=%d)", m.content.viewport.TotalLineCount())
	}
}

func TestBrokenAnchor_IncrementsBrokenCount(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	if err := os.WriteFile(target, []byte("# Real\n\njust text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.md")
	if err := os.WriteFile(source, []byte("See [[target^missing]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestModelAtSize(t, dir, source, 100, 30)
	if got := m.content.brokenCount; got != 1 {
		t.Errorf("brokenCount = %d, want 1 (file exists, anchor doesn't)", got)
	}
}

func TestSplitAnchor(t *testing.T) {
	tests := []struct {
		in            string
		wantHeading   string
		wantBlock     string
	}{
		{"deep-dive", "deep-dive", ""},
		{"^key", "", "key"},
		{"", "", ""},
	}
	for _, tt := range tests {
		h, b := splitAnchor(tt.in)
		if h != tt.wantHeading || b != tt.wantBlock {
			t.Errorf("splitAnchor(%q) = (%q, %q), want (%q, %q)", tt.in, h, b, tt.wantHeading, tt.wantBlock)
		}
	}
}

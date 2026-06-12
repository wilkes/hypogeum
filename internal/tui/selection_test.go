package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func stripANSItest(s string) string { return ansi.Strip(s) }

func TestModel_CopyToClipboard_DefaultIsSet(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	if m.copyToClipboard == nil {
		t.Fatal("copyToClipboard should default to a non-nil writer")
	}
}

func TestModel_RenderedBaseIsStored(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, filepath.Join(root, "index.md"))
	if m.content.rendered == "" {
		t.Fatal("content.rendered should hold the rendered output after open")
	}
	if !strings.Contains(stripANSItest(m.content.rendered), "Index") {
		t.Errorf("rendered base should contain the heading text; got %q",
			stripANSItest(m.content.rendered))
	}
}

package tui

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestModel_CopyPath_CopiesCurrentAbsolutePath(t *testing.T) {
	root := writeFixture(t)
	want := filepath.Join(root, "index.md")
	m := sized(t, root, want)

	var copied string
	m.copyToClipboard = func(s string) { copied = s }

	m = pressRune(t, m, 'y')

	if copied != want {
		t.Errorf("copyToClipboard got %q, want %q", copied, want)
	}
	if !strings.Contains(m.renderFooter(), "Copied path") {
		t.Errorf("footer should show a copy-path toast; got %q", m.renderFooter())
	}
}

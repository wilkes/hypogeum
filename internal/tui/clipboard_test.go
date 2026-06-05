package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wilkes/hypogeum/internal/nav"
)

// fakeClipboard records the text handed to it and returns the configured
// error. Replaces copyToClipboard in tests so we don't shell out.
type fakeClipboard struct {
	calls []string
	err   error
}

func (f *fakeClipboard) write(text string) error {
	f.calls = append(f.calls, text)
	return f.err
}

// withFakeClipboard installs a fakeClipboard on m and returns it.
func withFakeClipboard(m *Model) *fakeClipboard {
	f := &fakeClipboard{}
	m.copyClipboard = f.write
	return f
}

func TestCopyPath_CopiesAbsolutePathOfOpenDoc(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	fake := withFakeClipboard(&m)
	m = switchToContent(t, m)

	want := m.history.Current()
	if want == "" {
		t.Fatalf("setup: expected a document open after New")
	}

	m = pressRune(t, m, 'y')

	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 clipboard write, got %d", len(fake.calls))
	}
	if !filepath.IsAbs(fake.calls[0]) {
		t.Errorf("clipboard text %q is not an absolute path", fake.calls[0])
	}
	if fake.calls[0] != want {
		t.Errorf("clipboard got %q, want current path %q", fake.calls[0], want)
	}
	if m.diag != nil {
		if e, ok := m.diag.transientStatus(); !ok || !strings.Contains(e.Message, "copied path") {
			t.Errorf("expected a 'copied path' transient, got %+v (ok=%v)", e, ok)
		}
	}
}

func TestCopyPath_ModernAltC(t *testing.T) {
	root := writeFixture(t)
	m := sizedWithOptions(t, root, "", Options{Dialect: "modern"})
	fake := withFakeClipboard(&m)
	m = switchToContent(t, m)

	// alt+c is the modern-dialect copy-path chord.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}, Alt: true})

	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 clipboard write via alt+c, got %d", len(fake.calls))
	}
	if fake.calls[0] != m.history.Current() {
		t.Errorf("clipboard got %q, want %q", fake.calls[0], m.history.Current())
	}
}

func TestCopyPath_WriterErrorSurfacesInDiag(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	fake := withFakeClipboard(&m)
	fake.err = errors.New("no clipboard tool found")
	m = switchToContent(t, m)

	m = pressRune(t, m, 'y')

	if len(fake.calls) != 1 {
		t.Fatalf("expected the writer to be attempted once, got %d", len(fake.calls))
	}
	if m.diag != nil {
		e, ok := m.diag.transientStatus()
		if !ok || !strings.Contains(e.Message, "copy path failed") {
			t.Errorf("expected a 'copy path failed' transient, got %+v (ok=%v)", e, ok)
		}
		if !strings.Contains(e.Message, "no clipboard tool found") {
			t.Errorf("expected wrapped error in transient, got %q", e.Message)
		}
	}
}

func TestCopyCurrentPath_NoDocumentIsNoOp(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	fake := withFakeClipboard(&m)

	// Force the "no document open" branch by clearing history. We exercise
	// the method directly because the model always auto-opens a top-level
	// file, so there's no key-driven path to an empty history.
	m.history = nav.New()
	m.copyCurrentPath()

	if len(fake.calls) != 0 {
		t.Errorf("expected no clipboard write with no document open, got %d", len(fake.calls))
	}
}

package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wilkes/hypogeum/internal/markdown"
)

// fakeOpener records calls and returns the configured error. Replaces
// openExternalURL in tests so we don't actually launch a browser.
type fakeOpener struct {
	calls []string
	err   error
}

func (f *fakeOpener) open(rawURL string) error {
	f.calls = append(f.calls, rawURL)
	return f.err
}

// withFakeOpener installs a fakeOpener on m and returns it for assertion.
func withFakeOpener(m *Model) *fakeOpener {
	f := &fakeOpener{}
	m.openExternal = f.open
	return f
}

// cycleToExternalLink walks the link cursor forward until the selected
// link is external, failing the test if none is found.
func cycleToExternalLink(t *testing.T, m Model) Model {
	t.Helper()
	m = switchToContent(t, m)
	for i := 0; i < len(m.content.links); i++ {
		m = pressRune(t, m, 'n')
		if m.content.links[m.content.linkCursor].Resolved.Kind == markdown.LinkExternal {
			return m
		}
	}
	t.Fatalf("no external link in fixture")
	return m
}

func TestExternal_EnterArmsConfirmPrompt(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	m = cycleToExternalLink(t, m)

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.pendingExternal == "" {
		t.Fatalf("expected pendingExternal set after Enter on external link")
	}
	if !strings.Contains(m.status, "press Enter again") {
		t.Errorf("expected status to show confirm prompt, got %q", m.status)
	}
}

func TestExternal_SecondEnterExecsOpener(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	fake := withFakeOpener(&m)
	m = cycleToExternalLink(t, m)

	// First Enter arms.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.pendingExternal == "" {
		t.Fatalf("setup: pendingExternal should be set after first Enter")
	}
	armed := m.pendingExternal

	// Second Enter exec's.
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 opener call, got %d", len(fake.calls))
	}
	if fake.calls[0] != armed {
		t.Errorf("opener called with %q, want %q", fake.calls[0], armed)
	}
	if m.pendingExternal != "" {
		t.Errorf("pendingExternal should clear after confirm, got %q", m.pendingExternal)
	}
	if !strings.Contains(m.status, "opened:") {
		t.Errorf("status after confirm = %q, want to mention 'opened:'", m.status)
	}
}

func TestExternal_OpenerErrorSurfacesInStatus(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	fake := withFakeOpener(&m)
	fake.err = errors.New("xdg-open not found")
	m = cycleToExternalLink(t, m)

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // arm
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // confirm

	if !strings.Contains(m.status, "open failed") {
		t.Errorf("expected 'open failed' in status, got %q", m.status)
	}
	if !strings.Contains(m.status, "xdg-open not found") {
		t.Errorf("expected wrapped error message in status, got %q", m.status)
	}
}

func TestExternal_NonEnterKeyCancelsAndFallsThrough(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	fake := withFakeOpener(&m)
	m = cycleToExternalLink(t, m)
	armedCursor := m.content.linkCursor

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // arm
	if m.pendingExternal == "" {
		t.Fatalf("setup: pendingExternal should be set")
	}

	// Press `n` — should cancel the prompt AND advance the link cursor.
	m = pressRune(t, m, 'n')

	if m.pendingExternal != "" {
		t.Errorf("pendingExternal should clear on non-Enter, got %q", m.pendingExternal)
	}
	if len(fake.calls) != 0 {
		t.Errorf("opener must not be called on cancel, got %d calls", len(fake.calls))
	}
	if m.content.linkCursor == armedCursor {
		t.Errorf("cancellation should let 'n' fall through and advance the cursor")
	}
}

func TestExternal_EscCancelsWithoutExec(t *testing.T) {
	root := writeFixture(t)
	m := sized(t, root, "")
	fake := withFakeOpener(&m)
	m = cycleToExternalLink(t, m)

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // arm
	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})   // cancel via Esc

	if m.pendingExternal != "" {
		t.Errorf("pendingExternal should clear on Esc, got %q", m.pendingExternal)
	}
	if len(fake.calls) != 0 {
		t.Errorf("opener must not be called on Esc, got %d calls", len(fake.calls))
	}
}

func TestOpenExternalURL_RejectsNonHTTPSchemes(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"javascript", "javascript:alert(1)"},
		{"data", "data:text/html,<script>alert(1)</script>"},
		{"file", "file:///etc/passwd"},
		{"mailto", "mailto:user@example.com"},
		{"ftp", "ftp://example.com/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := openExternalURL(tc.url)
			if err == nil {
				t.Fatalf("expected error for %q scheme, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), "scheme") {
				t.Errorf("expected scheme-rejection error, got %v", err)
			}
		})
	}
}

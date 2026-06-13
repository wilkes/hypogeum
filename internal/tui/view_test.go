package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRenderFooter_AppendsBrokenSuffixWhenNonZero(t *testing.T) {
	isolatedHome(t)
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	m.content.brokenCount = 3
	rendered := m.renderFooter()
	if !strings.Contains(rendered, "3 broken") {
		t.Fatalf("expected footer to contain %q, got: %q", "3 broken", rendered)
	}
	if !strings.Contains(rendered, "⚠") {
		t.Fatalf("expected footer to contain warning sigil ⚠, got: %q", rendered)
	}
}

func TestRenderFooter_NoSuffixWhenZero(t *testing.T) {
	isolatedHome(t)
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	m.content.brokenCount = 0
	rendered := m.renderFooter()
	if strings.Contains(rendered, "broken") {
		t.Fatalf("expected no broken suffix when count is zero, got: %q", rendered)
	}
}

func TestRenderFooter_SuffixSuppressedDuringTransient(t *testing.T) {
	isolatedHome(t)
	dir := t.TempDir()
	m, _ := New(dir, "")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = mm.(Model)

	m.content.brokenCount = 3
	m.diag.Warn("transient warning here")

	rendered := m.renderFooter()
	if strings.Contains(rendered, "3 broken") {
		t.Fatalf("expected broken suffix suppressed during transient, got: %q", rendered)
	}
}

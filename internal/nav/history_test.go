package nav

import "testing"

func TestHistory_Empty(t *testing.T) {
	h := New()
	if h.Current() != "" {
		t.Errorf("Current on empty = %q, want \"\"", h.Current())
	}
	if h.CanBack() || h.CanForward() {
		t.Errorf("empty history should not allow back or forward")
	}
	if _, ok := h.Back(); ok {
		t.Errorf("Back on empty should return false")
	}
}

func TestHistory_VisitAndBack(t *testing.T) {
	h := New()
	h.Visit("a.md")
	h.Visit("b.md")
	h.Visit("c.md")

	if got := h.Current(); got != "c.md" {
		t.Errorf("Current = %q, want c.md", got)
	}

	got, ok := h.Back()
	if !ok || got != "b.md" {
		t.Errorf("Back = %q, %v; want b.md, true", got, ok)
	}

	got, ok = h.Back()
	if !ok || got != "a.md" {
		t.Errorf("Back = %q, %v; want a.md, true", got, ok)
	}

	if _, ok := h.Back(); ok {
		t.Errorf("Back past beginning should fail")
	}
}

func TestHistory_ForwardAfterBack(t *testing.T) {
	h := New()
	h.Visit("a.md")
	h.Visit("b.md")
	h.Back()

	got, ok := h.Forward()
	if !ok || got != "b.md" {
		t.Errorf("Forward = %q, %v; want b.md, true", got, ok)
	}
}

func TestHistory_VisitTruncatesForward(t *testing.T) {
	h := New()
	h.Visit("a.md")
	h.Visit("b.md")
	h.Visit("c.md")
	h.Back() // now at b
	h.Back() // now at a
	h.Visit("d.md")

	if h.CanForward() {
		t.Errorf("forward history should be truncated after a new visit")
	}
	if got := h.Current(); got != "d.md" {
		t.Errorf("Current = %q, want d.md", got)
	}
}

func TestHistory_VisitSameNoOp(t *testing.T) {
	h := New()
	h.Visit("a.md")
	h.Visit("a.md")
	h.Visit("a.md")

	if h.CanBack() {
		t.Errorf("repeated visits to the same path should not create history")
	}
}

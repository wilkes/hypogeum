package markdown

import (
	"strings"
	"testing"
)

func TestStripSentinels_NoSentinels(t *testing.T) {
	in := "hello world\nno markers here\n"
	cleaned, spans := stripSentinels(in, nil)
	if cleaned != in {
		t.Errorf("cleaned: got %q want %q", cleaned, in)
	}
	if len(spans) != 0 {
		t.Errorf("expected zero spans, got %d", len(spans))
	}
}

func TestStripSentinels_SingleLineLink(t *testing.T) {
	in := "hello \x1cworld\x1e end"
	cleaned, spans := stripSentinels(in, nil)
	wantClean := "hello world end"
	if cleaned != wantClean {
		t.Errorf("cleaned: got %q want %q", cleaned, wantClean)
	}
	if len(spans) != 1 {
		t.Fatalf("spans: got %d want 1", len(spans))
	}
	if spans[0].row != 0 {
		t.Errorf("row: got %d want 0", spans[0].row)
	}
	if spans[0].text != "world" {
		t.Errorf("text: got %q want world", spans[0].text)
	}
}

func TestStripSentinels_LinkWrappingTwoLines(t *testing.T) {
	in := "a\x1cone\ntwo\x1e b"
	cleaned, spans := stripSentinels(in, nil)
	wantClean := "aone\ntwo b"
	if cleaned != wantClean {
		t.Errorf("cleaned: got %q want %q", cleaned, wantClean)
	}
	if len(spans) != 1 {
		t.Fatalf("spans: got %d want 1", len(spans))
	}
	if spans[0].row != 0 {
		t.Errorf("row: got %d want 0", spans[0].row)
	}
	if !strings.Contains(spans[0].text, "\n") {
		t.Errorf("expected span text to contain newline, got %q", spans[0].text)
	}
	if spans[0].text != "one\ntwo" {
		t.Errorf("text: got %q want %q", spans[0].text, "one\ntwo")
	}
}

func TestStripSentinels_MultipleLinksOneLine(t *testing.T) {
	in := "\x1cfoo\x1e and \x1cbar\x1e"
	cleaned, spans := stripSentinels(in, nil)
	wantClean := "foo and bar"
	if cleaned != wantClean {
		t.Errorf("cleaned: got %q want %q", cleaned, wantClean)
	}
	if len(spans) != 2 {
		t.Fatalf("spans: got %d want 2", len(spans))
	}
	for i, s := range spans {
		if s.row != 0 {
			t.Errorf("spans[%d].row: got %d want 0", i, s.row)
		}
	}
	if spans[0].text != "foo" || spans[1].text != "bar" {
		t.Errorf("texts: got %q,%q want foo,bar", spans[0].text, spans[1].text)
	}
}

func TestStripSentinels_LinkInsideSGR(t *testing.T) {
	in := "\x1b[1m\x1cbold\x1e\x1b[0m"
	cleaned, spans := stripSentinels(in, nil)
	// SGR escapes survive intact, sentinels disappear.
	wantClean := "\x1b[1mbold\x1b[0m"
	if cleaned != wantClean {
		t.Errorf("cleaned: got %q want %q", cleaned, wantClean)
	}
	if strings.ContainsRune(cleaned, sentinelStart) || strings.ContainsRune(cleaned, sentinelEnd) {
		t.Errorf("sentinels leaked: %q", cleaned)
	}
	if len(spans) != 1 {
		t.Fatalf("spans: got %d want 1", len(spans))
	}
	if spans[0].text != "bold" {
		t.Errorf("text: got %q want bold (SGR should not pollute span text)", spans[0].text)
	}
}

func TestStripSentinels_MarkerBracketsLink(t *testing.T) {
	marker := func(i int) (string, string) {
		return "<<O" + string(rune('0'+i)) + ">>", "<<C" + string(rune('0'+i)) + ">>"
	}
	in := "\x1cone\x1e and \x1ctwo\x1e"
	cleaned, spans := stripSentinels(in, marker)
	if !strings.Contains(cleaned, "<<O0>>one<<C0>>") {
		t.Errorf("expected first marker pair in cleaned output: %q", cleaned)
	}
	if !strings.Contains(cleaned, "<<O1>>two<<C1>>") {
		t.Errorf("expected second marker pair in cleaned output: %q", cleaned)
	}
	if strings.ContainsRune(cleaned, sentinelStart) || strings.ContainsRune(cleaned, sentinelEnd) {
		t.Errorf("sentinel runes leaked despite marker: %q", cleaned)
	}
	if len(spans) != 2 {
		t.Fatalf("spans: got %d want 2", len(spans))
	}
}

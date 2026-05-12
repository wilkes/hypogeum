package recent

import (
	"testing"
	"time"
)

func TestRankedZeroValue(t *testing.T) {
	r := Ranked{}
	if r.Path != "" {
		t.Errorf("zero Ranked.Path: got %q want \"\"", r.Path)
	}
	if r.Score != 0 {
		t.Errorf("zero Ranked.Score: got %v want 0", r.Score)
	}
	if !r.MTime.IsZero() {
		t.Errorf("zero Ranked.MTime: got %v want zero", r.MTime)
	}
	if !r.Visit.IsZero() {
		t.Errorf("zero Ranked.Visit: got %v want zero", r.Visit)
	}
}

func TestConstants(t *testing.T) {
	// Sanity check that the published constants are positive and finite.
	if mtimeHalfLifeHours <= 0 {
		t.Errorf("mtimeHalfLifeHours must be > 0, got %v", mtimeHalfLifeHours)
	}
	if visitHalfLifeHours <= 0 {
		t.Errorf("visitHalfLifeHours must be > 0, got %v", visitHalfLifeHours)
	}
	if visitWeight <= 0 {
		t.Errorf("visitWeight must be > 0, got %v", visitWeight)
	}
	_ = time.Now() // keeps the time import used in this file
}

package search

import (
	"reflect"
	"testing"
)

func TestRerankByRecency(t *testing.T) {
	hits := []Hit{
		{Path: "/a.md", Line: 1},
		{Path: "/b.md", Line: 1},
		{Path: "/a.md", Line: 5},
		{Path: "/c.md", Line: 1},
	}
	// Recency order puts b first, then a; c is omitted (never visited).
	order := func(paths []string) []string {
		return []string{"/b.md", "/a.md"}
	}

	got := RerankByRecency(order, hits)
	want := []Hit{
		{Path: "/b.md", Line: 1},
		{Path: "/a.md", Line: 1},
		{Path: "/a.md", Line: 5},
		{Path: "/c.md", Line: 1}, // omitted-from-order paths trail, input order
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RerankByRecency =\n%+v\nwant\n%+v", got, want)
	}
}

func TestRerankByRecencyNilOrder(t *testing.T) {
	hits := []Hit{{Path: "/a.md", Line: 1}}
	if got := RerankByRecency(nil, hits); !reflect.DeepEqual(got, hits) {
		t.Errorf("nil order changed hits: %+v", got)
	}
}

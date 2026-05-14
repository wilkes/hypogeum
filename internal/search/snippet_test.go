package search

import "testing"

func TestBuildSnippet(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		matchAt  int // byte offset of match in line
		matchLen int
		budget   int // total visible chars budget
		want     string
	}{
		{
			name:     "short line fits whole",
			line:     "the quick brown fox",
			matchAt:  4,
			matchLen: 5,
			budget:   60,
			want:     "the \x11quick\x12 brown fox",
		},
		{
			name:     "match at start, line too long",
			line:     "foo " + repeat("x", 200),
			matchAt:  0,
			matchLen: 3,
			budget:   30,
			want:     "\x11foo\x12 " + repeat("x", 24) + "…",
		},
		{
			name:     "match at end, line too long",
			line:     repeat("x", 200) + " end",
			matchAt:  201,
			matchLen: 3,
			budget:   30,
			want:     "…" + repeat("x", 24) + " \x11end\x12",
		},
		{
			name:     "match in middle, line too long, centered window",
			line:     repeat("a", 100) + "needle" + repeat("b", 100),
			matchAt:  100,
			matchLen: 6,
			budget:   30,
			want:     "…" + repeat("a", 10) + "\x11needle\x12" + repeat("b", 10) + "…",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildSnippet(c.line, c.matchAt, c.matchLen, c.budget)
			if got != c.want {
				t.Errorf("buildSnippet:\n got: %q\nwant: %q", got, c.want)
			}
		})
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

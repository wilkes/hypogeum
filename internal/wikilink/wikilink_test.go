package wikilink

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		name string
		body string
		want *Body
	}{
		{"empty", "", nil},
		{"only whitespace", "   ", nil},
		{"name only", "Foo", &Body{Name: "Foo"}},
		{"alias only", "Foo|the foo", &Body{Name: "Foo", Alias: "the foo"}},
		{"heading only", "Foo#Section Two", &Body{Name: "Foo", Heading: "Section Two"}},
		{"block only", "Foo^abc123", &Body{Name: "Foo", Block: "abc123"}},
		{"heading and block", "Foo#Section^abc123", &Body{Name: "Foo", Heading: "Section", Block: "abc123"}},
		{"heading and alias", "Foo#Section|that section", &Body{Name: "Foo", Heading: "Section", Alias: "that section"}},
		{"heading block and alias", "Foo#Section^abc123|that section", &Body{Name: "Foo", Heading: "Section", Block: "abc123", Alias: "that section"}},
		{"trims components", "  Foo  #  Section  ^  abc  |  alias  ", &Body{Name: "Foo", Heading: "Section", Block: "abc", Alias: "alias"}},
		{"alias without name", "|Alias", nil},
		{"heading without name", "#Heading", nil},
		{"block without name", "^block", nil},
		{"name with trailing block-then-heading order keeps name", "Foo#H^B", &Body{Name: "Foo", Heading: "H", Block: "B"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.body)
			if got == nil && tc.want == nil {
				return
			}
			if got == nil || tc.want == nil {
				t.Fatalf("Parse(%q) = %+v, want %+v", tc.body, got, tc.want)
			}
			if *got != *tc.want {
				t.Fatalf("Parse(%q) = %+v, want %+v", tc.body, got, tc.want)
			}
		})
	}
}

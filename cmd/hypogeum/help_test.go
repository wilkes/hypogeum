package main

import (
	"strings"
	"testing"
)

// TestHelpTextDocumentsEveryQueryVerb guards against the help drifting out of
// sync with the actual reserved verbs: if a verb is added to queryVerbs but not
// to helpText (or vice versa), this fails.
func TestHelpTextDocumentsEveryQueryVerb(t *testing.T) {
	help := helpText()
	for verb := range queryVerbs {
		if !strings.Contains(help, verb) {
			t.Errorf("helpText() does not mention query verb %q", verb)
		}
	}
}

func TestHelpTextMentionsGlobalFlags(t *testing.T) {
	help := helpText()
	for _, want := range []string{"Usage:", "--version", "--help", "-vault", "[path]"} {
		if !strings.Contains(help, want) {
			t.Errorf("helpText() missing %q", want)
		}
	}
}

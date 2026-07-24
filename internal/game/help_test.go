package game

import (
	"strings"
	"testing"
)

func TestHelpTopicsWellFormed(t *testing.T) {
	topics := helpTopics("!")
	if len(topics) == 0 {
		t.Fatal("no help topics defined")
	}
	seen := map[string]bool{}
	for _, tp := range topics {
		if tp.name == "" {
			t.Error("topic with empty name")
		}
		if seen[tp.name] {
			t.Errorf("duplicate topic %q", tp.name)
		}
		seen[tp.name] = true
		if tp.summary == "" {
			t.Errorf("topic %q has no summary", tp.name)
		}
		if len(tp.lines) == 0 {
			t.Errorf("topic %q has no body lines", tp.name)
		}
		for _, line := range tp.lines {
			if strings.TrimSpace(line) == "" {
				t.Errorf("topic %q has a blank line", tp.name)
			}
		}
	}
}

func TestHelpAliasesResolve(t *testing.T) {
	topics := helpTopics("!")
	valid := map[string]bool{}
	for _, tp := range topics {
		valid[tp.name] = true
	}
	for alias, target := range helpAlias {
		if !valid[target] {
			t.Errorf("alias %q points at unknown topic %q", alias, target)
		}
	}
}

func TestHelpPrefixFoldedIn(t *testing.T) {
	// non-default prefix appears
	for _, tp := range helpTopics("@") {
		joined := strings.Join(tp.lines, " ")
		if strings.Contains(joined, "!ohayou") || strings.Contains(joined, "!buy") {
			t.Errorf("topic %q leaked a hardcoded prefix: %q", tp.name, joined)
		}
	}
}

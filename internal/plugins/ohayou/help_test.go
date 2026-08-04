package ohayou

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
		if tp.Name == "" {
			t.Error("topic with empty name")
		}
		if seen[tp.Name] {
			t.Errorf("duplicate topic %q", tp.Name)
		}
		seen[tp.Name] = true
		if tp.Summary == "" {
			t.Errorf("topic %q has no summary", tp.Name)
		}
		if len(tp.Lines) == 0 {
			t.Errorf("topic %q has no body lines", tp.Name)
		}
		for _, line := range tp.Lines {
			if strings.TrimSpace(line) == "" {
				t.Errorf("topic %q has a blank line", tp.Name)
			}
		}
	}
}

// An alias that collides with a topic name would shadow it, and one repeated
// across topics would send a reader somewhere arbitrary.
func TestHelpAliasesAreDistinct(t *testing.T) {
	topics := helpTopics("!")

	names := map[string]bool{}
	for _, tp := range topics {
		names[tp.Name] = true
	}

	seen := map[string]string{}
	for _, tp := range topics {
		for _, alias := range tp.Aliases {
			if names[alias] {
				t.Errorf("topic %q has alias %q, which is another topic's name", tp.Name, alias)
			}
			if other, dup := seen[alias]; dup {
				t.Errorf("alias %q is claimed by both %q and %q", alias, other, tp.Name)
			}
			seen[alias] = tp.Name
		}
	}
}

// Every command the game registers should reach some help, or a reader who
// types !help <command> is told there is none.
func TestEveryGameCommandHasHelp(t *testing.T) {
	topics := helpTopics("!")

	covered := map[string]bool{}
	for _, tp := range topics {
		covered[tp.Name] = true
		for _, alias := range tp.Aliases {
			covered[alias] = true
		}
	}

	for _, cmd := range []string{
		"ohayou", "buy", "items", "item", "use", "equip", "unequip", "build",
		"recipe", "quarry", "inventory", "stats", "top", "steal", "report",
		"deposit", "withdraw", "register", "identify",
	} {
		if !covered[cmd] {
			t.Errorf("!%s reaches no help topic", cmd)
		}
	}
}

func TestHelpPrefixFoldedIn(t *testing.T) {
	for _, tp := range helpTopics("@") {
		joined := strings.Join(tp.Lines, " ")
		if strings.Contains(joined, "!ohayou") || strings.Contains(joined, "!buy") {
			t.Errorf("topic %q leaked a hardcoded prefix: %q", tp.Name, joined)
		}
	}
}

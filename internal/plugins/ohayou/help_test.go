package ohayou

import (
	"strings"
	"testing"
)

func TestHelpTopicsWellFormed(t *testing.T) {
	topics := helpTopics("!", "https://hemera.day/ohayou/help")
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
	topics := helpTopics("!", "https://hemera.day/ohayou/help")

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
	topics := helpTopics("!", "https://hemera.day/ohayou/help")

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

// The handbook is where a new player is sent, so !help has to carry it when
// there is a site, and must not invent one when there isn't.
func TestHelpCarriesTheHandbook(t *testing.T) {
	const url = "https://hemera.day/ohayou/help"

	sent := map[string]bool{}
	for _, tp := range helpTopics("!", url) {
		if strings.Contains(strings.Join(tp.Lines, " "), url) {
			sent[tp.Name] = true
		}
	}
	for _, name := range []string{"basics", "shop"} {
		if !sent[name] {
			t.Errorf("topic %q does not link the handbook", name)
		}
	}

	for _, tp := range helpTopics("!", "") {
		for _, line := range tp.Lines {
			if strings.Contains(line, "http") {
				t.Errorf("topic %q links a site the bot has not got: %q", tp.Name, line)
			}
		}
	}
}

func TestHelpPrefixFoldedIn(t *testing.T) {
	for _, tp := range helpTopics("@", "https://hemera.day/ohayou/help") {
		joined := strings.Join(tp.Lines, " ")
		if strings.Contains(joined, "!ohayou") || strings.Contains(joined, "!buy") {
			t.Errorf("topic %q leaked a hardcoded prefix: %q", tp.Name, joined)
		}
	}
}

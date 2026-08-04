package bot

import (
	"strings"
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/bot/irctext"
)

func helpBot() *Bot {
	b := testBot()
	b.Help(
		Topic{Name: "basics", Summary: "how it works", Aliases: []string{"ohayou"},
			Lines: []string{"first line", "second line"}},
		Topic{Name: "deer", Summary: "art in the channel", Aliases: []string{"deerme"},
			Lines: []string{"about deer"}},
	)
	return b
}

func TestTopicByName(t *testing.T) {
	b := helpBot()
	got, ok := b.topic("basics")
	if !ok || got.Name != "basics" {
		t.Errorf("topic(basics) = %+v, %v", got, ok)
	}
}

func TestTopicFoldsCaseAndSpace(t *testing.T) {
	b := helpBot()
	for _, want := range []string{"BASICS", "  basics  ", "Deer"} {
		if _, ok := b.topic(want); !ok {
			t.Errorf("topic(%q) not found", want)
		}
	}
}

func TestTopicByAlias(t *testing.T) {
	b := helpBot()
	got, ok := b.topic("deerme")
	if !ok || got.Name != "deer" {
		t.Errorf("topic(deerme) = %+v, %v, want the deer topic", got, ok)
	}
}

func TestTopicUnknown(t *testing.T) {
	if _, ok := helpBot().topic("nothing"); ok {
		t.Error("topic() found something that was never registered")
	}
}

// A plugin registering an alias that is another plugin's topic name must not
// shadow it: the real topic wins.
func TestTopicNameBeatsAnAlias(t *testing.T) {
	b := testBot()
	b.Help(
		Topic{Name: "deer", Lines: []string{"the real one"}},
		Topic{Name: "other", Aliases: []string{"deer"}, Lines: []string{"the impostor"}},
	)
	got, ok := b.topic("deer")
	if !ok || got.Lines[0] != "the real one" {
		t.Errorf("topic(deer) = %+v, want the topic named deer", got)
	}
}

func TestHelpIndexFitsTheLine(t *testing.T) {
	b := testBot()
	for _, name := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight"} {
		b.Help(Topic{Name: name, Summary: strings.Repeat("x", 60), Lines: []string{"."}})
	}

	lines := b.helpIndex("#chan")
	if len(lines) < 2 {
		t.Fatalf("%d lines, want the index split across several", len(lines))
	}
	budget := irctext.LineBudget("#chan")
	for _, line := range lines {
		if len(line) > budget {
			t.Errorf("line is %d bytes, over the %d budget: %q", len(line), budget, line)
		}
	}

	// Every topic still has to appear somewhere in the index.
	all := strings.Join(lines, " ")
	for _, name := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight"} {
		if !strings.Contains(all, name) {
			t.Errorf("topic %q is missing from the index", name)
		}
	}
}

func TestHelpIndexOnOneLineWhenItFits(t *testing.T) {
	b := helpBot()
	if lines := b.helpIndex("#chan"); len(lines) != 1 {
		t.Errorf("%d lines for two short topics, want 1: %q", len(lines), lines)
	}
}

func TestCommandsAreGeneratedFromTheTable(t *testing.T) {
	b := testBot()
	b.HandleFunc("zulu", false, func(m *Message) {})
	b.HandleFunc("alpha", false, func(m *Message) {})
	b.HandleFunc("secret", true, func(m *Message) {})

	names := b.commandNames(false)
	joined := strings.Join(names, " ")

	if !strings.Contains(joined, "!alpha") || !strings.Contains(joined, "!zulu") {
		t.Errorf("commands = %q, want the registered ones", joined)
	}
	if strings.Contains(joined, "!secret") {
		t.Errorf("commands = %q, want admin commands hidden from everyone else", joined)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("commands out of order at %d: %q", i, names)
		}
	}
}

func TestAdminsSeeAdminCommands(t *testing.T) {
	b := testBot()
	b.HandleFunc("secret", true, func(m *Message) {})

	if got := strings.Join(b.commandNames(true), " "); !strings.Contains(got, "!secret") {
		t.Errorf("commands = %q, want the admin ones listed to an admin", got)
	}
}

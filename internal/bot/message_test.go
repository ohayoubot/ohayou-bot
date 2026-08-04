package bot

import (
	"strings"
	"testing"
)

func msg(text, target, nick string) *Message {
	return &Message{Text: text, Args: strings.Fields(text), Target: target, Nick: nick}
}

func TestReplyTo(t *testing.T) {
	if got := msg("!ohayou", "#chan", "someone").ReplyTo(); got != "#chan" {
		t.Errorf("channel: got %q, want %q", got, "#chan")
	}
	if got := msg("!ohayou", "ohayoubot", "someone").ReplyTo(); got != "someone" {
		t.Errorf("private: got %q, want %q", got, "someone")
	}
}

func TestArg(t *testing.T) {
	m := msg("!buy acre 3", "#chan", "someone")
	for n, want := range map[int]string{0: "!buy", 1: "acre", 2: "3", 3: "", 9: "", -1: ""} {
		if got := m.Arg(n); got != want {
			t.Errorf("Arg(%d): got %q, want %q", n, got, want)
		}
	}
}

func TestRest(t *testing.T) {
	m := msg("!say  #chan   hello  there", "#chan", "someone")
	for n, want := range map[int]string{
		0: "!say #chan hello there",
		1: "#chan hello there",
		2: "hello there",
		3: "there",
		4: "",
		9: "",
	} {
		if got := m.Rest(n); got != want {
			t.Errorf("Rest(%d): got %q, want %q", n, got, want)
		}
	}
}

func TestArgAndRestOnEmptyMessage(t *testing.T) {
	m := &Message{}
	if got := m.Arg(0); got != "" {
		t.Errorf("Arg(0): got %q, want empty", got)
	}
	if got := m.Rest(0); got != "" {
		t.Errorf("Rest(0): got %q, want empty", got)
	}
}

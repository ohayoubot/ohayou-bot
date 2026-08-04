package bottest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/bot/bottest"
)

func TestCommandsReachTheBotAndTheReplyComesBack(t *testing.T) {
	h := bottest.New(t)
	h.Bot.HandleFunc("ping", false, func(m *bot.Message) {
		h.Bot.Say(m.ReplyTo(), "pong")
	})
	h.Start()

	h.Say("someone", "#chan", "!ping")
	if got := h.Next(); got != "PRIVMSG #chan :pong" {
		t.Errorf("got %q", got)
	}
}

func TestPrivateMessagesAnswerTheSender(t *testing.T) {
	h := bottest.New(t)
	h.Bot.HandleFunc("ping", false, func(m *bot.Message) {
		h.Bot.Say(m.ReplyTo(), "pong")
	})
	h.Start()

	h.Say("someone", "ohayoubot", "!ping")
	if got := h.Next(); got != "PRIVMSG someone :pong" {
		t.Errorf("got %q", got)
	}
}

func TestSilentPassesWhenNothingIsSaid(t *testing.T) {
	h := bottest.New(t)
	h.Start()

	h.Say("someone", "#chan", "just talking")
	h.Silent()
}

func TestIgnoringDropsTheLine(t *testing.T) {
	h := bottest.New(t, bottest.Ignoring("spammer"))
	h.Bot.HandleFunc("ping", false, func(m *bot.Message) {
		h.Bot.Say(m.ReplyTo(), "pong")
	})
	h.Start()

	h.Say("spammer", "#chan", "!ping")
	h.Silent()
}

func TestAdminCommandsNeedTheRightHost(t *testing.T) {
	h := bottest.New(t, bottest.WithAdmin("boss", "example.net"))
	h.Bot.HandleFunc("secret", true, func(m *bot.Message) {
		h.Bot.Say(m.ReplyTo(), "yes boss")
	})
	h.Start()

	// bottest sends every nick from example.net, so this one matches.
	h.Say("boss", "#chan", "!secret")
	if got := h.Next(); got != "PRIVMSG #chan :yes boss" {
		t.Errorf("got %q", got)
	}

	h.Say("nobody", "#chan", "!secret")
	h.Silent()
}

func TestChannelsTheBotWasPutIn(t *testing.T) {
	h := bottest.New(t, bottest.InChannels("#one", "#two"))
	h.Start()

	got := strings.Join(h.Bot.Channels(), " ")
	if !strings.Contains(got, "#one") || !strings.Contains(got, "#two") {
		t.Errorf("channels = %q", got)
	}
}

func TestWhoisAnswersWhatItWasTold(t *testing.T) {
	h := bottest.New(t)
	h.Says("someone", bottest.Whois{Account: "SomeAccount", Channels: "#chan"})
	h.Start()

	who, err := h.Bot.WhoisInfo(context.Background(), "someone")
	if err != nil {
		t.Fatal(err)
	}
	if who.Account != "SomeAccount" {
		t.Errorf("Account = %q", who.Account)
	}
	if len(who.Channels) != 1 || who.Channels[0] != "#chan" {
		t.Errorf("Channels = %q", who.Channels)
	}
	if n := h.WhoisCount(); n != 1 {
		t.Errorf("WhoisCount = %d, want the bot's own lookup not counted", n)
	}
}

func TestWhoisOnAnUnknownNick(t *testing.T) {
	h := bottest.New(t)
	h.Start()

	who, err := h.Bot.WhoisInfo(context.Background(), "nobody")
	if err != nil {
		t.Fatal(err)
	}
	if who.Identified() {
		t.Error("a nick the server never heard of came back identified")
	}
}

func TestCollectTakesAWholeBurst(t *testing.T) {
	h := bottest.New(t)
	h.Bot.HandleFunc("many", false, func(m *bot.Message) {
		for _, s := range []string{"one", "two", "three"} {
			h.Bot.Say(m.ReplyTo(), s)
		}
	})
	h.Start()

	h.Say("someone", "#chan", "!many")
	if lines := h.Collect(); len(lines) != 3 {
		t.Errorf("got %d lines, want the whole burst: %q", len(lines), lines)
	}
}

func TestSaid(t *testing.T) {
	lines := []string{"PRIVMSG #chan :hello", "PRIVMSG #chan :goodbye"}
	if !bottest.Said(lines, "goodbye") {
		t.Error("Said missed a line that is there")
	}
	if bottest.Said(lines, "nothing") {
		t.Error("Said found a line that is not")
	}
}

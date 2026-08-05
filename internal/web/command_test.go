package web_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/bot/bottest"
	"github.com/ohayoubot/ohayou-bot/internal/web"
)

const (
	testSecret = "0123456789abcdef0123456789abcdef"
	testURL    = "https://hemera.day/"
)

func install(t *testing.T, scopes web.Scope, opts ...bottest.Option) *bottest.Harness {
	t.Helper()
	h := bottest.New(t, opts...)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if !web.Install(h.Bot, web.NewMinter(testSecret), log, testURL, scopes) {
		t.Fatal("Install refused a site that should have come up")
	}
	h.Start()
	return h
}

// identify does what !identify does, so the nick has proved itself to the bot.
func identify(t *testing.T, h *bottest.Harness, nick string) {
	t.Helper()
	if _, err := h.Bot.Identify(context.Background(), nick); err != nil {
		t.Fatalf("identifying %q: %v", nick, err)
	}
}

// linkIn returns the target and grant out of whichever line carries the url.
func linkIn(t *testing.T, lines []string) (target, token string) {
	t.Helper()
	for _, line := range lines {
		if i := strings.Index(line, testURL+"#"); i >= 0 {
			return strings.Fields(line)[1], line[i+len(testURL)+1:]
		}
	}
	t.Fatalf("no link in %v", lines)
	return "", ""
}

// A link said out loud is a session for whoever reads the channel first.
func TestWebSendsTheLinkPrivately(t *testing.T) {
	h := install(t, web.ScopeDrop|web.ScopeOhayou)
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "@#chan"})
	identify(t, h, "mallow")

	h.Say("mallow", "#chan", "!web")
	lines := h.Drain()

	target, _ := linkIn(t, lines)
	if strings.HasPrefix(target, "#") {
		t.Errorf("the link went to %q, which is a channel", target)
	}
	var pointed bool
	for _, line := range lines {
		if strings.Contains(line, "PRIVMSG #chan") && strings.Contains(line, "PMs") {
			pointed = true
		}
	}
	if !pointed {
		t.Error("nothing in the channel said to look in PMs")
	}
}

// One link covers every part of the site the asker can use.
func TestWebGrantCarriesEveryEnabledScope(t *testing.T) {
	h := install(t, web.ScopeDrop|web.ScopeOhayou, bottest.InChannels("#chan", "#other"))
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "@#chan +#other"})
	identify(t, h, "mallow")

	h.Say("mallow", "#chan", "!web")
	_, token := linkIn(t, h.Drain())

	g, id, err := web.NewMinter(testSecret).Verify(token)
	if err != nil {
		t.Fatalf("the link the user was given does not verify: %v", err)
	}
	if id == "" {
		t.Error("the grant carries no id, so it could be redeemed twice")
	}
	if g.Account != "Mallow" || g.Nick != "mallow" {
		t.Errorf("account/nick = %q/%q", g.Account, g.Nick)
	}
	if g.Scopes != web.ScopeDrop|web.ScopeOhayou {
		t.Errorf("scopes = %d, want both", g.Scopes)
	}
	if strings.Join(g.Channels, " ") != "#chan #other" {
		t.Errorf("channels = %v", g.Channels)
	}
}

// A plugin that is off must not be reachable with a link minted while it was.
func TestWebGrantOmitsScopesNobodyAskedFor(t *testing.T) {
	h := install(t, web.ScopeDrop)
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})
	identify(t, h, "mallow")

	h.Say("mallow", "#chan", "!web")
	_, token := linkIn(t, h.Drain())

	g, _, err := web.NewMinter(testSecret).Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if g.Scopes&web.ScopeOhayou != 0 {
		t.Errorf("scopes = %d, which reaches a plugin that is off", g.Scopes)
	}
}

func TestWebRefusesAnUnidentifiedNick(t *testing.T) {
	h := install(t, web.ScopeDrop)
	// Logged in to services when they identified, logged out since.
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})
	identify(t, h, "mallow")
	h.Says("mallow", bottest.Whois{Channels: "#chan"})

	h.Say("mallow", "#chan", "!web")
	lines := h.Drain()

	if _, ok := anyLink(lines); ok {
		t.Error("a nick that is not identified was given a link")
	}
	if !said(lines, "identified") {
		t.Errorf("nothing explained the refusal: %v", lines)
	}
}

// The site login is the bot's, so services alone does not open it: without this
// a nick left behind by somebody offline is a session in their name.
func TestWebRefusesANickThatNeverIdentifiedWithTheBot(t *testing.T) {
	h := install(t, web.ScopeDrop)
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})

	h.Say("mallow", "#chan", "!web")
	lines := h.Drain()

	if _, ok := anyLink(lines); ok {
		t.Error("a nick that never identified with the bot was given a link")
	}
	if !said(lines, "identify") {
		t.Errorf("the refusal did not say how to fix it: %v", lines)
	}
	if n := h.WhoisCount(); n != 0 {
		t.Errorf("%d whois, want the refusal decided without asking the server", n)
	}
}

// A proof the bot never saw expire, against a nick now logged in as somebody
// else.
func TestWebRefusesWhenTheAccountHasChangedSinceIdentifying(t *testing.T) {
	h := install(t, web.ScopeDrop)
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})
	identify(t, h, "mallow")
	h.Says("mallow", bottest.Whois{Account: "Someone", Channels: "#chan"})

	h.Say("mallow", "#chan", "!web")
	lines := h.Drain()

	if _, ok := anyLink(lines); ok {
		t.Error("a nick whose account changed was given a link anyway")
	}
	if !said(lines, "identify") {
		t.Errorf("the refusal did not say how to fix it: %v", lines)
	}
}

// The same account in another case is the same account.
func TestWebAcceptsTheAccountInAnotherCase(t *testing.T) {
	h := install(t, web.ScopeDrop)
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})
	identify(t, h, "mallow")
	h.Says("mallow", bottest.Whois{Account: "mallow", Channels: "#chan"})

	h.Say("mallow", "#chan", "!web")
	if _, ok := anyLink(h.Drain()); !ok {
		t.Error("no link for a nick identified as the same account")
	}
}

func TestWebRefusesANickWeShareNothingWith(t *testing.T) {
	h := install(t, web.ScopeDrop)
	h.Says("stranger", bottest.Whois{Account: "Stranger", Channels: "#elsewhere"})
	identify(t, h, "stranger")

	h.Say("stranger", "#chan", "!web")
	lines := h.Drain()

	if _, ok := anyLink(lines); ok {
		t.Error("a nick sharing no channel was given a link")
	}
}

// Without it, repeating the command is a WHOIS at the server each time.
func TestWebCooldownStopsARepeat(t *testing.T) {
	h := install(t, web.ScopeDrop)
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})
	identify(t, h, "mallow")
	before := h.WhoisCount()

	h.Say("mallow", "#chan", "!web")
	h.Drain()
	if n := h.WhoisCount() - before; n != 1 {
		t.Fatalf("%d whois for the first link, want 1", n)
	}

	h.Say("mallow", "#chan", "!web")
	lines := h.Drain()
	if _, ok := anyLink(lines); ok {
		t.Error("a second link came straight away")
	}
	if n := h.WhoisCount() - before; n != 1 {
		t.Errorf("%d whois, want the repeat refused before asking", n)
	}
}

// Nothing to sign with, nowhere to send anyone, or nothing to do there.
func TestInstallRefusesAnIncompleteSite(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for name, run := range map[string]func() bool{
		"no secret": func() bool {
			return web.Install(bottest.New(t).Bot, web.NewMinter(""), log, testURL, web.ScopeDrop)
		},
		"no url": func() bool {
			return web.Install(bottest.New(t).Bot, web.NewMinter(testSecret), log, "", web.ScopeDrop)
		},
		"no scopes": func() bool {
			return web.Install(bottest.New(t).Bot, web.NewMinter(testSecret), log, testURL, 0)
		},
	} {
		if run() {
			t.Errorf("%s: installed anyway", name)
		}
	}
}

func anyLink(lines []string) (string, bool) {
	for _, line := range lines {
		if i := strings.Index(line, testURL+"#"); i >= 0 {
			return line[i:], true
		}
	}
	return "", false
}

func said(lines []string, want string) bool {
	for _, line := range lines {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}

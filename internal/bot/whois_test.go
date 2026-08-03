package bot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	irc "github.com/ohayoubot/go-ircevent"

	"github.com/ohayoubot/ohayou-bot/internal/config"
)

// newWhoisBot returns a bot with the WHOIS handlers installed and no
// connection. Numerics are fed straight to the callbacks, and the WHOIS the
// lookup sends goes nowhere, which is what lets a test decide what comes back.
func newWhoisBot(t *testing.T) *Bot {
	t.Helper()
	return New(&config.Config{Nick: "ohayoubot", User: "ohayoubot", Server: "127.0.0.1"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func (b *Bot) feed(code string, args ...string) {
	b.conn.RunCallbacks(&irc.Event{Code: code, Arguments: args})
}

// waitPending blocks until n callers are waiting on a lookup for nick, so a
// test cannot answer a WHOIS before everyone it is checking has asked for it.
func waitPending(t *testing.T, b *Bot, nick string, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b.whoisMu.Lock()
		pending, ok := b.whois[nick]
		waiters := 0
		if ok {
			waiters = pending.waiters
		}
		b.whoisMu.Unlock()
		if waiters >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("fewer than %d waiters on a whois for %q", n, nick)
}

func TestWhoisReportsAccountAndChannels(t *testing.T) {
	b := newWhoisBot(t)

	go func() {
		waitPending(t, b, "someone", 1)
		b.feed("330", "ohayoubot", "someone", "SomeAccount", "is logged in as")
		b.feed("319", "ohayoubot", "someone", "@#chan +#other")
		b.feed("318", "ohayoubot", "someone", "End of /WHOIS list")
	}()

	got, err := b.WhoisInfo(context.Background(), "someone")
	if err != nil {
		t.Fatalf("WhoisInfo: %v", err)
	}
	if got.Account != "SomeAccount" || !got.Identified() {
		t.Errorf("account = %q, want SomeAccount", got.Account)
	}
	if len(got.Channels) != 2 || got.Channels[0] != "#chan" || got.Channels[1] != "#other" {
		t.Errorf("channels = %v, want [#chan #other]", got.Channels)
	}
}

// Rizon's ircd has no account name, so 307 alone means identified.
func TestWhois307MeansIdentifiedAsTheNick(t *testing.T) {
	b := newWhoisBot(t)

	go func() {
		waitPending(t, b, "someone", 1)
		b.feed("307", "ohayoubot", "someone", "has identified for this nick")
		b.feed("318", "ohayoubot", "someone", "End of /WHOIS list")
	}()

	got, err := b.WhoisInfo(context.Background(), "someone")
	if err != nil {
		t.Fatalf("WhoisInfo: %v", err)
	}
	if got.Account != "someone" {
		t.Errorf("account = %q, want someone", got.Account)
	}
}

func TestWhoisUnidentifiedNickHasNoAccount(t *testing.T) {
	b := newWhoisBot(t)

	go func() {
		waitPending(t, b, "someone", 1)
		b.feed("319", "ohayoubot", "someone", "#chan")
		b.feed("318", "ohayoubot", "someone", "End of /WHOIS list")
	}()

	got, err := b.WhoisInfo(context.Background(), "someone")
	if err != nil {
		t.Fatalf("WhoisInfo: %v", err)
	}
	if got.Identified() {
		t.Errorf("account = %q, want empty", got.Account)
	}
}

// The reason this package has its own resolver: the old implementation keyed on
// nothing, so whichever WHOIS ended first answered every caller.
func TestOverlappingWhoisDoNotCrossTalk(t *testing.T) {
	b := newWhoisBot(t)

	type answer struct {
		result WhoisResult
		err    error
	}
	alice := make(chan answer, 1)
	bob := make(chan answer, 1)

	go func() {
		r, err := b.WhoisInfo(context.Background(), "alice")
		alice <- answer{r, err}
	}()
	go func() {
		r, err := b.WhoisInfo(context.Background(), "bob")
		bob <- answer{r, err}
	}()

	waitPending(t, b, "alice", 1)
	waitPending(t, b, "bob", 1)

	// Bob is unidentified and his WHOIS ends first. Alice's must not take it.
	b.feed("318", "ohayoubot", "bob", "End of /WHOIS list")
	b.feed("330", "ohayoubot", "alice", "AliceAcct", "is logged in as")
	b.feed("318", "ohayoubot", "alice", "End of /WHOIS list")

	got := <-bob
	if got.err != nil || got.result.Identified() {
		t.Errorf("bob = %+v, %v; want unidentified", got.result, got.err)
	}
	got = <-alice
	if got.err != nil || got.result.Account != "AliceAcct" {
		t.Errorf("alice = %+v, %v; want AliceAcct", got.result, got.err)
	}
}

func TestWhoisSharesOneLookupPerNick(t *testing.T) {
	b := newWhoisBot(t)

	first := make(chan WhoisResult, 1)
	second := make(chan WhoisResult, 1)

	go func() {
		r, _ := b.WhoisInfo(context.Background(), "someone")
		first <- r
	}()
	waitPending(t, b, "someone", 1)
	go func() {
		r, _ := b.WhoisInfo(context.Background(), "SomeOne")
		second <- r
	}()
	waitPending(t, b, "someone", 2)

	b.feed("330", "ohayoubot", "someone", "Acct", "is logged in as")
	b.feed("318", "ohayoubot", "someone", "End of /WHOIS list")

	if a, bb := <-first, <-second; a.Account != "Acct" || bb.Account != "Acct" {
		t.Errorf("got %q and %q, want both Acct", a.Account, bb.Account)
	}
}

func TestWhoisNoSuchNickEndsTheLookup(t *testing.T) {
	b := newWhoisBot(t)

	go func() {
		waitPending(t, b, "ghost", 1)
		b.feed("401", "ohayoubot", "ghost", "No such nick/channel")
	}()

	got, err := b.WhoisInfo(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("WhoisInfo: %v", err)
	}
	if got.Identified() {
		t.Error("a nick that does not exist came back identified")
	}
}

func TestWhoisCancelledContext(t *testing.T) {
	b := newWhoisBot(t)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		waitPending(t, b, "someone", 1)
		cancel()
	}()

	if _, err := b.WhoisInfo(ctx, "someone"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	// The entry must not survive: a later reply completes it, and the sweep
	// clears it either way.
	b.feed("318", "ohayoubot", "someone", "End of /WHOIS list")
	b.whoisMu.Lock()
	_, leaked := b.whois["someone"]
	b.whoisMu.Unlock()
	if leaked {
		t.Error("pending whois leaked after cancellation")
	}
}

func TestWhoisChannelsStripsPrefixes(t *testing.T) {
	got := whoisChannels("@#one +#two #three ~#four &#five")
	want := []string{"#one", "#two", "#three", "#four", "#five"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if n := len(whoisChannels("&local +&other")); n != 0 {
		t.Errorf("kept %d non-# channels, want 0", n)
	}
}

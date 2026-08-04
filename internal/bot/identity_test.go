package bot

import (
	"context"
	"testing"
	"time"

	irc "github.com/ohayoubot/go-ircevent"
)

// answerWhois ends a pending lookup for nick, optionally logged in as account.
func answerWhois(b *Bot, nick, account string) {
	if account != "" {
		b.updateWhois(nick, func(p *whoisPending) { p.result.Account = account })
	}
	b.finishWhois(nick, false)
}

// identify runs an Identify in the background and answers the lookup it makes.
func identify(t *testing.T, b *Bot, nick, account string) bool {
	t.Helper()

	type result struct {
		ok  bool
		err error
	}
	done := make(chan result, 1)
	go func() {
		ok, err := b.Identify(context.Background(), nick)
		done <- result{ok, err}
	}()

	waitPendingWhois(t, b, nick)
	answerWhois(b, nick, account)

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Identify(%q): %v", nick, r.err)
		}
		return r.ok
	case <-time.After(3 * time.Second):
		t.Fatalf("Identify(%q) never returned", nick)
		return false
	}
}

// waitPendingWhois blocks until nick's lookup is registered, so the answer
// cannot arrive before there is anything to answer.
func waitPendingWhois(t *testing.T, b *Bot, nick string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		b.whoisMu.Lock()
		_, ok := b.whois[nick]
		b.whoisMu.Unlock()
		if ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no whois was sent for %q", nick)
}

func TestIdentifiedIsFalseUntilProven(t *testing.T) {
	b := testBot()
	if b.Identified("someone") {
		t.Error("a nick nobody checked is identified")
	}
}

func TestIdentifyRemembersALoggedInNick(t *testing.T) {
	b := testBot()

	if !identify(t, b, "someone", "SomeAccount") {
		t.Fatal("Identify said no for a nick logged in to services")
	}
	if !b.Identified("someone") {
		t.Error("the proof was not remembered")
	}
	if !b.Identified("SoMeOnE") {
		t.Error("the proof is case sensitive")
	}
}

func TestIdentifyRemembersNothingForALoggedOutNick(t *testing.T) {
	b := testBot()

	if identify(t, b, "someone", "") {
		t.Fatal("Identify said yes for a nick that is not logged in")
	}
	if b.Identified("someone") {
		t.Error("a nick that failed the check was remembered anyway")
	}
}

// The proof is only good while the nick is the one that gave it.
func TestChangingNickDropsTheProof(t *testing.T) {
	b := testBot()
	identify(t, b, "someone", "SomeAccount")

	b.conn.RunCallbacks(&irc.Event{Code: "NICK", Nick: "someone", Arguments: []string{"someoneelse"}})
	b.Wait() // handlers run in their own goroutines

	if b.Identified("someone") {
		t.Error("the old nick is still identified after a nick change")
	}
	if b.Identified("someoneelse") {
		t.Error("the new nick inherited a proof it never gave")
	}
}

func TestQuittingDropsTheProof(t *testing.T) {
	b := testBot()
	identify(t, b, "someone", "SomeAccount")

	b.conn.RunCallbacks(&irc.Event{Code: "QUIT", Nick: "someone"})
	b.Wait()

	if b.Identified("someone") {
		t.Error("a nick that quit is still identified")
	}
}

func TestOneNickQuittingLeavesAnotherAlone(t *testing.T) {
	b := testBot()
	identify(t, b, "someone", "SomeAccount")
	identify(t, b, "another", "AnotherAccount")

	b.conn.RunCallbacks(&irc.Event{Code: "QUIT", Nick: "someone"})
	b.Wait()

	if !b.Identified("another") {
		t.Error("one nick quitting dropped another's proof")
	}
}

// A lookup that failed is not a "no": Verify must not report a nick as logged
// out when the server never answered.
func TestVerifyReportsATimeoutAsAnError(t *testing.T) {
	b := testBot()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := b.Verify(ctx, "someone"); err == nil {
		t.Error("a lookup that never answered came back as a clean no")
	}
	if b.Identified("someone") {
		t.Error("a failed lookup was remembered")
	}
}

package bot

import (
	"testing"

	irc "github.com/ohayoubot/go-ircevent"
)

func TestJoinEvent(t *testing.T) {
	b := testBot()

	var got JoinEvent
	b.OnJoin(func(e JoinEvent) { got = e })

	b.conn.RunCallbacks(&irc.Event{
		Code: "JOIN", Nick: "someone", Host: "a.host", Arguments: []string{"#chan"},
	})
	b.Wait()

	if got.Nick != "someone" || got.Channel != "#chan" || got.Host != "a.host" {
		t.Errorf("got %+v", got)
	}
}

// Servers put the channel in either the trailing parameter or the argument.
func TestJoinEventReadsEitherSpelling(t *testing.T) {
	b := testBot()

	var got []string
	b.OnJoin(func(e JoinEvent) { got = append(got, e.Channel) })

	b.conn.RunCallbacks(&irc.Event{Code: "JOIN", Nick: "a", Arguments: []string{"#one"}})
	b.Wait()
	b.conn.RunCallbacks(&irc.Event{Code: "JOIN", Nick: "a", Arguments: []string{"", "#two"}})
	b.Wait()

	if len(got) != 2 || got[0] != "#one" || got[1] != "#two" {
		t.Errorf("channels = %q", got)
	}
}

func TestPartEvent(t *testing.T) {
	b := testBot()

	var got PartEvent
	b.OnPart(func(e PartEvent) { got = e })

	b.conn.RunCallbacks(&irc.Event{
		Code: "PART", Nick: "someone", Arguments: []string{"#chan", "bye then"},
	})
	b.Wait()

	if got.Nick != "someone" || got.Channel != "#chan" || got.Reason != "bye then" {
		t.Errorf("got %+v", got)
	}
}

// KICK names the person removed in the arguments; e.Nick is whoever did it.
func TestKickEventSeparatesVictimFromKicker(t *testing.T) {
	b := testBot()

	var got KickEvent
	b.OnKick(func(e KickEvent) { got = e })

	b.conn.RunCallbacks(&irc.Event{
		Code: "KICK", Nick: "anop", Arguments: []string{"#chan", "someone", "out"},
	})
	b.Wait()

	if got.Nick != "someone" {
		t.Errorf("Nick = %q, want the one removed", got.Nick)
	}
	if got.By != "anop" {
		t.Errorf("By = %q, want the one doing the removing", got.By)
	}
	if got.Channel != "#chan" || got.Reason != "out" {
		t.Errorf("got %+v", got)
	}
}

func TestNickEvent(t *testing.T) {
	b := testBot()

	var got NickEvent
	b.OnNick(func(e NickEvent) { got = e })

	b.conn.RunCallbacks(&irc.Event{
		Code: "NICK", Nick: "before", Arguments: []string{"after"},
	})
	b.Wait()

	if got.From != "before" || got.To != "after" {
		t.Errorf("got %+v", got)
	}
}

func TestQuitEvent(t *testing.T) {
	b := testBot()

	var got QuitEvent
	b.OnQuit(func(e QuitEvent) { got = e })

	b.conn.RunCallbacks(&irc.Event{
		Code: "QUIT", Nick: "someone", Arguments: []string{"Ping timeout"},
	})
	b.Wait()

	if got.Nick != "someone" || got.Reason != "Ping timeout" {
		t.Errorf("got %+v", got)
	}
}

func TestEventsWithNoReasonAreNotAnError(t *testing.T) {
	b := testBot()

	var got PartEvent
	b.OnPart(func(e PartEvent) { got = e })

	b.conn.RunCallbacks(&irc.Event{Code: "PART", Nick: "someone", Arguments: []string{"#chan"}})
	b.Wait()

	if got.Channel != "#chan" || got.Reason != "" {
		t.Errorf("got %+v", got)
	}
}

func TestEveryHandlerIsCalled(t *testing.T) {
	b := testBot()

	calls := make(chan int, 3)
	for i := 0; i < 3; i++ {
		b.OnQuit(func(e QuitEvent) { calls <- i })
	}

	b.conn.RunCallbacks(&irc.Event{Code: "QUIT", Nick: "someone"})
	b.Wait()

	if len(calls) != 3 {
		t.Errorf("%d handlers ran, want 3", len(calls))
	}
}

// The bot tracks its own channel list through the same events a plugin uses.
func TestJoiningAndBeingKickedTrackTheChannelList(t *testing.T) {
	b := testBot()

	b.conn.RunCallbacks(&irc.Event{
		Code: "JOIN", Nick: b.conn.GetNick(), Arguments: []string{"#chan"},
	})
	b.Wait()
	if got := b.Channels(); len(got) != 1 || got[0] != "#chan" {
		t.Fatalf("channels = %q after joining", got)
	}

	b.conn.RunCallbacks(&irc.Event{
		Code: "KICK", Nick: "anop", Arguments: []string{"#chan", b.conn.GetNick(), "out"},
	})
	b.Wait()
	if got := b.Channels(); len(got) != 0 {
		t.Errorf("channels = %q after being kicked", got)
	}
}

// Somebody else being kicked is not the bot leaving.
func TestAnotherNickBeingKickedLeavesTheListAlone(t *testing.T) {
	b := testBot()
	b.conn.RunCallbacks(&irc.Event{
		Code: "JOIN", Nick: b.conn.GetNick(), Arguments: []string{"#chan"},
	})
	b.Wait()

	b.conn.RunCallbacks(&irc.Event{
		Code: "KICK", Nick: "anop", Arguments: []string{"#chan", "someone-else", "out"},
	})
	b.Wait()

	if got := b.Channels(); len(got) != 1 {
		t.Errorf("channels = %q, want the bot still in it", got)
	}
}

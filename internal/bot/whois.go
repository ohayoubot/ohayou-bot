package bot

import (
	"context"
	"errors"
	"strings"
	"time"

	irc "github.com/ohayoubot/go-ircevent"
)

// whoisWait bounds a lookup whose reply never arrives, so a pending entry
// cannot outlive the request that made it.
const whoisWait = 10 * time.Second

// ErrWhoisTimeout means the server never ended the WHOIS. The result is unknown
// rather than negative: callers gating on identification must refuse, not
// assume the nick is unregistered.
var ErrWhoisTimeout = errors.New("bot: whois timed out")

// WhoisResult is what a WHOIS said about a nick.
type WhoisResult struct {
	Nick string
	// Account is the services account, empty when the nick is not identified.
	Account string
	// Channels are those the server disclosed, which for a nick that shares
	// channels with the bot includes the shared ones.
	Channels []string
}

// Identified reports whether the nick was logged in to services.
func (r WhoisResult) Identified() bool { return r.Account != "" }

type whoisPending struct {
	result   WhoisResult
	timedOut bool
	waiters  int
	done     chan struct{}
}

// registerWhois installs the WHOIS numeric handlers once, for the life of the
// connection. They dispatch on the nick each numeric names, so overlapping
// lookups cannot answer each other. Nothing may ClearCallback these codes.
func (b *Bot) registerWhois() {
	// 330 RPL_WHOISACCOUNT: "<me> <nick> <account> :is logged in as".
	b.conn.AddCallback("330", func(e *irc.Event) {
		if len(e.Arguments) < 3 {
			return
		}
		b.updateWhois(e.Arguments[1], func(p *whoisPending) {
			p.result.Account = e.Arguments[2]
		})
	})

	// 307 RPL_WHOISREGNICK: "<me> <nick> :has identified for this nick". Sent
	// by ircds that have no separate account name, Rizon's among them, so the
	// account is the nick. 330 wins where both arrive.
	b.conn.AddCallback("307", func(e *irc.Event) {
		if len(e.Arguments) < 2 {
			return
		}
		b.updateWhois(e.Arguments[1], func(p *whoisPending) {
			if p.result.Account == "" {
				p.result.Account = e.Arguments[1]
			}
		})
	})

	// 319 RPL_WHOISCHANNELS: "<me> <nick> :@#one +#two #three". May arrive more
	// than once for a nick in many channels.
	b.conn.AddCallback("319", func(e *irc.Event) {
		if len(e.Arguments) < 3 {
			return
		}
		b.updateWhois(e.Arguments[1], func(p *whoisPending) {
			p.result.Channels = append(p.result.Channels, whoisChannels(e.Arguments[2])...)
		})
	})

	// 318 RPL_ENDOFWHOIS, and 401 ERR_NOSUCHNICK for a nick that is not there.
	end := func(e *irc.Event) {
		if len(e.Arguments) < 2 {
			return
		}
		b.finishWhois(e.Arguments[1], false)
	}
	b.conn.AddCallback("318", end)
	b.conn.AddCallback("401", end)
}

// WhoisInfo asks the server about nick and waits for the reply.
//
// Concurrent calls for the same nick share one lookup and one answer. Callers
// run in their own goroutine (see router.go), so blocking here is expected.
func (b *Bot) WhoisInfo(ctx context.Context, nick string) (WhoisResult, error) {
	key := strings.ToLower(nick)

	b.whoisMu.Lock()
	pending, inFlight := b.whois[key]
	if !inFlight {
		pending = &whoisPending{
			result: WhoisResult{Nick: nick},
			done:   make(chan struct{}),
		}
		b.whois[key] = pending
	}
	pending.waiters++
	b.whoisMu.Unlock()

	if !inFlight {
		b.conn.Whois(nick)
		go b.sweepWhois(key, pending)
	}

	select {
	case <-pending.done:
		b.whoisMu.Lock()
		defer b.whoisMu.Unlock()
		if pending.timedOut {
			return WhoisResult{}, ErrWhoisTimeout
		}
		return pending.result, nil
	case <-ctx.Done():
		// The entry stays for any other waiter; sweepWhois always clears it.
		return WhoisResult{}, ctx.Err()
	}
}

// sweepWhois finishes a lookup the server never ended.
func (b *Bot) sweepWhois(key string, pending *whoisPending) {
	timer := time.NewTimer(whoisWait)
	defer timer.Stop()

	select {
	case <-pending.done:
	case <-timer.C:
		b.whoisMu.Lock()
		waiting := pending.waiters
		b.whoisMu.Unlock()
		b.log.Warn("whois timed out", "nick", key, "waiting", waiting)
		b.finishWhois(key, true)
	}
}

func (b *Bot) updateWhois(nick string, fn func(*whoisPending)) {
	b.whoisMu.Lock()
	defer b.whoisMu.Unlock()
	if pending, ok := b.whois[strings.ToLower(nick)]; ok {
		fn(pending)
	}
}

// finishWhois wakes the waiters. Removing the entry first makes a second end
// numeric, or a sweep racing one, a no-op.
func (b *Bot) finishWhois(nick string, timedOut bool) {
	key := strings.ToLower(nick)

	b.whoisMu.Lock()
	pending, ok := b.whois[key]
	if ok {
		delete(b.whois, key)
		pending.timedOut = timedOut
	}
	b.whoisMu.Unlock()

	if ok {
		close(pending.done)
	}
}

// whoisChannels strips the status prefixes ("@#chan") the server puts on each
// name. Anything that is not a "#" channel is dropped: the bot joins no others,
// so a caller intersecting with its channels would discard them anyway.
func whoisChannels(field string) []string {
	var out []string
	for _, name := range strings.Fields(field) {
		if i := strings.IndexByte(name, '#'); i >= 0 {
			out = append(out, name[i:])
		}
	}
	return out
}

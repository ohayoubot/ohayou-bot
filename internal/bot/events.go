package bot

import (
	"strings"

	irc "github.com/ohayoubot/go-ircevent"
)

// The channel comings and goings a plugin can react to. They are plain structs
// rather than the irc library's Event so that plugins never touch it, and the
// library stays swappable.
type (
	// JoinEvent is somebody arriving in a channel, including the bot itself.
	JoinEvent struct {
		Nick, Host, Channel string
	}
	// PartEvent is somebody leaving a channel of their own accord.
	PartEvent struct {
		Nick, Host, Channel, Reason string
	}
	// KickEvent is somebody being removed from a channel by another.
	KickEvent struct {
		Nick, Channel, By, Reason string
	}
	// NickEvent is a nick change. From is the nick they were using.
	NickEvent struct {
		From, To, Host string
	}
	// QuitEvent is somebody leaving the network.
	QuitEvent struct {
		Nick, Host, Reason string
	}
)

type events struct {
	join []func(JoinEvent)
	part []func(PartEvent)
	kick []func(KickEvent)
	nick []func(NickEvent)
	quit []func(QuitEvent)
}

// The On* methods register a handler, each called in its own tracked goroutine.
// Like Handle and Watch they are for startup: the lists are read without a lock
// once the connection is up.
func (b *Bot) OnJoin(fn func(JoinEvent)) { b.events.join = append(b.events.join, fn) }
func (b *Bot) OnPart(fn func(PartEvent)) { b.events.part = append(b.events.part, fn) }
func (b *Bot) OnKick(fn func(KickEvent)) { b.events.kick = append(b.events.kick, fn) }
func (b *Bot) OnNick(fn func(NickEvent)) { b.events.nick = append(b.events.nick, fn) }
func (b *Bot) OnQuit(fn func(QuitEvent)) { b.events.quit = append(b.events.quit, fn) }

// registerEvents turns the irc library's callbacks into the typed ones. It runs
// once for the life of the connection, so nothing may ClearCallback these
// codes.
func (b *Bot) registerEvents() {
	b.conn.AddCallback("JOIN", func(e *irc.Event) {
		ev := JoinEvent{Nick: e.Nick, Host: e.Host, Channel: channelOf(e)}
		for _, fn := range b.events.join {
			b.Go(func() { fn(ev) })
		}
	})

	b.conn.AddCallback("PART", func(e *irc.Event) {
		ev := PartEvent{Nick: e.Nick, Host: e.Host, Channel: arg(e, 0), Reason: reasonAfter(e, 1)}
		for _, fn := range b.events.part {
			b.Go(func() { fn(ev) })
		}
	})

	// KICK is "<channel> <nick> :<reason>": the nick is the one removed, and
	// e.Nick is whoever did it.
	b.conn.AddCallback("KICK", func(e *irc.Event) {
		ev := KickEvent{Nick: arg(e, 1), Channel: arg(e, 0), By: e.Nick, Reason: reasonAfter(e, 2)}
		for _, fn := range b.events.kick {
			b.Go(func() { fn(ev) })
		}
	})

	b.conn.AddCallback("NICK", func(e *irc.Event) {
		ev := NickEvent{From: e.Nick, To: e.Message(), Host: e.Host}
		for _, fn := range b.events.nick {
			b.Go(func() { fn(ev) })
		}
	})

	b.conn.AddCallback("QUIT", func(e *irc.Event) {
		ev := QuitEvent{Nick: e.Nick, Host: e.Host, Reason: e.Message()}
		for _, fn := range b.events.quit {
			b.Go(func() { fn(ev) })
		}
	})
}

// channelOf reads the channel a JOIN names, which servers put in either the
// trailing parameter or the first argument.
func channelOf(e *irc.Event) string {
	if ch := e.Message(); ch != "" {
		return ch
	}
	return arg(e, 0)
}

func arg(e *irc.Event, n int) string {
	if n < len(e.Arguments) {
		return e.Arguments[n]
	}
	return ""
}

// reasonAfter joins whatever follows the fixed arguments, which is the reason
// when the server sent one and empty when it did not.
func reasonAfter(e *irc.Event, n int) string {
	if n >= len(e.Arguments) {
		return ""
	}
	return strings.Join(e.Arguments[n:], " ")
}

// IsMe reports whether a nick is the bot's own.
func (b *Bot) IsMe(nick string) bool { return strings.EqualFold(nick, b.conn.GetNick()) }

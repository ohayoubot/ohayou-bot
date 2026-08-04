// Package bot provides the irc connection and bot functionalities.
package bot

import (
	"context"
	"crypto/tls"
	"fmt"
	stdlog "log"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	irc "github.com/ohayoubot/go-ircevent"

	"github.com/ohayoubot/ohayou-bot/internal/config"
)

// Bot encapsulates the irc conn, config, and commands. The game contains a single instance of this
type Bot struct {
	cfg  *config.Config
	conn *irc.Connection
	send *sender
	log  *slog.Logger

	commands map[string]Command
	watchers []Watcher

	joined atomic.Bool // set once channels have been joined

	wg sync.WaitGroup // tracks goroutines so shutdown can drain them

	mu       sync.RWMutex // guards channels and ignore
	channels []string     // channel names currently joined. can differ from config if !join used
	ignore   map[string]string

	whoisMu sync.Mutex // guards whois
	whois   map[string]*whoisPending

	identMu    sync.RWMutex // guards identified
	identified map[string]bool
}

func New(cfg *config.Config, log *slog.Logger) *Bot {
	conn := irc.IRC(cfg.Nick, cfg.User)
	conn.Debug = cfg.Debug
	conn.VerboseCallbackHandler = cfg.Verbose
	// go-ircevent has its own logger for connect/disconnect/etc. Ensure it's routed through the bot
	// logger (slog.Logger).
	conn.Log = stdlog.New(slogWriter{log: log}, "", 0)

	if cfg.TLS {
		conn.UseTLS = true
		conn.TLSConfig = &tls.Config{ServerName: cfg.Server}
	}
	if cfg.SASL.Use() {
		conn.UseSASL = true
		conn.SASLMech = cfg.SASL.Mech()
		conn.SASLLogin = cfg.SASL.Login
		conn.SASLPassword = cfg.SASL.Password
		log.Info("sasl enabled", "mechanism", conn.SASLMech, "login", cfg.SASL.Login)
		if conn.SASLMech == "PLAIN" && !cfg.TLS {
			log.Warn("sasl PLAIN over a plaintext connection. enable tls to protect the password")
		}
	}

	b := &Bot{
		cfg:      cfg,
		conn:     conn,
		send:     newSender(conn, cfg.FloodDelay()),
		log:      log,
		commands: map[string]Command{},
		channels: channelNames(cfg.Channels),
		ignore:   cloneMap(cfg.IgnoreList),
		whois:    map[string]*whoisPending{},

		identified: map[string]bool{},
	}
	b.registerBotCommands()
	b.registerAdminCommands()
	b.registerWhois()
	b.registerIdentity()
	return b
}

// Handle registers a command. Later registrations override earlier ones.
func (b *Bot) Handle(c Command) { b.commands[c.Name] = c }

// HandleFunc is a convenience wrapper around Handle.
func (b *Bot) HandleFunc(name string, admin bool, h Handler) {
	b.Handle(Command{Name: name, Admin: admin, Handler: h})
}

// Watch registers w to receive every message that isn't a command, each in its
// own goroutine. Like Handle it is for startup: the list is read without a lock
// once the connection is up.
func (b *Bot) Watch(w Watcher) { b.watchers = append(b.watchers, w) }

// Go runs fn in a tracked goroutine. Every goroutine that touches the store
// goes through here so Wait can drain them on shutdown before the database is
// closed. Otherwise a final write races db.Close and fails with "database is
// closed".
func (b *Bot) Go(fn func()) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		fn()
	}()
}

// Wait blocks until everything started with Go has finished. Callers cancel
// the context they gave Run first, or it waits for work that will not stop.
func (b *Bot) Wait() { b.wg.Wait() }

func (b *Bot) Say(target, msg string)    { b.send.Privmsg(target, msg) }
func (b *Bot) Notice(target, msg string) { b.send.Notice(target, msg) }
func (b *Bot) Action(target, msg string) { b.send.Action(target, msg) }

func (b *Bot) Whois(nick string)                                { b.conn.Whois(nick) }
func (b *Bot) AddCallback(code string, cb func(*irc.Event)) int { return b.conn.AddCallback(code, cb) }
func (b *Bot) RemoveCallback(code string, id int) bool          { return b.conn.RemoveCallback(code, id) }
func (b *Bot) ClearCallback(code string) bool                   { return b.conn.ClearCallback(code) }
func (b *Bot) Nick() string                                     { return b.conn.GetNick() }
func (b *Bot) Prefix() string                                   { return b.cfg.CommandPrefix }
func (b *Bot) Logger() *slog.Logger                             { return b.log }

// Channels returns a snapshot of the channel names the bot is in.
func (b *Bot) Channels() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]string(nil), b.channels...)
}

func (b *Bot) addChannel(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.channels {
		if c == name {
			return
		}
	}
	b.channels = append(b.channels, name)
}

func (b *Bot) removeChannel(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, c := range b.channels {
		if c == name {
			b.channels = append(b.channels[:i], b.channels[i+1:]...)
			return
		}
	}
}

// Ignored reports whether the bot is ignoring nick, so a plugin reading the
// lines the router hands it can apply the same bar.
func (b *Bot) Ignored(nick string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.ignore[strings.ToLower(nick)]
	return ok
}

func (b *Bot) addIgnore(nick, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ignore[strings.ToLower(nick)] = reason
}

func (b *Bot) removeIgnore(nick string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := strings.ToLower(nick)
	if _, ok := b.ignore[key]; !ok {
		return false
	}
	delete(b.ignore, key)
	return true
}

func (b *Bot) ignoreSnapshot() map[string]string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return cloneMap(b.ignore)
}

// Run connects, registers callbacks, and blocks until the context is cancelled
// or the connection drops.
func (b *Bot) Run(ctx context.Context) error {
	// 001 marks a fresh (server) registration, including after an automatic reconnect.
	b.conn.AddCallback("001", func(e *irc.Event) {
		// Reset the join guard so channels are rejoined every time (otherwise the bot would sit
		// idle after its first netsplit).
		b.joined.Store(false)

		usingSASL := b.cfg.SASL.Use()
		b.log.Info("connected",
			"server", b.cfg.Server,
			"nick", b.conn.GetNick(),
			"tls", b.cfg.TLS,
			"sasl", usingSASL,
			"vhostGate", b.cfg.VHostEnabled())

		// Don't bother identifying if using SASL
		if b.cfg.NickPW != "" && !usingSASL {
			b.log.Info("identifying with nickserv")
			b.Say("NickServ", "identify "+b.cfg.NickPW)
		}

		if !b.cfg.VHostEnabled() {
			b.joinChannels()
			return
		}

		if usingSASL {
			b.log.Info("vhost already on via sasl login. joining channels")
			b.joinChannels()
			return
		}

		// If we didn't use sasl, ensure that the vhost is enabled from ns identify
		if cmd := b.cfg.VHost.Command; cmd != "" {
			b.log.Info("enabling vhost", "service", b.cfg.VHost.Service, "command", cmd)
			b.Say(b.cfg.VHost.Service, cmd)
		}
		b.log.Info("waiting for vhost confirmation before joining",
			"timeout", b.cfg.VHostTimeout())
		b.awaitVHost()
	})

	// 396 RPL_HOSTHIDDEN: "<nick> <displayed-host> :is now your displayed host".
	// This will tell us that a vhost is active and it is safe to join
	b.conn.AddCallback("396", func(e *irc.Event) {
		host := ""
		if len(e.Arguments) >= 2 {
			host = e.Arguments[1]
		}
		b.log.Info("host masked (396)", "host", host)
		if b.cfg.VHostEnabled() {
			b.joinChannels()
		}
	})

	// 311 RPL_WHOISUSER: "<me> <nick> <user> <host> * :<realname>". After
	// connecting the bot WHOISes itself (see joinChannels) so the host it is
	// actually displaying shows up in the logs
	b.conn.AddCallback("311", func(e *irc.Event) {
		if len(e.Arguments) < 4 || !strings.EqualFold(e.Arguments[1], b.conn.GetNick()) {
			return
		}
		b.log.Info("host confirmed", "host", e.Arguments[3], "user", e.Arguments[2])
	})
	b.conn.AddCallback("PRIVMSG", b.onPrivmsg)
	b.registerDiagnosticCallbacks()

	for admin, host := range b.cfg.Admins {
		b.log.Info("admin", "mask", fmt.Sprintf("%s!*@%s", admin, host))
	}

	addr := fmt.Sprintf("%s:%d", b.cfg.Server, b.cfg.Port)
	b.log.Info("connecting",
		"addr", addr,
		"tls", b.cfg.TLS,
		"sasl", b.cfg.SASL.Use(),
		"vhostGate", b.cfg.VHostEnabled())
	if err := b.conn.Connect(addr); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	go func() {
		<-ctx.Done()
		b.log.Info("shutting down")
		b.conn.Quit()
	}()

	b.conn.Loop()
	b.log.Info("irc loop exited")
	return nil
}

// registerDiagnosticCallbacks sets callbacks whose only job is to log noteworthy
// events if something has gone wrong while running. They are otherwise silent
// unless observed from within the same channel
func (b *Bot) registerDiagnosticCallbacks() {
	me := func() string { return b.conn.GetNick() }

	// ERROR: servers send this on kill/ban/kline. Closest thing to a reason for a disconnect
	b.conn.AddCallback("ERROR", func(e *irc.Event) {
		b.log.Error("server ERROR", "msg", e.Message())
	})

	// KICK "<channel> <nick> :<reason>". Only care when we're kick, not when _we_ kick
	b.conn.AddCallback("KICK", func(e *irc.Event) {
		if len(e.Arguments) < 2 || !strings.EqualFold(e.Arguments[1], me()) {
			return
		}
		ch := e.Arguments[0]
		b.log.Warn("kicked from channel", "channel", ch, "by", e.Nick, "reason", e.Message())
		b.removeChannel(ch)
	})

	b.conn.AddCallback("JOIN", func(e *irc.Event) {
		if !strings.EqualFold(e.Nick, me()) {
			return
		}
		ch := e.Message()
		if ch == "" && len(e.Arguments) > 0 {
			ch = e.Arguments[0]
		}
		b.log.Info("joined channel", "channel", ch)
		b.addChannel(ch)
	})

	// Nick problems during server registration
	b.conn.AddCallback("433", func(e *irc.Event) { b.log.Warn("nick in use", "detail", e.Message()) })
	b.conn.AddCallback("437", func(e *irc.Event) { b.log.Warn("nick/channel temporarily unavailable", "detail", e.Message()) })

	// Channel-join issues. without these a failed join is invisible
	joinErrors := map[string]string{
		"403": "no such channel",
		"405": "too many channels",
		"471": "channel is full (+l)",
		"473": "invite-only channel (+i)",
		"474": "banned from channel (+b)",
		"475": "bad channel key (+k)",
		"476": "bad channel mask",
		"477": "channel requires a registered nick (+r)",
	}
	for code, reason := range joinErrors {
		reason := reason
		b.conn.AddCallback(code, func(e *irc.Event) {
			ch := ""
			if len(e.Arguments) >= 2 {
				ch = e.Arguments[1]
			}
			b.log.Warn("cannot join channel", "channel", ch, "reason", reason, "code", e.Code, "detail", e.Message())
		})
	}
}

type slogWriter struct{ log *slog.Logger }

func (w slogWriter) Write(p []byte) (int, error) {
	w.log.Warn("irc", "msg", strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func (b *Bot) joinChannels() {
	if !b.joined.CompareAndSwap(false, true) {
		return
	}
	// Confirm the host we're using matches what's expected
	b.conn.Whois(b.conn.GetNick())
	for _, ch := range b.cfg.Channels {
		b.log.Info("joining", "channel", ch)
		b.conn.Join(ch)
	}
}

// awaitVHost logs an error if the 396 host-hidden confirmation has not arrived
// within the configured timeout. a late 396 will still trigger the join.
func (b *Bot) awaitVHost() {
	d := b.cfg.VHostTimeout()
	time.AfterFunc(d, func() {
		if !b.joined.Load() {
			b.log.Error("vhost not confirmed (no 396 received); not joining channels to avoid leaking hostname",
				"waited", d, "service", b.cfg.VHost.Service,
				"hint", "if authenticating with sasl the vhost is applied at login and no 396 is sent; ensure sasl is enabled or disable the vhost gate")
		}
	})
}

func cloneMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func channelNames(entries []string) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, firstToken(e)) // strip join keys. just #channel
	}
	return names
}

func firstToken(s string) string {
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i]
	}
	return s
}

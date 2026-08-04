// Package bottest runs a bot against a fake ircd, so a plugin's tests can feed
// it lines and read back what it said.
package bottest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/config"
)

// nick is who the bot is, on the fake network.
const nick = "ohayoubot"

// quiet is how long to wait for a line that should not arrive, and for the
// lull that means a burst of them has finished.
const quiet = 250 * time.Millisecond

// Whois is what the fake server says about a nick when asked. A nick with no
// entry comes back as "no such nick", and an empty Account is a nick that is
// online but not logged in to services.
type Whois struct {
	Account  string
	Channels string
}

// Harness is a bot wired to a fake server.
type Harness struct {
	// Bot is connected and in the channels it was given.
	Bot *bot.Bot
	// Log is the bot's logger, discarded by default.
	Log *slog.Logger

	t     *testing.T
	lines chan string

	mu     sync.Mutex
	conn   net.Conn
	whois  map[string]Whois
	asked  []string
	silent bool
}

// Option configures a harness before it connects.
type Option func(*config.Config)

// InChannels puts the bot in the given channels rather than the default #chan.
func InChannels(names ...string) Option {
	return func(c *config.Config) { c.Channels = names }
}

// WithAdmin makes nick an admin from host.
func WithAdmin(nick, host string) Option {
	return func(c *config.Config) { c.Admins[strings.ToLower(nick)] = host }
}

// Ignoring puts a nick on the bot's ignore list.
func Ignoring(nick string) Option {
	return func(c *config.Config) { c.IgnoreList[strings.ToLower(nick)] = "test" }
}

// New starts a fake server, connects a bot to it and waits until it has
// registered. Everything is torn down when the test ends.
func New(t *testing.T, opts ...Option) *Harness {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	h := &Harness{
		t:     t,
		lines: make(chan string, 256),
		whois: map[string]Whois{},
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	cfg := &config.Config{
		Nick: nick, User: nick,
		Server: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port,
		CommandPrefix: "!",
		Channels:      []string{"#chan"},
		Admins:        map[string]string{},
		IgnoreList:    map[string]string{},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	ready := make(chan struct{})
	go h.serve(ln, ready)

	h.Bot = bot.New(cfg, h.Log)
	return h
}

// Start connects the bot. It is separate from New so a caller can register
// plugins against Harness.Bot first, the way the registry does.
func (h *Harness) Start() {
	h.t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.Bot.Run(ctx) }()
	h.t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	deadline := time.After(5 * time.Second)
	for {
		h.mu.Lock()
		up := h.conn != nil
		h.mu.Unlock()
		if up {
			return
		}
		select {
		case <-deadline:
			h.t.Fatal("the bot never registered with the fake server")
		case <-time.After(time.Millisecond):
		}
	}
}

func (h *Harness) serve(ln net.Listener, ready chan struct{}) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	welcomed := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		switch {
		case strings.HasPrefix(line, "NICK") && !welcomed:
			welcomed = true
			h.mu.Lock()
			h.conn = conn
			h.mu.Unlock()
			io.WriteString(conn, ":srv 001 "+nick+" :Welcome\r\n")
			close(ready)
		case strings.HasPrefix(line, "WHOIS "):
			h.answerWhois(conn, strings.TrimSpace(strings.TrimPrefix(line, "WHOIS ")))
		case strings.HasPrefix(line, "PRIVMSG"), strings.HasPrefix(line, "NOTICE"):
			h.lines <- line
		case strings.HasPrefix(line, "QUIT"):
			// Hang up, or the bot's read loop waits on a server that is
			// waiting on it and shutdown costs every test a timeout.
			return
		}
	}
}

func (h *Harness) answerWhois(conn net.Conn, who string) {
	h.mu.Lock()
	reply, known := h.whois[strings.ToLower(who)]
	silent := h.silent
	// The bot whoises itself on connect to log its own host. That is not a
	// lookup any command asked for, so it does not count.
	if !strings.EqualFold(who, nick) {
		h.asked = append(h.asked, who)
	}
	h.mu.Unlock()

	if silent {
		return
	}
	if !known {
		fmt.Fprintf(conn, ":srv 401 %s %s :No such nick/channel\r\n", nick, who)
		return
	}
	if reply.Account != "" {
		fmt.Fprintf(conn, ":srv 330 %s %s %s :is logged in as\r\n", nick, who, reply.Account)
	}
	if reply.Channels != "" {
		fmt.Fprintf(conn, ":srv 319 %s %s :%s\r\n", nick, who, reply.Channels)
	}
	fmt.Fprintf(conn, ":srv 318 %s %s :End of /WHOIS list\r\n", nick, who)
}

// Says tells the fake server what to answer when a nick is looked up.
func (h *Harness) Says(who string, reply Whois) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.whois[strings.ToLower(who)] = reply
}

// NeverAnswers makes every lookup hang, for testing what happens when the
// server does not reply.
func (h *Harness) NeverAnswers() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.silent = true
}

// WhoisCount is how many lookups the bot has made, not counting the one it
// makes about itself on connect.
func (h *Harness) WhoisCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.asked)
}

// Send writes a raw line to the bot as if the server had sent it.
func (h *Harness) Send(format string, args ...any) {
	h.t.Helper()

	h.mu.Lock()
	conn := h.conn
	h.mu.Unlock()
	if conn == nil {
		h.t.Fatal("the fake server has no connection yet; call Start first")
	}
	fmt.Fprintf(conn, format+"\r\n", args...)
}

// Say puts a line in a channel, or in private when target is the bot's nick.
func (h *Harness) Say(who, target, text string) {
	h.t.Helper()
	h.Send(":%s!%s@example.net PRIVMSG %s :%s", who, who, target, text)
}

// Next waits for one line from the bot.
func (h *Harness) Next() string {
	h.t.Helper()
	select {
	case line := <-h.lines:
		return line
	case <-time.After(5 * time.Second):
		h.t.Fatal("timed out waiting for the bot to say something")
		return ""
	}
}

// Collect waits for the first line and then for the rest to stop arriving.
func (h *Harness) Collect() []string {
	h.t.Helper()
	return append([]string{h.Next()}, h.Drain()...)
}

// Drain takes whatever has arrived once a lull says the bot has finished. It
// is happy with nothing.
func (h *Harness) Drain() []string {
	h.t.Helper()
	return h.DrainFor(quiet)
}

// DrainFor is Drain given longer to wait for the first line, for work the bot
// only gets to on a timer. Once one arrives the usual lull decides the end.
func (h *Harness) DrainFor(first time.Duration) []string {
	h.t.Helper()

	var out []string
	deadline := time.After(first)
	for {
		select {
		case line := <-h.lines:
			out = append(out, line)
			deadline = time.After(quiet)
		case <-deadline:
			return out
		}
	}
}

// Silent fails if the bot says anything at all.
func (h *Harness) Silent() {
	h.t.Helper()
	if extra := h.Drain(); len(extra) > 0 {
		h.t.Fatalf("expected silence, got %q", extra)
	}
}

// Said reports whether any of lines contains want.
func Said(lines []string, want string) bool {
	for _, line := range lines {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}

package youtube

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/bot/irctext"
	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
)

// testDeps is what the registry would hand the plugin.
func testDeps(b *bot.Bot) plugin.Deps {
	return plugin.Deps{Bot: b, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

const testID = "dQw4w9WgXcQ"

// harness runs the plugin against a fake irc server and a fake oembed endpoint,
// so a test can put a line in a channel and read what comes back out.
type harness struct {
	plugin *Plugin
	lines  chan string

	mu     sync.Mutex
	server net.Conn
	asked  []string         // the video ids oembed was asked about
	videos map[string]video // what oembed knows
	status map[string]int   // ids that answer with an error instead
	clock  time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	h := &harness{
		lines:  make(chan string, 64),
		videos: map[string]video{},
		status: map[string]int{},
		clock:  time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
	}
	h.videos[testID] = video{
		Title:  "Rick Astley - Never Gonna Give You Up",
		Author: "Rick Astley",
	}

	oembed := httptest.NewServer(http.HandlerFunc(h.oembed))
	t.Cleanup(oembed.Close)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	ready := make(chan struct{})
	go h.serve(ln, ready)

	botCfg := &config.Config{
		Nick: "ohayoubot", User: "ohayoubot",
		Server: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port,
		CommandPrefix: "!",
		Channels:      []string{"#chan"},
		Admins:        map[string]string{},
		IgnoreList:    map[string]string{"spammer": "posts too much"},
	}
	b := bot.New(botCfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(250 * time.Millisecond):
		}
	})

	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("the bot never registered with the fake server")
	}

	h.plugin = New()
	h.plugin.now = h.now
	if _, err := h.plugin.Configure(plugin.Config{Block: json.RawMessage(`{
		"requestTimeout": 2000,
		"ignoreChannels": ["#quiet"]
	}`)}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := h.plugin.Register(testDeps(b)); err != nil {
		t.Fatalf("register: %v", err)
	}
	h.plugin.api = newClient(oembed.URL, 2*time.Second)

	return h
}

func (h *harness) now() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.clock
}

// advance moves the plugin's clock forward.
func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clock = h.clock.Add(d)
}

// knows teaches the fake endpoint about a video.
func (h *harness) knows(id string, v video) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.videos[id] = v
}

// fails makes the fake endpoint answer for an id with a status instead.
func (h *harness) fails(id string, code int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status[id] = code
}

// oembed answers as youtube's endpoint does, for the videos the test set up.
func (h *harness) oembed(w http.ResponseWriter, r *http.Request) {
	target, _ := url.Parse(r.URL.Query().Get("url"))
	id := ""
	if target != nil {
		id = target.Query().Get("v")
	}

	h.mu.Lock()
	h.asked = append(h.asked, id)
	v, known := h.videos[id]
	code, failing := h.status[id]
	h.mu.Unlock()

	switch {
	case failing:
		w.WriteHeader(code)
	case !known:
		w.WriteHeader(http.StatusNotFound)
	default:
		json.NewEncoder(w).Encode(v)
	}
}

// serve is the fake irc server: it welcomes the bot, keeps the connection so
// the test can push lines down it, and collects everything the bot says.
func (h *harness) serve(ln net.Listener, ready chan struct{}) {
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
			h.server = conn
			h.mu.Unlock()
			io.WriteString(conn, ":srv 001 ohayoubot :Welcome\r\n")
			close(ready)
		case strings.HasPrefix(line, "PRIVMSG"), strings.HasPrefix(line, "NOTICE"):
			h.lines <- line
		}
	}
}

// say puts a line in a channel as nick.
func (h *harness) say(t *testing.T, nick, target, text string) {
	t.Helper()
	h.mu.Lock()
	conn := h.server
	h.mu.Unlock()
	if conn == nil {
		t.Fatal("the fake server has no connection yet")
	}
	fmt.Fprintf(conn, ":%s!%s@example.net PRIVMSG %s :%s\r\n", nick, nick, target, text)
}

// next waits for a line from the bot.
func (h *harness) next(t *testing.T) string {
	t.Helper()
	select {
	case line := <-h.lines:
		return line
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the bot to say something")
		return ""
	}
}

// silent fails if the bot says anything in the next moment.
func (h *harness) silent(t *testing.T) {
	t.Helper()
	select {
	case line := <-h.lines:
		t.Fatalf("expected silence, got %q", line)
	case <-time.After(300 * time.Millisecond):
	}
}

func (h *harness) askedFor() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.asked...)
}

func TestPreviewsALink(t *testing.T) {
	h := newHarness(t)

	h.say(t, "someone", "#chan", "have you seen https://youtu.be/"+testID+" yet")

	want := "PRIVMSG #chan :YouTube: Rick Astley - Never Gonna Give You Up (Rick Astley)"
	if got := h.next(t); got != want {
		t.Errorf("bot said %q, want %q", got, want)
	}
}

func TestPreviewsInAPrivateMessage(t *testing.T) {
	h := newHarness(t)

	h.say(t, "someone", "ohayoubot", "https://www.youtube.com/watch?v="+testID)

	if got := h.next(t); !strings.HasPrefix(got, "PRIVMSG someone :YouTube: ") {
		t.Errorf("bot said %q, want a preview back to the sender", got)
	}
}

func TestIgnoresLinesWithoutAVideo(t *testing.T) {
	h := newHarness(t)

	h.say(t, "someone", "#chan", "https://example.com/watch?v="+testID+" is not youtube")
	h.silent(t)

	if asked := h.askedFor(); len(asked) != 0 {
		t.Errorf("oembed was asked about %v, want nothing", asked)
	}
}

func TestSaysNothingAboutAMissingVideo(t *testing.T) {
	h := newHarness(t)

	h.say(t, "someone", "#chan", "https://youtu.be/oHg5SJYRHA0")
	h.silent(t)

	if asked := h.askedFor(); len(asked) != 1 || asked[0] != "oHg5SJYRHA0" {
		t.Errorf("oembed was asked about %v, want the one id", asked)
	}
}

func TestCooldownHoldsTheChannel(t *testing.T) {
	h := newHarness(t)

	h.say(t, "someone", "#chan", "https://youtu.be/"+testID)
	h.next(t)

	// A different video, still inside the channel's cooldown.
	h.knows("oHg5SJYRHA0", video{Title: "Another one", Author: "Someone"})
	h.say(t, "someone", "#chan", "https://youtu.be/oHg5SJYRHA0")
	h.silent(t)

	h.advance(11 * time.Second)
	h.say(t, "someone", "#chan", "https://youtu.be/oHg5SJYRHA0")
	if got := h.next(t); !strings.Contains(got, "Another one") {
		t.Errorf("bot said %q, want the second video once the cooldown passed", got)
	}
}

func TestDoesNotRepeatTheSameVideo(t *testing.T) {
	h := newHarness(t)

	h.say(t, "someone", "#chan", "https://youtu.be/"+testID)
	h.next(t)

	// Past the cooldown, but well inside the repeat window.
	h.advance(time.Minute)
	h.say(t, "friend", "#chan", "https://www.youtube.com/watch?v="+testID)
	h.silent(t)

	h.advance(10 * time.Minute)
	h.say(t, "friend", "#chan", "https://www.youtube.com/watch?v="+testID)
	if got := h.next(t); !strings.Contains(got, "Never Gonna Give You Up") {
		t.Errorf("bot said %q, want the video again once the repeat window passed", got)
	}
}

func TestNamesEveryVideoInOneLineUpToMax(t *testing.T) {
	h := newHarness(t)
	h.knows("oHg5SJYRHA0", video{Title: "Another one", Author: "Someone"})
	h.knows("QH2-TGUlwu4", video{Title: "A third", Author: "Someone"})

	h.say(t, "someone", "#chan", "https://youtu.be/"+testID+
		" https://youtu.be/oHg5SJYRHA0 https://youtu.be/QH2-TGUlwu4")

	if got := h.next(t); !strings.Contains(got, "Never Gonna Give You Up") {
		t.Errorf("first line was %q", got)
	}
	if got := h.next(t); !strings.Contains(got, "Another one") {
		t.Errorf("second line was %q", got)
	}
	h.silent(t)
}

func TestIgnoredChannelStaysQuiet(t *testing.T) {
	h := newHarness(t)

	h.say(t, "someone", "#quiet", "https://youtu.be/"+testID)
	h.silent(t)
}

func TestIgnoredNickGetsNoPreview(t *testing.T) {
	h := newHarness(t)

	h.say(t, "spammer", "#chan", "https://youtu.be/"+testID)
	h.silent(t)
}

func TestCommandsDoNotReachTheWatcher(t *testing.T) {
	h := newHarness(t)

	// !code is a real command, and it answers with its own line.
	h.say(t, "someone", "#chan", "!code https://youtu.be/"+testID)
	if got := h.next(t); strings.Contains(got, "YouTube:") {
		t.Errorf("bot previewed a link inside a command: %q", got)
	}
	h.silent(t)
}

func TestOembedErrorSaysNothing(t *testing.T) {
	h := newHarness(t)
	h.fails(testID, http.StatusInternalServerError)

	h.say(t, "someone", "#chan", "https://youtu.be/"+testID)
	h.silent(t)
}

func TestLineIsCleanedAndFitted(t *testing.T) {
	v := video{Title: "a\r\nPRIVMSG #chan :fake", Author: "  some\tone  "}
	if got, want := line("#chan", v), "YouTube: a PRIVMSG #chan :fake (some one)"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}

	long := video{Title: strings.Repeat("\u304b", 400)}
	got := line("#chan", long)
	if n := len("PRIVMSG #chan :" + got + "\r\n"); n > irctext.LineLimit {
		t.Errorf("line is %d bytes, over the %d limit", n, irctext.LineLimit)
	}
}

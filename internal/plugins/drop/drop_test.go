package drop

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
	"github.com/ohayoubot/ohayou-bot/internal/store/sqlite"
)

const testSecret = "0123456789abcdef0123456789abcdef"

// whoisReply is what the fake server says about a nick.
type whoisReply struct {
	account  string
	channels string
}

type harness struct {
	plugin *Plugin
	lines  chan string

	mu     sync.Mutex
	whois  map[string]whoisReply
	asked  []string
	silent bool // when set, a WHOIS gets no reply at all
}

// testDeps is what the registry would hand the plugin.
func testDeps(b *bot.Bot) plugin.Deps {
	return plugin.Deps{Bot: b, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessIn(t, []string{"#chan", "#Other"})
}

// newHarnessIn builds a bot already in the given channels, which is what
// shared() intersects a nick's whois against.
func newHarnessIn(t *testing.T, channels []string) *harness {
	t.Helper()

	h := &harness{
		lines: make(chan string, 256),
		whois: map[string]whoisReply{},
	}

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
		Channels:      channels,
		Admins:        map[string]string{},
		IgnoreList:    map[string]string{},
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

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Init(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}

	t.Setenv("OHAYOU_DROP_SECRET", testSecret)
	h.plugin = New()
	if _, err := h.plugin.Configure(plugin.Config{
		Block: json.RawMessage(`{
			"url": "https://hemera.day/drop/",
			"imageBase": "https://img.hemera.day"
		}`),
		Cloudflare: config.Cloudflare{AccountID: "acct", DatabaseID: "db", APIToken: "token"},
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	deps := testDeps(b)
	deps.Store = db
	if err := h.plugin.Register(deps); err != nil {
		t.Fatalf("register: %v", err)
	}

	return h
}

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
			io.WriteString(conn, ":srv 001 ohayoubot :Welcome\r\n")
			close(ready)
		case strings.HasPrefix(line, "WHOIS "):
			h.answerWhois(conn, strings.TrimSpace(strings.TrimPrefix(line, "WHOIS ")))
		case strings.HasPrefix(line, "PRIVMSG"), strings.HasPrefix(line, "NOTICE"):
			h.lines <- line
		}
	}
}

func (h *harness) answerWhois(conn net.Conn, nick string) {
	h.mu.Lock()
	reply, known := h.whois[strings.ToLower(nick)]
	silent := h.silent
	// The bot whoises itself on connect to log its own host. That is not a
	// lookup any command asked for, so it does not count.
	if !strings.EqualFold(nick, "ohayoubot") {
		h.asked = append(h.asked, nick)
	}
	h.mu.Unlock()

	if silent {
		return
	}
	if !known {
		fmt.Fprintf(conn, ":srv 401 ohayoubot %s :No such nick/channel\r\n", nick)
		return
	}
	if reply.account != "" {
		fmt.Fprintf(conn, ":srv 330 ohayoubot %s %s :is logged in as\r\n", nick, reply.account)
	}
	if reply.channels != "" {
		fmt.Fprintf(conn, ":srv 319 ohayoubot %s :%s\r\n", nick, reply.channels)
	}
	fmt.Fprintf(conn, ":srv 318 ohayoubot %s :End of /WHOIS list\r\n", nick)
}

func (h *harness) says(nick string, reply whoisReply) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.whois[strings.ToLower(nick)] = reply
}

func (h *harness) whoisCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.asked)
}

func (h *harness) drain(first time.Duration) []string {
	var out []string
	timeout := time.After(first)
	for {
		select {
		case line := <-h.lines:
			out = append(out, line)
			timeout = time.After(150 * time.Millisecond)
		case <-timeout:
			return out
		}
	}
}

func (h *harness) collect(t *testing.T) []string {
	t.Helper()
	return h.drain(3 * time.Second)
}

func message(nick, target, text string) *bot.Message {
	fields := strings.Fields(text)
	return &bot.Message{
		Prefix: "!", Command: strings.TrimPrefix(fields[0], "!"),
		Args: fields, Target: target, Nick: nick, Host: "example.host",
	}
}

// linkIn returns the grant out of whichever line carries the upload url.
func linkIn(t *testing.T, lines []string) (target, token string) {
	t.Helper()
	for _, line := range lines {
		if i := strings.Index(line, "https://hemera.day/drop/#"); i >= 0 {
			return strings.Fields(line)[1], line[i+len("https://hemera.day/drop/#"):]
		}
	}
	t.Fatalf("no upload link in %v", lines)
	return "", ""
}

func payloadOf(t *testing.T, token string) grant {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q has %d parts", token, len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decoding payload: %v", err)
	}
	var g grant
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	return g
}

func TestUploadSendsTheLinkPrivately(t *testing.T) {
	h := newHarness(t)
	h.says("mallow", whoisReply{account: "Mallow", channels: "@#chan +#other"})

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	lines := h.collect(t)

	target, token := linkIn(t, lines)
	if target != "mallow" {
		t.Errorf("the link went to %q, want a private message to mallow", target)
	}

	// Nothing said in the channel may carry the grant.
	for _, line := range lines {
		if strings.HasPrefix(strings.Fields(line)[1], "#") && strings.Contains(line, token) {
			t.Fatalf("the grant was said in a channel: %s", line)
		}
	}

	var pointed bool
	for _, line := range lines {
		if strings.HasPrefix(line, "PRIVMSG #chan") && strings.Contains(line, "PMs") {
			pointed = true
		}
	}
	if !pointed {
		t.Errorf("the channel was not told to look at their PMs: %v", lines)
	}
}

func TestUploadGrantCarriesAccountAndSharedChannels(t *testing.T) {
	h := newHarness(t)
	h.says("mallow", whoisReply{account: "Mallow", channels: "@#chan +#other #elsewhere"})

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	_, token := linkIn(t, h.collect(t))

	g := payloadOf(t, token)
	if g.A != "Mallow" {
		t.Errorf("account = %q, want Mallow", g.A)
	}
	if g.N != "mallow" {
		t.Errorf("nick = %q, want mallow", g.N)
	}
	// #elsewhere is not somewhere the bot is, so it cannot be a destination.
	// #Other keeps the bot's spelling, not the one whois echoed.
	if len(g.C) != 2 || g.C[0] != "#chan" || g.C[1] != "#Other" {
		t.Errorf("channels = %v, want [#chan #Other]", g.C)
	}
	if g.E <= time.Now().Unix() {
		t.Errorf("expiry %d is not in the future", g.E)
	}
}

func TestUploadRefusesAnUnidentifiedNick(t *testing.T) {
	h := newHarness(t)
	h.says("mallow", whoisReply{channels: "#chan"})

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	lines := h.collect(t)

	for _, line := range lines {
		if strings.Contains(line, "hemera.day/drop/#") {
			t.Fatalf("an unidentified nick got a link: %s", line)
		}
	}
	if !said(lines, "identified") {
		t.Errorf("no explanation given: %v", lines)
	}
}

func TestUploadRefusesWhenNoChannelIsShared(t *testing.T) {
	h := newHarness(t)
	h.says("mallow", whoisReply{account: "Mallow", channels: "#elsewhere"})

	h.plugin.cmdUpload(message("mallow", "mallow", "!upload"))
	lines := h.collect(t)

	for _, line := range lines {
		if strings.Contains(line, "hemera.day/drop/#") {
			t.Fatalf("got a link with nowhere to post: %s", line)
		}
	}
	if !said(lines, "channel together") {
		t.Errorf("no explanation given: %v", lines)
	}
}

// A lookup that fails is not a "no": the nick may well be identified.
func TestUploadRefusesWhenTheLookupFails(t *testing.T) {
	h := newHarness(t)
	h.mu.Lock()
	h.silent = true
	h.mu.Unlock()

	done := make(chan struct{})
	go func() {
		h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the command never gave up on the whois")
	}

	lines := h.drain(time.Second)
	for _, line := range lines {
		if strings.Contains(line, "hemera.day/drop/#") {
			t.Fatalf("a failed lookup produced a link: %s", line)
		}
	}
	if !said(lines, "couldn't check") {
		t.Errorf("no explanation given: %v", lines)
	}
}

func TestUploadCooldownStopsRepeatWhois(t *testing.T) {
	h := newHarness(t)
	h.says("mallow", whoisReply{account: "Mallow", channels: "#chan"})

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	h.collect(t)
	if n := h.whoisCount(); n != 1 {
		t.Fatalf("%d whois for the first link, want 1", n)
	}

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	lines := h.collect(t)

	if n := h.whoisCount(); n != 1 {
		t.Errorf("%d whois after a repeat, want the second to be refused before asking", n)
	}
	if !said(lines, "Try again in") {
		t.Errorf("no cooldown message: %v", lines)
	}
}

func TestUploadCooldownExpires(t *testing.T) {
	h := newHarness(t)
	h.says("mallow", whoisReply{account: "Mallow", channels: "#chan"})

	now := time.Now()
	h.plugin.now = func() time.Time { return now }

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	h.collect(t)

	now = now.Add(61 * time.Second)
	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	h.collect(t)

	if n := h.whoisCount(); n != 2 {
		t.Errorf("%d whois, want 2 once the cooldown has run out", n)
	}
}

func TestUploadInPrivateSaysNothingInChannel(t *testing.T) {
	h := newHarness(t)
	h.says("mallow", whoisReply{account: "Mallow", channels: "#chan"})

	h.plugin.cmdUpload(message("mallow", "mallow", "!upload"))
	lines := h.collect(t)

	for _, line := range lines {
		if strings.HasPrefix(strings.Fields(line)[1], "#") {
			t.Errorf("a private request was answered in a channel: %s", line)
		}
	}
}

func TestSharedCapsTheChannelList(t *testing.T) {
	var all []string
	for i := range maxChannels + 5 {
		all = append(all, fmt.Sprintf("#c%d", i))
	}
	h := newHarnessIn(t, all)

	if got := h.plugin.shared(all); len(got) != maxChannels {
		t.Errorf("kept %d channels, want the worker's ceiling of %d", len(got), maxChannels)
	}
}

func TestSharedIgnoresRepeatsAndStrangers(t *testing.T) {
	h := newHarness(t)

	got := h.plugin.shared([]string{"#chan", "#CHAN", "#nowhere", "#other"})
	if len(got) != 2 || got[0] != "#chan" || got[1] != "#Other" {
		t.Errorf("shared = %v, want [#chan #Other]", got)
	}
}

func said(lines []string, want string) bool {
	for _, line := range lines {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}

// announceHarness swaps the plugin's queue for a fake and hands back both.
func announceHarness(t *testing.T) (*harness, *fakeQueue) {
	t.Helper()
	h := newHarness(t)
	fake := &fakeQueue{}
	h.plugin.db = fake.start(t)
	return h, fake
}

func TestAnnouncePostsUploadsInOrder(t *testing.T) {
	h, fake := announceHarness(t)
	fake.set(
		map[string]any{"id": 1, "nick": "mallow", "channel": "#chan", "key": "abc.png"},
		map[string]any{"id": 2, "nick": "svaj", "channel": "#Other", "key": "def.gif"},
	)

	if got := h.plugin.announce(context.Background(), 0); got != 2 {
		t.Errorf("cursor = %d, want 2", got)
	}

	lines := h.collect(t)
	if len(lines) != 2 {
		t.Fatalf("said %d lines, want 2: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "PRIVMSG #chan :mallow uploaded: https://img.hemera.day/abc.png") {
		t.Errorf("first line = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "PRIVMSG #Other :svaj uploaded: https://img.hemera.day/def.gif") {
		t.Errorf("second line = %q", lines[1])
	}
}

func TestAnnounceSkipsChannelsTheBotHasLeft(t *testing.T) {
	h, fake := announceHarness(t)
	fake.set(
		map[string]any{"id": 5, "nick": "mallow", "channel": "#gone", "key": "abc.png"},
		map[string]any{"id": 6, "nick": "svaj", "channel": "#chan", "key": "def.png"},
	)

	// The skipped row still advances the cursor, or it would be retried forever.
	if got := h.plugin.announce(context.Background(), 0); got != 6 {
		t.Errorf("cursor = %d, want 6", got)
	}

	lines := h.collect(t)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PRIVMSG #chan :svaj") {
		t.Fatalf("said %v, want only the #chan line", lines)
	}
}

func TestAnnounceHoldsTheCursorWhenD1Fails(t *testing.T) {
	h, fake := announceHarness(t)
	fake.mu.Lock()
	fake.status = http.StatusUnauthorized
	fake.mu.Unlock()

	if got := h.plugin.announce(context.Background(), 4); got != 4 {
		t.Errorf("cursor = %d, want it left at 4 so nothing is skipped", got)
	}
}

func TestAnnounceSavesTheCursor(t *testing.T) {
	h, fake := announceHarness(t)
	fake.set(map[string]any{"id": 9, "nick": "mallow", "channel": "#chan", "key": "a.png"})

	h.plugin.announce(context.Background(), 0)
	h.collect(t)

	got, err := h.plugin.store.GetKV(context.Background(), cursorKey)
	if err != nil || got != "9" {
		t.Errorf("stored cursor = %q, %v; want 9", got, err)
	}
}

// Switching the plugin on against a database that already holds uploads must
// not recite them all into the channels.
func TestStartAtBeginsAtTheEndOfTheQueue(t *testing.T) {
	h, fake := announceHarness(t)
	fake.newest = 120

	got, err := h.plugin.startAt(context.Background())
	if err != nil {
		t.Fatalf("startAt: %v", err)
	}
	if got != 120 {
		t.Errorf("startAt = %d, want 120", got)
	}

	saved, err := h.plugin.store.GetKV(context.Background(), cursorKey)
	if err != nil || saved != "120" {
		t.Errorf("stored cursor = %q, %v; want 120", saved, err)
	}
}

func TestStartAtResumesFromTheStoredCursor(t *testing.T) {
	h, fake := announceHarness(t)
	fake.newest = 999
	if err := h.plugin.store.SetKV(context.Background(), cursorKey, "12"); err != nil {
		t.Fatal(err)
	}

	got, err := h.plugin.startAt(context.Background())
	if err != nil {
		t.Fatalf("startAt: %v", err)
	}
	if got != 12 {
		t.Errorf("startAt = %d, want the stored 12, not the newest id", got)
	}
}

func TestPollStopsWithTheContext(t *testing.T) {
	h, fake := announceHarness(t)
	fake.newest = 3

	ctx, cancel := context.WithCancel(context.Background())
	h.plugin.Start(ctx)

	done := make(chan struct{})
	go func() {
		h.plugin.bot.Wait()
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the poller did not stop when the context was cancelled")
	}
}

// A poller that cannot find its starting point must keep trying: the upload
// table may not exist yet, which is exactly the state a first deploy is in.
func TestPollRetriesAFailedStart(t *testing.T) {
	h, fake := announceHarness(t)
	h.plugin.cfg.PollSeconds = 1

	fake.mu.Lock()
	fake.status = http.StatusInternalServerError
	fake.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h.plugin.Start(ctx)

	// Two ticks with d1 unreachable. The old behaviour returned here for good.
	time.Sleep(2500 * time.Millisecond)
	if lines := h.drain(200 * time.Millisecond); len(lines) != 0 {
		t.Fatalf("said something while d1 was down: %v", lines)
	}

	fake.set(map[string]any{"id": 1, "nick": "whatapath", "channel": "#chan", "key": "a.png"})
	fake.mu.Lock()
	fake.status = 0
	fake.mu.Unlock()

	lines := h.drain(5 * time.Second)
	if len(lines) != 1 || !strings.Contains(lines[0], "whatapath uploaded: https://img.hemera.day/a.png") {
		t.Fatalf("did not recover once d1 came back: %v", lines)
	}

	cancel()
	h.plugin.bot.Wait()
}

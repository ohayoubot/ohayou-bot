package deerkins

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/bot/irctext"
	"github.com/ohayoubot/ohayou-bot/internal/bot/ratelimit"
	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
	"github.com/ohayoubot/ohayou-bot/internal/store/sqlite"
)

// testDeps is what the registry would hand the plugin.
func testDeps(b *bot.Bot) plugin.Deps {
	return plugin.Deps{Bot: b, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

const senordeer = "        GGGG\n      GGGGGGGG\n         AA\n        AAA\n         AA\n  AGDGDGAAA\n AAGGDGGAAA\n AAAAAAAAAA\n A A    A A\n A A    A A\n A A    A A"

// fakeD1 answers query calls the way the cloudflare API does. It records the
// statements it was asked to run so the tests can assert on the binding.
type fakeD1 struct {
	mu      sync.Mutex
	calls   []d1Call
	rows    []map[string]any
	status  int
	failure string
}

type d1Call struct {
	SQL    string   `json:"sql"`
	Params []string `json:"params"`
}

func (f *fakeD1) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("Authorization = %q", got)
		}
		var call d1Call
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			t.Errorf("decode request: %v", err)
		}
		f.mu.Lock()
		f.calls = append(f.calls, call)
		rows, status, failure := f.rows, f.status, f.failure
		f.mu.Unlock()

		if status != 0 {
			w.WriteHeader(status)
			io.WriteString(w, `{"success":false}`)
			return
		}
		body := map[string]any{"success": true, "result": []any{map[string]any{"results": rows}}}
		if failure != "" {
			body = map[string]any{"success": false,
				"errors": []any{map[string]any{"code": 7500, "message": failure}}}
		}
		json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeD1) statements() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = c.SQL
	}
	return out
}

// testConfig goes through Configure so the tests run against the same defaults
// a deployment gets.
func testConfig(t *testing.T) Config {
	t.Helper()

	p := New()
	on, err := p.Configure(plugin.Config{Cloudflare: config.Cloudflare{
		AccountID: "account", DatabaseID: "database", APIToken: "token",
	}})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	if !on {
		t.Fatal("deerkins did not come up from a complete config")
	}
	return p.cfg
}

// harness wires the plugin to a bot talking to a fake irc server and a fake D1,
// and hands back everything the bot writes to the network.
type harness struct {
	plugin *Plugin
	d1     *fakeD1
	lines  chan string
}

func newHarness(t *testing.T, cfg Config) *harness {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	lines := make(chan string, 256)
	ready := make(chan struct{})
	go func() {
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
			if strings.HasPrefix(line, "NICK") && !welcomed {
				welcomed = true
				io.WriteString(conn, ":srv 001 ohayoubot :Welcome\r\n")
				close(ready)
			}
			if strings.HasPrefix(line, "PRIVMSG") || strings.HasPrefix(line, "NOTICE") {
				lines <- line
			}
		}
	}()

	botCfg := &config.Config{
		Nick: "ohayoubot", User: "ohayoubot",
		Server: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port,
		CommandPrefix: "!",
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

	fake := &fakeD1{}
	srv := fake.start(t)

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Init(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}

	p := New()
	p.cfg = cfg
	p.roll = func(int) int { return 0 }
	deps := testDeps(b)
	deps.Store = db
	if err := p.Register(deps.For("deerkins")); err != nil {
		t.Fatalf("register: %v", err)
	}
	p.db = newGallery(srv.URL, cfg.AccountID, cfg.DatabaseID, cfg.APIToken, cfg.RequestTimeout())

	return &harness{plugin: p, d1: fake, lines: lines}
}

// collect drains the lines the bot sent, waiting for the first one and then
// for the rest to stop arriving.
func (h *harness) collect(t *testing.T) []string {
	t.Helper()
	return h.drain(2 * time.Second)
}

// silence returns whatever the bot said when it should have said nothing.
func (h *harness) silence(t *testing.T) []string {
	t.Helper()
	return h.drain(300 * time.Millisecond)
}

// forget clears the gate on the replies that aren't art, standing in for the
// wait between them.
func (h *harness) forget() {
	h.plugin.spoke = ratelimit.New(chatterWait)
}

func (h *harness) drain(first time.Duration) []string {
	var out []string
	timeout := time.After(first)
	for {
		select {
		case line := <-h.lines:
			out = append(out, line)
			timeout = time.After(100 * time.Millisecond)
		case <-timeout:
			return out
		}
	}
}

func message(nick, target, text string) *bot.Message {
	fields := strings.Fields(text)
	return &bot.Message{
		Prefix: "!", Command: strings.TrimPrefix(fields[0], "!"),
		Args: fields, Target: target, Nick: nick, Host: "example.host",
	}
}

func TestDeermePaintsArtAndCredits(t *testing.T) {
	h := newHarness(t, testConfig(t))
	h.d1.rows = []map[string]any{{
		"deer": "senordeer", "creator": "svaj", "date": "2008-11-23 03:18:07", "kinskode": senordeer,
	}}

	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme senordeer"))
	lines := h.collect(t)

	if len(lines) != 11 {
		t.Fatalf("got %d lines, want the 11 rows of art:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "PRIVMSG #pank :") {
			t.Errorf("line went somewhere odd: %q", line)
		}
		if len(line)+2 > irctext.LineLimit {
			t.Errorf("line is %d bytes, over the protocol limit", len(line)+2)
		}
	}
	if !strings.Contains(lines[0], "\x0301,01@") {
		t.Errorf("first line is not painted: %q", lines[0])
	}

	sql := h.d1.statements()
	if len(sql) != 1 || !strings.Contains(sql[0], "WHERE deer = ?1") {
		t.Errorf("statements = %q, want a single bound lookup", sql)
	}
	if got := h.d1.calls[0].Params; len(got) != 1 || got[0] != "senordeer" {
		t.Errorf("params = %q", got)
	}
}

func TestDeermeCreditsRandomAndLatest(t *testing.T) {
	for _, tt := range []struct{ name, want string }{
		{"random", "ORDER BY RANDOM()"},
		{"latest", "ORDER BY date DESC"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, testConfig(t))
			h.d1.rows = []map[string]any{{"deer": "slime", "creator": "svaj", "kinskode": "AB"}}

			h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme "+tt.name))
			lines := h.collect(t)

			if len(lines) != 2 {
				t.Fatalf("got %d lines, want art plus a credit: %q", len(lines), lines)
			}
			if !strings.HasSuffix(lines[1], "slime by svaj") {
				t.Errorf("credit = %q", lines[1])
			}
			if sql := h.d1.statements(); !strings.Contains(sql[0], tt.want) {
				t.Errorf("statement = %q, want %q", sql[0], tt.want)
			}
		})
	}
}

func TestDeermeCreditsTheModifiersXPicked(t *testing.T) {
	h := newHarness(t, testConfig(t))
	h.d1.rows = []map[string]any{{"deer": "slime", "creator": "svaj", "kinskode": "ABCD\nEFGH"}}

	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme x|slime"))
	lines := h.collect(t)

	credit := lines[len(lines)-1]
	if !strings.Contains(credit, "slime by svaj (") || !strings.Contains(credit, "invert") {
		t.Errorf("credit = %q, want the rolled modifiers", credit)
	}
}

func TestDeermeDefaultsToDeer(t *testing.T) {
	h := newHarness(t, testConfig(t))
	h.d1.rows = []map[string]any{{"deer": "deer", "creator": "n/a", "kinskode": "A"}}

	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme"))
	h.collect(t)

	if got := h.d1.calls[0].Params; len(got) != 1 || got[0] != "deer" {
		t.Errorf("params = %q, want the default deer", got)
	}
}

func TestDeerme404s(t *testing.T) {
	h := newHarness(t, testConfig(t))
	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme nosuchdeer"))

	lines := h.collect(t)
	if len(lines) != 1 || !strings.Contains(lines[0], "404: Deer Not Found") {
		t.Fatalf("lines = %q", lines)
	}
	if !strings.Contains(lines[0], "hemera.day") {
		t.Errorf("the 404 doesn't point anywhere: %q", lines[0])
	}
}

func TestDeermeSurvivesABrokenDatabase(t *testing.T) {
	h := newHarness(t, testConfig(t))
	h.d1.status = http.StatusUnauthorized

	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme senordeer"))
	lines := h.collect(t)

	if len(lines) != 1 || !strings.Contains(lines[0], "not answering") {
		t.Fatalf("lines = %q", lines)
	}
	if strings.Contains(lines[0], "token") {
		t.Errorf("the failure leaked credentials: %q", lines[0])
	}
}

func TestDeermeStripsControlCharactersFromTheGallery(t *testing.T) {
	h := newHarness(t, testConfig(t))
	h.d1.rows = []map[string]any{{
		"deer":    "evil\r\nJOIN #gotcha",
		"creator": "sv\x03aj\x02",
		// The name is echoed on a credit line, so the injection has to be
		// scrubbed before it can reach the socket.
		"kinskode": "A",
	}}

	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme random"))
	lines := h.collect(t)

	credit := lines[len(lines)-1]
	if strings.Contains(credit, "\r") || strings.Contains(credit, "\n") || strings.Contains(credit, "\x03") {
		t.Fatalf("credit carries control characters: %q", credit)
	}
	if credit != "PRIVMSG #pank :evil JOIN #gotcha by svaj" {
		t.Errorf("credit = %q", credit)
	}
	for _, line := range lines {
		if strings.Count(line, "PRIVMSG") > 1 {
			t.Errorf("a second command was smuggled onto the line: %q", line)
		}
	}
}

func TestDeermeThrottlesAChannel(t *testing.T) {
	h := newHarness(t, testConfig(t))
	h.d1.rows = []map[string]any{{"deer": "slime", "creator": "svaj", "kinskode": "A"}}

	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme slime"))
	h.collect(t)

	h.plugin.cmdDeerme(message("pihl", "#pank", "!deerme senordeer"))
	lines := h.collect(t)

	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PRIVMSG pihl :") {
		t.Fatalf("lines = %q, want a private message to the asker", lines)
	}
	if !strings.Contains(lines[0], "not so fast") {
		t.Errorf("refusal = %q", lines[0])
	}
	if n := len(h.d1.statements()); n != 1 {
		t.Errorf("%d queries reached d1, want the throttled one to be dropped first", n)
	}
}

func TestDeermeThrottlesEachChannelSeparately(t *testing.T) {
	h := newHarness(t, testConfig(t))
	h.d1.rows = []map[string]any{{"deer": "slime", "creator": "svaj", "kinskode": "A"}}

	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme slime"))
	h.collect(t)
	h.plugin.cmdDeerme(message("pihl", "#other", "!deerme slime"))
	lines := h.collect(t)

	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PRIVMSG #other :") {
		t.Fatalf("lines = %q, want the art in the other channel", lines)
	}
}

func TestPunishesRepeats(t *testing.T) {
	cfg := testConfig(t)
	p := &Plugin{cfg: cfg, last: map[string]sighting{}, used: ratelimit.New(cfg.Wait())}
	p.last["#pank"] = sighting{deer: "slime", nick: "mallow", seen: true}

	normal := cfg.Wait()
	punished := time.Duration(float64(normal) * cfg.TimeoutPunish)

	if got := p.timeoutFor("#pank", "pihl", "senordeer", nil); got != normal {
		t.Errorf("fresh request = %v, want %v", got, normal)
	}
	if got := p.timeoutFor("#pank", "mallow", "senordeer", nil); got != punished {
		t.Errorf("same nick = %v, want %v", got, punished)
	}
	if got := p.timeoutFor("#pank", "pihl", "slime", nil); got != punished {
		t.Errorf("same deer = %v, want %v", got, punished)
	}
	if got := p.timeoutFor("#pank", "pihl", "random", nil); got != normal {
		t.Errorf("random = %v, want %v", got, normal)
	}
	if got := p.timeoutFor("#other", "mallow", "slime", nil); got != normal {
		t.Errorf("another channel = %v, want %v", got, normal)
	}
}

func TestAMissCostsLessThanADeer(t *testing.T) {
	cfg := testConfig(t)
	h := newHarness(t, cfg)

	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme nosuchdeer"))
	h.collect(t)

	if wait := h.plugin.used.Until("#pank"); wait > cfg.MissWait() || wait <= 0 {
		t.Errorf("wait after a miss = %v, want at most %v", wait, cfg.MissWait())
	}
}

func TestDeermeIgnoresPrivateMessages(t *testing.T) {
	h := newHarness(t, testConfig(t))
	h.d1.rows = []map[string]any{{"deer": "slime", "creator": "svaj", "kinskode": "A"}}

	h.plugin.cmdDeerme(message("mallow", "ohayoubot", "!deerme slime"))

	if lines := h.silence(t); len(lines) != 0 {
		t.Errorf("the bot answered a private deerme: %q", lines)
	}
}

func TestPrivilegedNickDeersInPrivate(t *testing.T) {
	cfg := testConfig(t)
	zero := 0
	cfg.Privileged = map[string]User{
		"mallow": {Host: "example.host", Timeout: &zero},
	}
	h := newHarness(t, cfg)
	h.d1.rows = []map[string]any{{"deer": "slime", "creator": "svaj", "kinskode": "A"}}

	h.plugin.cmdDeerme(message("mallow", "ohayoubot", "!deerme slime"))
	lines := h.collect(t)

	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PRIVMSG mallow :") {
		t.Fatalf("lines = %q, want the art back in private", lines)
	}

	// A zero timeout means the second one lands too.
	h.plugin.cmdDeerme(message("mallow", "ohayoubot", "!deerme slime"))
	if lines := h.collect(t); len(lines) != 1 || !strings.HasPrefix(lines[0], "PRIVMSG mallow :") {
		t.Errorf("lines = %q, want no wait for a privileged nick", lines)
	}
}

func TestPrivilegeNeedsTheRightHost(t *testing.T) {
	cfg := testConfig(t)
	cfg.Privileged = map[string]User{"mallow": {Host: "the.real.host"}}
	h := newHarness(t, cfg)

	if _, ok := h.plugin.privilegedFor(message("mallow", "#pank", "!deerme")); ok {
		t.Error("a borrowed nick was privileged")
	}

	cfg.PrivilegedMatch = []string{"nick"}
	h = newHarness(t, cfg)
	if _, ok := h.plugin.privilegedFor(message("mallow", "#pank", "!deerme")); !ok {
		t.Error("nick-only matching did not apply")
	}

	cfg.PrivilegedMatch = []string{"host"}
	cfg.Privileged = map[string]User{"someone": {Host: "example.host"}}
	h = newHarness(t, cfg)
	if _, ok := h.plugin.privilegedFor(message("anynick", "#pank", "!deerme")); !ok {
		t.Error("host-only matching did not apply")
	}
}

func TestBanned(t *testing.T) {
	cfg := testConfig(t)
	cfg.IgnoreNicks = []string{"Spammer"}
	cfg.IgnoreHosts = []string{"bad.host"}
	cfg.IgnoreChannels = []string{"#Quiet"}
	h := newHarness(t, cfg)

	banned := []*bot.Message{
		{Nick: "spammer", Host: "example.host", Target: "#pank"},
		{Nick: "mallow", Host: "BAD.host", Target: "#pank"},
		{Nick: "mallow", Host: "example.host", Target: "#quiet"},
	}
	for _, m := range banned {
		if h.plugin.bannedBy(m) == "" {
			t.Errorf("%+v was not banned", m)
		}
	}
	if got := h.plugin.bannedBy(&bot.Message{Nick: "mallow", Host: "example.host", Target: "#pank"}); got != "" {
		t.Errorf("an ordinary request matched %q", got)
	}
}

func TestBannedUsersGetNothing(t *testing.T) {
	cfg := testConfig(t)
	cfg.IgnoreNicks = []string{"spammer"}
	h := newHarness(t, cfg)
	h.d1.rows = []map[string]any{{"deer": "slime", "creator": "svaj", "kinskode": "A"}}

	h.plugin.cmdDeerme(message("spammer", "#pank", "!deerme slime"))
	h.plugin.cmdPrevDeer(message("spammer", "#pank", "!prevdeer"))

	if lines := h.silence(t); len(lines) != 0 {
		t.Errorf("a banned nick got a reply: %q", lines)
	}
	if n := len(h.d1.statements()); n != 0 {
		t.Errorf("%d queries reached d1 for a banned nick", n)
	}
}

func TestPrevDeer(t *testing.T) {
	h := newHarness(t, testConfig(t))

	h.plugin.cmdPrevDeer(message("mallow", "#pank", "!prevdeer"))
	if lines := h.collect(t); len(lines) != 1 || !strings.Contains(lines[0], "No deer has been sighted") {
		t.Fatalf("lines = %q", lines)
	}

	h.d1.rows = []map[string]any{{"deer": "slime", "creator": "svaj", "kinskode": "AB\nCD"}}
	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme u|slime"))
	h.collect(t)

	h.forget()
	h.plugin.cmdPrevDeer(message("pihl", "#pank", "!prevdeer"))
	lines := h.collect(t)
	if len(lines) != 1 {
		t.Fatalf("lines = %q", lines)
	}
	if !strings.Contains(lines[0], "slime by svaj") || !strings.Contains(lines[0], "upsidedown") {
		t.Errorf("prevdeer = %q", lines[0])
	}
}

func TestHelp(t *testing.T) {
	h := newHarness(t, testConfig(t))
	h.d1.rows = []map[string]any{{"n": 1640}}

	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme help"))
	lines := h.collect(t)

	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PRIVMSG mallow :") {
		t.Fatalf("lines = %q, want one private message", lines)
	}
	for _, want := range []string{"How to deer", "!deerme <mods>|<deer>", "1640 deer total", "Ready to deer!", "hemera.day"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("help is missing %q: %q", want, lines[0])
		}
	}

	// The count is cached, so another nick's help doesn't mean a second query.
	h.plugin.cmdDeerme(message("pihl", "#pank", "!deerme help"))
	h.collect(t)
	if n := len(h.d1.statements()); n != 1 {
		t.Errorf("%d count queries, want 1", n)
	}
}

func TestHelpModifiers(t *testing.T) {
	h := newHarness(t, testConfig(t))
	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme help modifiers"))
	lines := h.collect(t)

	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PRIVMSG mallow :") {
		t.Fatalf("lines = %q", lines)
	}
	for _, c := range modifierOrder {
		if !strings.Contains(lines[0], string(c)+"(=") {
			t.Errorf("modifier %q is not listed: %q", string(c), lines[0])
		}
	}
	if n := len(h.d1.statements()); n != 0 {
		t.Errorf("listing modifiers hit d1 %d times", n)
	}
}

func TestSanitiseName(t *testing.T) {
	tests := map[string]string{
		"SenorDeer":             "senordeer",
		"  spaced  out  ":       "spaced out",
		"drop/these;chars--_":   "dropthesechars--_",
		"'; DROP TABLE deer;":   "drop table deer",
		"\r\nJOIN #gotcha":      "join gotcha",
		strings.Repeat("a", 80): strings.Repeat("a", maxNameLen),
	}
	for in, want := range tests {
		if got := sanitiseName(in); got != want {
			t.Errorf("sanitiseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitiseText(t *testing.T) {
	tests := map[string]string{
		"svaj":                  "svaj",
		"sv\x03aj":              "svaj",
		"a\r\nQUIT":             "a QUIT",
		"\u202een":              "en",
		strings.Repeat("é", 80): strings.Repeat("é", maxTextLen),
	}
	for in, want := range tests {
		if got := sanitiseText(in); got != want {
			t.Errorf("sanitiseText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLineBudgetLeavesRoomForTheProtocol(t *testing.T) {
	target := "#a-fairly-long-channel-name"
	budget := lineBudget(target)
	line := "PRIVMSG " + target + " :" + strings.Repeat("@", budget) + "\r\n"
	if len(line) > irctext.LineLimit {
		t.Errorf("a full line is %d bytes, over the %d limit", len(line), irctext.LineLimit)
	}
}

func TestRepeatedRefusalsAreNotRepeated(t *testing.T) {
	h := newHarness(t, testConfig(t))
	h.d1.rows = []map[string]any{{"deer": "slime", "creator": "svaj", "kinskode": "A"}}

	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme slime"))
	h.collect(t)

	for i := 0; i < 5; i++ {
		h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme slime"))
	}
	if lines := h.collect(t); len(lines) != 1 {
		t.Errorf("got %d refusals for 5 requests, want 1: %q", len(lines), lines)
	}
}

func TestWaitMessageQuotesTheTimeoutThatApplied(t *testing.T) {
	cfg := testConfig(t)
	p := &Plugin{cfg: cfg}
	normal := cfg.Wait()
	punished := time.Duration(float64(normal) * cfg.TimeoutPunish)

	got := p.waitMessage(290*time.Second, normal, User{}, false)
	for _, want := range []string{"every 300 seconds", "291 seconds from now"} {
		if !strings.Contains(got, want) {
			t.Errorf("ordinary refusal = %q, want %q", got, want)
		}
	}

	// The punished timeout is the one that refused, so quoting the configured
	// 300 would put a countdown longer than the wait it claims to explain.
	got = p.waitMessage(499*time.Second, punished, User{}, false)
	for _, want := range []string{"usual 300 seconds to 510", "500 seconds from now"} {
		if !strings.Contains(got, want) {
			t.Errorf("punished refusal = %q, want %q", got, want)
		}
	}

	higher := 600
	got = p.waitMessage(500*time.Second, 600*time.Second, User{Timeout: &higher}, true)
	for _, want := range []string{"+300 seconds", "501 seconds from now"} {
		if !strings.Contains(got, want) {
			t.Errorf("privileged punishment = %q, want %q", got, want)
		}
	}
}

func TestRefusalCountdownFitsTheTimeoutItQuotes(t *testing.T) {
	cfg := testConfig(t)
	h := newHarness(t, cfg)
	h.d1.rows = []map[string]any{{"deer": "slime", "creator": "svaj", "kinskode": "A"}}

	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme slime"))
	h.collect(t)

	// The same nick asking for the same deer is punished, so the refusal has to
	// carry the punished timeout down from claim.
	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme slime"))
	lines := h.collect(t)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "PRIVMSG mallow :") {
		t.Fatalf("lines = %q, want one refusal to the asker", lines)
	}

	punished := int(float64(cfg.Timeout) * cfg.TimeoutPunish)
	if !strings.Contains(lines[0], strconv.Itoa(punished)) {
		t.Errorf("refusal = %q, want the %d second timeout it applied", lines[0], punished)
	}

	m := regexp.MustCompile(`like (\d+) seconds from now`).FindStringSubmatch(lines[0])
	if m == nil {
		t.Fatalf("refusal has no countdown: %q", lines[0])
	}
	left, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if left > punished || left <= cfg.Timeout {
		t.Errorf("countdown = %d seconds, want between %d and %d: %q",
			left, cfg.Timeout, punished, lines[0])
	}
}

func TestPrevDeerIsRateLimited(t *testing.T) {
	h := newHarness(t, testConfig(t))
	for i := 0; i < 5; i++ {
		h.plugin.cmdPrevDeer(message("mallow", "#pank", "!prevdeer"))
	}
	if lines := h.collect(t); len(lines) != 1 {
		t.Errorf("got %d replies for 5 !prevdeer, want 1: %q", len(lines), lines)
	}
}

// A restart must not be a way to skip a channel's timeout.
func TestCooldownSurvivesARestart(t *testing.T) {
	cfg := testConfig(t)
	h := newHarness(t, cfg)
	h.d1.rows = []map[string]any{{"deer": "slime", "creator": "svaj", "kinskode": "A"}}

	h.plugin.cmdDeerme(message("mallow", "#pank", "!deerme slime"))
	h.collect(t)

	ctx := context.Background()
	if err := h.plugin.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// A second plugin over the same store stands in for the process restarting.
	fresh := New()
	fresh.cfg = cfg
	fresh.used = ratelimit.New(cfg.Wait())
	fresh.kv = h.plugin.kv
	if err := fresh.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	if wait := fresh.used.Until("#pank"); wait <= 0 {
		t.Error("the channel was free to deer again straight after a restart")
	}
}

// Nothing saved is not an error: the first run of a new plugin has no state.
func TestStartWithNothingSaved(t *testing.T) {
	h := newHarness(t, testConfig(t))
	if err := h.plugin.Start(context.Background()); err != nil {
		t.Errorf("Start with an empty store: %v", err)
	}
}

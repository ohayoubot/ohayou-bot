package drop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/bot/bottest"
	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
	"github.com/ohayoubot/ohayou-bot/internal/store/sqlite"
	"github.com/ohayoubot/ohayou-bot/internal/web"
)

const testSecret = "0123456789abcdef0123456789abcdef"

type harness struct {
	*bottest.Harness
	plugin *Plugin
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessIn(t, []string{"#chan", "#Other"})
}

// newHarnessIn builds a bot already in the given channels, which is what
// shared() intersects a nick's whois against.
// newHarnessIn builds a bot already in the given channels, which is what
// shared() intersects a nick's whois against.
func newHarnessIn(t *testing.T, channels []string) *harness {
	t.Helper()

	h := &harness{Harness: bottest.New(t, bottest.InChannels(channels...))}

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Init(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}

	h.plugin = New()
	if _, err := h.plugin.Configure(plugin.Config{
		Block: json.RawMessage(`{
			"url": "https://hemera.day/drop/",
			"imageBase": "https://img.hemera.day"
		}`),
		Cloudflare: config.Cloudflare{AccountID: "acct", DatabaseID: "db", APIToken: "token"},
		Web:        config.Web{Secret: testSecret},
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	deps := plugin.Deps{Bot: h.Bot, Store: db, Log: h.Log, Web: web.NewMinter(testSecret)}
	if err := h.plugin.Register(deps.For("drop")); err != nil {
		t.Fatalf("register: %v", err)
	}

	h.Start()
	return h
}

// identify does what !identify does, so the nick has proved itself to the bot.
func (h *harness) identify(t *testing.T, nick string) {
	t.Helper()
	if _, err := h.Bot.Identify(context.Background(), nick); err != nil {
		t.Fatalf("identifying %q: %v", nick, err)
	}
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

// payloadOf verifies the link the way the worker will, so this reads what the
// user was actually handed rather than what the minter meant to send.
func payloadOf(t *testing.T, token string) web.Grant {
	t.Helper()
	g, id, err := web.NewMinter(testSecret).Verify(token)
	if err != nil {
		t.Fatalf("the link the user was given does not verify: %v", err)
	}
	if id == "" {
		t.Error("the grant carries no id, so it could be redeemed twice")
	}
	return g
}

func TestUploadSendsTheLinkPrivately(t *testing.T) {
	h := newHarness(t)
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "@#chan +#other"})
	h.identify(t, "mallow")

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	lines := h.Collect()

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
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "@#chan +#other #elsewhere"})
	h.identify(t, "mallow")

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	_, token := linkIn(t, h.Collect())

	g := payloadOf(t, token)
	if g.Account != "Mallow" {
		t.Errorf("account = %q, want Mallow", g.Account)
	}
	if g.Nick != "mallow" {
		t.Errorf("nick = %q, want mallow", g.Nick)
	}
	// #elsewhere is not somewhere the bot is, so it cannot be a destination.
	// #Other keeps the bot's spelling, not the one whois echoed.
	if len(g.Channels) != 2 || g.Channels[0] != "#chan" || g.Channels[1] != "#Other" {
		t.Errorf("channels = %v, want [#chan #Other]", g.Channels)
	}
	// A grant good for anything else would be one the upload page could spend
	// somewhere the user never asked about.
	if g.Scopes != web.ScopeDrop {
		t.Errorf("scopes = %d, want only drop (%d)", g.Scopes, web.ScopeDrop)
	}
	if g.TTL <= 0 {
		t.Errorf("ttl = %s, so the link is already dead", g.TTL)
	}
}

func TestUploadRefusesAnUnidentifiedNick(t *testing.T) {
	h := newHarness(t)
	// Logged in to services when they identified, logged out since.
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})
	h.identify(t, "mallow")
	h.Says("mallow", bottest.Whois{Channels: "#chan"})

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	lines := h.Collect()

	for _, line := range lines {
		if strings.Contains(line, "hemera.day/drop/#") {
			t.Fatalf("an unidentified nick got a link: %s", line)
		}
	}
	if !said(lines, "identified") {
		t.Errorf("no explanation given: %v", lines)
	}
}

// An upload is announced under the nick that asked for it, so a nick somebody
// else is wearing must not become a link.
func TestUploadRefusesANickThatNeverIdentifiedWithTheBot(t *testing.T) {
	h := newHarness(t)
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	lines := h.Collect()

	for _, line := range lines {
		if strings.Contains(line, "hemera.day/drop/#") {
			t.Fatalf("a nick that never identified with the bot got a link: %s", line)
		}
	}
	if !said(lines, "identify") {
		t.Errorf("the refusal did not say how to fix it: %v", lines)
	}
	if n := h.WhoisCount(); n != 0 {
		t.Errorf("%d whois, want the refusal decided without asking the server", n)
	}
}

// A proof the bot never saw expire, against a nick now logged in as somebody
// else.
func TestUploadRefusesWhenTheAccountHasChangedSinceIdentifying(t *testing.T) {
	h := newHarness(t)
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})
	h.identify(t, "mallow")
	h.Says("mallow", bottest.Whois{Account: "Someone", Channels: "#chan"})

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	lines := h.Collect()

	for _, line := range lines {
		if strings.Contains(line, "hemera.day/drop/#") {
			t.Fatalf("a nick whose account changed got a link anyway: %s", line)
		}
	}
	if !said(lines, "identify") {
		t.Errorf("the refusal did not say how to fix it: %v", lines)
	}
}

func TestUploadRefusesWhenNoChannelIsShared(t *testing.T) {
	h := newHarness(t)
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#elsewhere"})
	h.identify(t, "mallow")

	h.plugin.cmdUpload(message("mallow", "mallow", "!upload"))
	lines := h.Collect()

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
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})
	h.identify(t, "mallow")
	h.NeverAnswers()

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

	lines := h.Drain()
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
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})
	h.identify(t, "mallow")
	before := h.WhoisCount()

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	h.Collect()
	if n := h.WhoisCount() - before; n != 1 {
		t.Fatalf("%d whois for the first link, want 1", n)
	}

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	lines := h.Collect()

	if n := h.WhoisCount() - before; n != 1 {
		t.Errorf("%d whois after a repeat, want the second to be refused before asking", n)
	}
	if !said(lines, "Try again in") {
		t.Errorf("no cooldown message: %v", lines)
	}
}

func TestUploadCooldownExpires(t *testing.T) {
	h := newHarness(t)
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})
	h.identify(t, "mallow")
	before := h.WhoisCount()

	now := time.Now()
	h.plugin.now = func() time.Time { return now }

	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	h.Collect()

	now = now.Add(61 * time.Second)
	h.plugin.cmdUpload(message("mallow", "#chan", "!upload"))
	h.Collect()

	if n := h.WhoisCount() - before; n != 2 {
		t.Errorf("%d whois, want 2 once the cooldown has run out", n)
	}
}

func TestUploadInPrivateSaysNothingInChannel(t *testing.T) {
	h := newHarness(t)
	h.Says("mallow", bottest.Whois{Account: "Mallow", Channels: "#chan"})
	h.identify(t, "mallow")

	h.plugin.cmdUpload(message("mallow", "mallow", "!upload"))
	lines := h.Collect()

	for _, line := range lines {
		if strings.HasPrefix(strings.Fields(line)[1], "#") {
			t.Errorf("a private request was answered in a channel: %s", line)
		}
	}
}

// The channel list a grant carries is capped, because the worker refuses a
// longer one. SharedWith itself is the bot's and tested there.
func TestGrantChannelsAreCapped(t *testing.T) {
	var all []string
	for i := range web.MaxChannels + 5 {
		all = append(all, fmt.Sprintf("#c%d", i))
	}
	h := newHarnessIn(t, all)

	got := web.Channels(h.Bot.SharedWith(all))
	if len(got) != web.MaxChannels {
		t.Errorf("kept %d channels, want the worker's ceiling of %d", len(got), web.MaxChannels)
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

	lines := h.Collect()
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

	lines := h.Collect()
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
	h.Collect()

	got, err := h.plugin.kv.Get(context.Background(), cursorKey)
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

	saved, err := h.plugin.kv.Get(context.Background(), cursorKey)
	if err != nil || saved != "120" {
		t.Errorf("stored cursor = %q, %v; want 120", saved, err)
	}
}

func TestStartAtResumesFromTheStoredCursor(t *testing.T) {
	h, fake := announceHarness(t)
	fake.newest = 999
	if err := h.plugin.kv.Set(context.Background(), cursorKey, "12"); err != nil {
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
	if lines := h.Drain(); len(lines) != 0 {
		t.Fatalf("said something while d1 was down: %v", lines)
	}

	fake.set(map[string]any{"id": 1, "nick": "whatapath", "channel": "#chan", "key": "a.png"})
	fake.mu.Lock()
	fake.status = 0
	fake.mu.Unlock()

	// Collect rather than Drain: the next poll is a tick away, so this has to
	// wait for a line rather than take what has already arrived.
	lines := h.Collect()
	if len(lines) != 1 || !strings.Contains(lines[0], "whatapath uploaded: https://img.hemera.day/a.png") {
		t.Fatalf("did not recover once d1 came back: %v", lines)
	}

	cancel()
	h.plugin.bot.Wait()
}

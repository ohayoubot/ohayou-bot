package ohayou_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/bot/bottest"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
	"github.com/ohayoubot/ohayou-bot/internal/plugins/ohayou"
	"github.com/ohayoubot/ohayou-bot/internal/store"
	"github.com/ohayoubot/ohayou-bot/internal/store/sqlite"
	"github.com/ohayoubot/ohayou-bot/internal/task"
)

// testGame wires the game to a fake network and an empty store, the way main
// does.
func testGame(t *testing.T, opts ...bottest.Option) (*bottest.Harness, *sqlite.DB) {
	t.Helper()
	h := bottest.New(t, opts...)

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	if err := db.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}

	g := ohayou.New()
	if _, err := g.Configure(plugin.Config{
		Block: json.RawMessage(`{"dataDir": "../../../data"}`),
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	deps := plugin.Deps{Bot: h.Bot, Store: db, Log: h.Log, Runner: task.NewRunner(db, h.Bot, h.Log)}
	if err := g.Register(deps.For("ohayou")); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := g.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	return h, db
}

// The whole path: a line in a channel reaches the game, which answers and
// writes the user to the store.
func TestOhayouEndToEnd(t *testing.T) {
	h, db := testGame(t, bottest.InChannels("#test"))
	ctx := context.Background()

	h.Start()
	h.Say("alice", "#test", "!ohayou")

	line := h.Next()
	if !strings.Contains(line, "PRIVMSG #test") ||
		!strings.Contains(strings.ToLower(line), "first ohayou") {
		t.Fatalf("unexpected reply: %q", line)
	}

	if _, err := db.GetUser(ctx, "alice"); err != nil {
		t.Errorf("expected alice to be persisted: %v", err)
	}
}

// The account is what the website will key on, and !register is the first
// place the bot learns it.
func TestRegisterRecordsTheServicesAccount(t *testing.T) {
	h, db := testGame(t, bottest.InChannels("#test"))
	ctx := context.Background()

	h.Says("alice", bottest.Whois{Account: "AliceAcct", Channels: "#test"})
	h.Start()

	h.Say("alice", "#test", "!ohayou")
	h.Drain()
	h.Say("alice", "#test", "!register yes")
	h.Drain()

	u, err := db.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("get alice: %v", err)
	}
	if u.Account != "AliceAcct" {
		t.Errorf("account = %q, want AliceAcct", u.Account)
	}
	if !u.Registered {
		t.Error("alice registered but the row does not say so")
	}
}

// Nobody is on the website until they say so, and saying so is reversible.
func TestWebVisibilityIsOffUntilAsked(t *testing.T) {
	h, db := testGame(t, bottest.InChannels("#test"))
	ctx := context.Background()

	h.Start()
	h.Say("alice", "#test", "!ohayou")
	h.Drain()

	u, err := db.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("get alice: %v", err)
	}
	if u.Web != store.VisibilityUnset {
		t.Fatalf("a new user starts at %q, want unset", u.Web)
	}

	for _, tc := range []struct {
		say  string
		want store.Visibility
	}{
		{"!web on", store.VisibilityPublic},
		{"!web off", store.VisibilityHidden},
		{"!web nonsense", store.VisibilityHidden},
	} {
		h.Say("alice", "#test", tc.say)
		h.Drain()

		u, err := db.GetUser(ctx, "alice")
		if err != nil {
			t.Fatalf("get alice after %q: %v", tc.say, err)
		}
		if u.Web != tc.want {
			t.Errorf("%q left visibility at %q, want %q", tc.say, u.Web, tc.want)
		}
	}
}

// A nick that is not logged in to services proves nothing, so nothing is
// recorded against it.
func TestRegisterRecordsNothingForALoggedOutNick(t *testing.T) {
	h, db := testGame(t, bottest.InChannels("#test"))
	ctx := context.Background()

	h.Says("mallory", bottest.Whois{Channels: "#test"})
	h.Start()

	h.Say("mallory", "#test", "!ohayou")
	h.Drain()
	h.Say("mallory", "#test", "!register yes")
	h.Drain()

	u, err := db.GetUser(ctx, "mallory")
	if err != nil {
		t.Fatalf("get mallory: %v", err)
	}
	if u.Account != "" {
		t.Errorf("account = %q for a nick that never identified", u.Account)
	}
	if u.Registered {
		t.Error("a nick that is not logged in to services was registered")
	}
}

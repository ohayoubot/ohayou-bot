package ohayou_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/bot/bottest"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
	"github.com/ohayoubot/ohayou-bot/internal/plugins/ohayou"
	"github.com/ohayoubot/ohayou-bot/internal/store/sqlite"
	"github.com/ohayoubot/ohayou-bot/internal/task"
)

// The whole path: a line in a channel reaches the game, which answers and
// writes the user to the store.
func TestOhayouEndToEnd(t *testing.T) {
	h := bottest.New(t, bottest.InChannels("#test"))

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

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

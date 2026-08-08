package ohayou

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/d1"
	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// Commands are what somebody asked for on the website. This reads them back and
// applies them through the store, so every guard there still stands between a
// browser and a player's ohayous.
//
// Only cosmetic changes are on the list: the game is played in irc.
const (
	commandPoll  = 20 * time.Second
	commandBatch = 50
	// commandCursor is how far this bot has read, kept so a restart does not
	// replay what it applied.
	commandCursor = "commands"
)

// command is one row of the queue.
type command struct {
	ID      int64  `json:"id"`
	Account string `json:"account"`
	Kind    string `json:"kind"`
	Value   string `json:"value"`
}

// startCommands begins draining the queue, when there is one to drain.
func (g *Plugin) startCommands(ctx context.Context) {
	if g.commands == nil {
		return
	}
	g.log.Info("taking requests from the website", "every", commandPoll)
	g.bot.Every(ctx, commandPoll, func() { g.pollCommands(ctx) })
}

func (g *Plugin) pollCommands(ctx context.Context) {
	// Guarded here as well as in startCommands: this is the one on a timer.
	if g.commands == nil {
		return
	}
	if !g.commandsStarted {
		from, err := g.commandStart(ctx)
		if err != nil {
			g.log.Warn("waiting to take requests", "err", err)
			return
		}
		g.commandCursor, g.commandsStarted = from, true
	}

	var rows []command
	err := g.commands.Query(ctx,
		"SELECT id, account, kind, value FROM command WHERE id > CAST(?1 AS INTEGER) ORDER BY id LIMIT CAST(?2 AS INTEGER)",
		[]string{strconv.FormatInt(g.commandCursor, 10), strconv.FormatInt(commandBatch, 10)}, &rows)
	if err != nil {
		g.log.Warn("reading the request queue", "err", err)
		return
	}

	for _, row := range rows {
		g.apply(ctx, row)
		// Per row rather than per batch: a crash repeats at most one request.
		g.commandCursor = row.ID
		if err := g.kv.Set(ctx, commandCursor, strconv.FormatInt(row.ID, 10)); err != nil {
			g.log.Error("saving the request cursor", "id", row.ID, "err", err)
		}
	}
}

// commandStart resumes where this bot left off, or at the end of the queue: one
// switched on against a database with requests in it should not act on ones
// made before it was listening.
func (g *Plugin) commandStart(ctx context.Context) (int64, error) {
	switch saved, err := g.kv.Get(ctx, commandCursor); {
	case err == nil:
		return strconv.ParseInt(saved, 10, 64)
	case !errors.Is(err, store.ErrNotFound):
		return 0, err
	}

	var rows []struct {
		ID int64 `json:"id"`
	}
	if err := g.commands.Query(ctx, "SELECT COALESCE(MAX(id), 0) AS id FROM command", nil, &rows); err != nil {
		return 0, err
	}
	newest := int64(0)
	if len(rows) > 0 {
		newest = rows[0].ID
	}
	return newest, g.kv.Set(ctx, commandCursor, strconv.FormatInt(newest, 10))
}

// apply does what was asked, if whoever asked may. The site checked the shape;
// this checks the half it cannot know, whether the account belongs to a player.
// A failure here is dropped rather than retried: nothing about it will differ.
func (g *Plugin) apply(ctx context.Context, c command) {
	nick, err := g.store.PlayerByAccount(ctx, c.Account)
	if errors.Is(err, store.ErrNotFound) {
		g.log.Info("request dropped", "reason", "no player for that account",
			"id", c.ID, "account", c.Account)
		return
	}
	if err != nil {
		g.log.Error("resolving a request", "id", c.ID, "err", err)
		return
	}

	switch c.Kind {
	case "flag":
		g.applyFlag(ctx, c, nick)
	case "territory":
		g.applyTerritory(ctx, c, nick)
	default:
		// The site's list and this one are the same list twice.
		g.log.Warn("request dropped", "reason", "unknown kind", "id", c.ID, "kind", c.Kind)
	}
}

func (g *Plugin) applyFlag(ctx context.Context, c command, nick string) {
	deer := strings.TrimSpace(c.Value)
	if len(deer) > maxFlag {
		g.log.Info("request dropped", "reason", "flag too long", "id", c.ID)
		return
	}
	if err := g.store.SetFlag(ctx, nick, deer); err != nil {
		g.log.Error("applying a flag", "id", c.ID, "nick", nick, "err", err)
		return
	}
	g.record(eventFlag, nick, "", map[string]string{"deer": deer})
	g.log.Info("request applied", "id", c.ID, "kind", c.Kind, "nick", nick, "flag", deer)
}

func (g *Plugin) applyTerritory(ctx context.Context, c command, nick string) {
	var want store.Visibility
	switch c.Value {
	case "on":
		want = store.VisibilityPublic
	case "off":
		want = store.VisibilityHidden
	default:
		g.log.Info("request dropped", "reason", "not on or off", "id", c.ID)
		return
	}
	if err := g.store.SetVisibility(ctx, nick, want); err != nil {
		g.log.Error("applying a visibility", "id", c.ID, "nick", nick, "err", err)
		return
	}
	g.log.Info("request applied", "id", c.ID, "kind", c.Kind, "nick", nick, "to", want)
}

// newCommandQueue reads the game's own database with a D1:Read token; the
// worker makes every write.
func newCommandQueue(account, database, token string, timeout time.Duration) *d1.Client {
	if account == "" || database == "" || token == "" {
		return nil
	}
	return d1.New(d1.APIBase, account, database, token, timeout)
}

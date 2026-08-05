package ohayou

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// saltKey is where the salt for anonymous plot ids is kept. It must survive a
// restart: a new one would reshuffle every unnamed plot on the map.
const saltKey = "plotsalt"

// publishEvery is how often the projection is rebuilt and compared. It is not
// how often anything is written: an unchanged projection is not published, so a
// quiet channel costs one read of the store and nothing else.
const publishEvery = 2 * time.Minute

// The two tables the site is taught to accept, in web/lib/site/ingest.js.
const (
	tablePlot        = "plot"
	tablePlotPrivate = "plot_private"
)

// startPublishing keeps the site's copy in step, when there is a site.
func (g *Plugin) startPublishing(ctx context.Context) {
	if g.feed == nil {
		g.log.Info("not publishing", "reason", "no site configured")
		return
	}

	// Once at startup, so a restart after a change does not wait for a tick.
	g.bot.Go(func() { g.publish(ctx) })
	g.bot.Every(ctx, publishEvery, func() { g.publish(ctx) })
}

// publish sends both tiers when either has changed.
//
// Rather than marking state dirty at every mutation, the projection is built
// and compared: there is no list of places to remember to update, so a new game
// feature cannot quietly stop the site from noticing. Building it costs one
// pass over the users who agreed to be on it.
func (g *Plugin) publish(ctx context.Context) {
	public, private, err := g.projection(ctx)
	if err != nil {
		g.log.Error("building the projection", "err", err)
		return
	}

	g.send(ctx, tablePlot, public)
	// The private tier is sent second: if the run is cut short between them, the
	// site is left showing less than it could rather than more than it should.
	g.send(ctx, tablePlotPrivate, private)
}

func (g *Plugin) send(ctx context.Context, table string, rows any) {
	raw, err := json.Marshal(rows)
	if err != nil {
		g.log.Error("encoding a projection", "table", table, "err", err)
		return
	}

	sum := sha256.Sum256(raw)
	if g.published[table] == sum {
		return
	}

	result, err := g.feed.Publish(ctx, table, rows)
	if err != nil {
		// Left unrecorded, so the next tick tries again.
		g.log.Error("publishing", "table", table, "err", err)
		return
	}
	g.published[table] = sum
	g.log.Info("published", "table", table, "status", result.Status, "rows", result.Rows)
}

// projection builds the world and the private tier.
//
// Everyone is on the map. Whether their plot carries their name is theirs to
// say: without consent it gets a salted id, no nick and no buildings, which
// leaves the scale of the holding and nothing that says whose it is. The
// private tier is consent-gated outright, so a balance and a vault never leave
// the bot for somebody who never asked to be here.
func (g *Plugin) projection(ctx context.Context) ([]Plot, []PrivatePlot, error) {
	names, err := g.store.Players(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing the players: %w", err)
	}

	salt, err := g.plotSalt(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the plot salt: %w", err)
	}

	runs, err := g.pendingRuns(ctx)
	if err != nil {
		// Not fatal: a plot without its countdowns is worth more than none.
		g.log.Warn("reading pending activities", "err", err)
		runs = map[string]map[string]time.Time{}
	}

	// Never nil, so an empty projection marshals to [] and publishes as "this
	// table is now empty" rather than as nothing at all.
	public := make([]Plot, 0, len(names))
	private := make([]PrivatePlot, 0, len(names))

	for _, name := range names {
		u, err := g.store.GetUser(ctx, name)
		if err != nil {
			g.log.Warn("reading a user to publish", "nick", name, "err", err)
			continue
		}
		if !publishable(u) {
			public = append(public, g.anonymousPlot(u, plotID(salt, name)))
			continue
		}
		public = append(public, g.publicPlot(u))
		private = append(private, g.privatePlot(u, runs[name]))
	}
	return public, private, nil
}

// plotSalt is what makes an anonymous plot id unguessable. It is generated
// once and kept, because an id that changed would move somebody's plot across
// the map every time the bot restarted.
func (g *Plugin) plotSalt(ctx context.Context) (string, error) {
	switch salt, err := g.kv.Get(ctx, saltKey); {
	case err == nil && salt != "":
		return salt, nil
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return "", err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	salt := hex.EncodeToString(raw)
	if err := g.kv.Set(ctx, saltKey, salt); err != nil {
		return "", err
	}
	g.log.Info("generated a salt for anonymous plots")
	return salt, nil
}

// plotID is a stable id for an unnamed plot. Salted, so a list of nicks cannot
// be turned into a list of plots by anybody holding the published table.
func plotID(salt, username string) string {
	sum := sha256.Sum256([]byte(salt + "\x00" + username))
	return base64.RawURLEncoding.EncodeToString(sum[:9])
}

// pendingRuns is every outstanding activity, by user and then by kind, so one
// query covers everybody rather than one per player.
func (g *Plugin) pendingRuns(ctx context.Context) (map[string]map[string]time.Time, error) {
	pending, err := g.tasks.Pending(ctx)
	if err != nil {
		return nil, err
	}

	out := map[string]map[string]time.Time{}
	for _, t := range pending {
		if !isRun(t.Kind) {
			continue
		}
		if out[t.Key] == nil {
			out[t.Key] = map[string]time.Time{}
		}
		out[t.Key][t.Kind] = t.Due
	}
	return out, nil
}

// isRun keeps housekeeping tasks off a player's page: the police decay is the
// bot's business, not something to count down to.
func isRun(kind string) bool {
	for _, r := range runs {
		if r == kind {
			return true
		}
	}
	return false
}

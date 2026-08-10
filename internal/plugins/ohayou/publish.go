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
	"maps"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
	"github.com/ohayoubot/ohayou-bot/internal/web"
)

// saltKey holds the salt for anonymous plot ids. It must survive a restart: a
// new one would move every unnamed plot on the map.
const saltKey = "plotsalt"

// publishEvery is how often the projection is rebuilt and compared, not how
// often anything is written: an unchanged one is not published.
const publishEvery = 2 * time.Minute

// The tables the site is taught to accept, in web/lib/site/ingest.js.
const (
	tablePlot        = "plot"
	tablePlotPrivate = "plot_private"
	tableEvent       = "event"
)

// publishedTables is how many of them one round sends.
const publishedTables = 3

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
// The projection is built and compared rather than state being marked dirty at
// every mutation: there is no list of call sites to keep up to date, so a new
// game feature cannot quietly stop the site noticing.
func (g *Plugin) publish(ctx context.Context) {
	public, private, err := g.projection(ctx)
	if err != nil {
		g.log.Error("building the projection", "err", err)
		return
	}

	g.send(ctx, tablePlot, public)
	// Second: a run cut short between the two leaves the site showing less than
	// it could rather than more than it should.
	g.send(ctx, tablePlotPrivate, private)

	// After both, for the same reason: the feed names people the map draws.
	events, err := g.eventProjection(ctx)
	if err != nil {
		g.log.Error("building the chronicle", "err", err)
		return
	}
	g.sendEvents(ctx, events)
}

// sendEvents brings the site's chronicle up to date with the entries it has
// not been given, so a new line costs a line rather than the whole feed. The
// whole of it still goes when an entry the site holds has changed, which is
// what withdrawing a name does to every entry naming you.
func (g *Plugin) sendEvents(ctx context.Context, events []Event) {
	fresh, whole := g.unsent(events)
	switch {
	case whole:
		result, err := g.feed.Publish(ctx, tableEvent, events)
		g.recordEvents(events, result, err, len(events))
	case len(fresh) == 0:
		return
	default:
		result, err := g.feed.Append(ctx, tableEvent, fresh, eventFeed)
		g.recordEvents(events, result, err, len(fresh))
	}
}

// unsent is what the site has not been given, and whether the whole feed has
// to go instead: an entry it holds reads differently, or it holds nothing.
func (g *Plugin) unsent(events []Event) ([]Event, bool) {
	if g.sentEvents == nil {
		return nil, true
	}
	var fresh []Event
	for _, e := range events {
		switch held, ok := g.sentEvents[e.ID]; {
		case !ok:
			fresh = append(fresh, e)
		case !sameEvent(held, e):
			return nil, true
		}
	}
	return fresh, false
}

// recordEvents remembers what the site holds, and forgets it when the answer
// says otherwise, which sends the whole feed next round.
func (g *Plugin) recordEvents(events []Event, result web.Result, err error, sent int) {
	if err != nil {
		// Left unrecorded, so the next tick retries.
		g.log.Error("publishing", "table", tableEvent, "err", err)
		return
	}
	if !result.Published() {
		g.log.Info("published", "table", tableEvent, "status", result.Status, "rows", sent)
		return
	}
	if result.Total != len(events) {
		g.sentEvents = nil
		g.log.Warn("the site's chronicle is not ours",
			"theirs", result.Total, "ours", len(events))
		return
	}

	held := make(map[int64]Event, len(events))
	for _, e := range events {
		held[e.ID] = e
	}
	g.sentEvents = held
	g.log.Info("published", "table", tableEvent, "status", result.Status,
		"rows", sent, "held", result.Total)
}

func sameEvent(a, b Event) bool {
	return a.ID == b.ID && a.TS == b.TS && a.Kind == b.Kind &&
		a.Actor == b.Actor && a.Subject == b.Subject &&
		maps.Equal(a.Detail, b.Detail)
}

// eventProjection is the newest events, with anyone hidden left unnamed.
func (g *Plugin) eventProjection(ctx context.Context) ([]Event, error) {
	vis, err := g.store.Visibilities(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading visibilities: %w", err)
	}
	events, err := g.store.RecentEvents(ctx, eventLog)
	if err != nil {
		return nil, fmt.Errorf("reading the chronicle: %w", err)
	}
	return g.chronicle(events, vis), nil
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
		// Left unrecorded, so the next tick retries.
		g.log.Error("publishing", "table", table, "err", err)
		return
	}
	g.published[table] = sum
	g.log.Info("published", "table", table, "status", result.Status, "rows", result.Rows)
}

// projection builds the world and the private tier.
//
// Everyone is on the map, under their nick unless they opted out; an opted out
// plot gets a salted id, no nick and no buildings, which leaves the scale of
// the holding and nothing that says whose it is. The private tier needs an
// account as well, since it is served against a session rather than displayed.
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
		// Not fatal: a plot without countdowns beats no plot.
		g.log.Warn("reading pending activities", "err", err)
		runs = map[string]map[string]time.Time{}
	}

	// Never nil, so an empty projection marshals to [] and publishes the table
	// as empty rather than as nothing.
	public := make([]Plot, 0, len(names))
	private := make([]PrivatePlot, 0, len(names))

	for _, name := range names {
		u, err := g.store.GetUser(ctx, name)
		if err != nil {
			g.log.Warn("reading a user to publish", "nick", name, "err", err)
			continue
		}
		id := plotID(salt, name)
		if !named(u) {
			public = append(public, g.anonymousPlot(u, id))
			continue
		}
		public = append(public, g.publicPlot(u, id))
		if claimable(u) {
			private = append(private, g.privatePlot(u, runs[name]))
		}
	}
	return public, private, nil
}

// plotSalt is generated once and kept: an id that changed would move somebody's
// plot across the map on every restart.
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

// plotID is salted so holding the published table and a list of nicks does not
// let the two be matched up.
func plotID(salt, username string) string {
	sum := sha256.Sum256([]byte(salt + "\x00" + username))
	return base64.RawURLEncoding.EncodeToString(sum[:9])
}

// pendingRuns is every outstanding activity by user then kind, so one query
// covers everybody.
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

// isRun keeps housekeeping tasks off a player's page.
func isRun(kind string) bool {
	for _, r := range runs {
		if r == kind {
			return true
		}
	}
	return false
}

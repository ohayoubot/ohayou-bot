// Package ohayou is the ohayou game: a daily ration, a shop, industry, animals
// and thievery.
package ohayou

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
	"github.com/ohayoubot/ohayou-bot/internal/store"
	"github.com/ohayoubot/ohayou-bot/internal/task"
)

//go:embed schema.sql
var schema string

type Plugin struct {
	bot   *bot.Bot
	store Store
	db    store.Store
	log   *slog.Logger
	est   *time.Location

	tasks *task.Queue
	kv    *store.KV

	cfg      Config
	items    []store.Item
	fortunes []string

	baseCtx context.Context

	police *policeRegistry
	offers *offers

	// event state
	evtMu        sync.Mutex // the event guard on state
	doubleOhayou bool
	canAdoptCat  bool
	catAdopt     chan string
}

func New() *Plugin {
	return &Plugin{
		baseCtx:  context.Background(),
		police:   newPoliceRegistry(),
		offers:   newOffers(),
		catAdopt: make(chan string),
	}
}

func (g *Plugin) Name() string { return "ohayou" }

func (g *Plugin) Register(deps plugin.Deps) error {
	est, err := time.LoadLocation(g.cfg.Timezone)
	if err != nil {
		return fmt.Errorf("load timezone %q: %w", g.cfg.Timezone, err)
	}

	// The bot-wide store interface is deliberately narrow, so the game asks
	// whether what it was handed knows its domain rather than assuming.
	st, ok := deps.Store.(Store)
	if !ok {
		return fmt.Errorf("the store does not carry the ohayou tables")
	}
	if deps.Tasks == nil {
		return fmt.Errorf("the game needs somewhere to queue its activities")
	}

	g.bot, g.log, g.db, g.store, g.est = deps.Bot, deps.Log, deps.Store, st, est
	g.tasks, g.kv = deps.Tasks, deps.KV
	g.registerTasks(g.tasks)
	g.registerDoubleOhayou(g.tasks)

	g.bot.HandleFunc("ohayou", false, g.cmdOhayou)
	g.bot.HandleFunc("buy", false, g.cmdBuy)
	g.bot.HandleFunc("equip", false, g.cmdEquip)
	g.bot.HandleFunc("unequip", false, g.cmdUnequip)
	g.bot.HandleFunc("items", false, g.cmdItems)
	g.bot.HandleFunc("item", false, g.cmdItem)
	g.bot.HandleFunc("use", false, g.cmdUse)
	g.bot.HandleFunc("steal", false, g.cmdSteal)
	g.bot.HandleFunc("deposit", false, g.cmdDeposit)
	g.bot.HandleFunc("withdraw", false, g.cmdWithdraw)
	g.bot.HandleFunc("stats", false, g.cmdStats)
	g.bot.HandleFunc("top", false, g.cmdTop)
	g.bot.HandleFunc("build", false, g.cmdBuild)
	g.bot.HandleFunc("recipe", false, g.cmdRecipe)
	g.bot.HandleFunc("inventory", false, g.cmdInventory)
	g.bot.HandleFunc("register", false, g.cmdRegister)
	g.bot.HandleFunc("identify", false, g.cmdIdentify)
	g.bot.HandleFunc("quarry", false, g.cmdQuarry)
	g.bot.HandleFunc("report", false, g.cmdReport)
	g.bot.Help(helpTopics(g.p())...)

	g.log.Info("enabled", "timezone", g.cfg.Timezone, "fortunes", len(g.fortunes))
	return nil
}

// Start syncs the item catalog, resets state and starts the events.
func (g *Plugin) Start(ctx context.Context) error {
	if err := g.db.Migrate(ctx, g.Name(), schema); err != nil {
		return err
	}

	// Inserts new items and updates prices and other fields on existing ones,
	// so editing items.json and restarting is enough to change the catalog.
	if n, err := g.store.SeedItems(ctx, g.items); err != nil {
		return fmt.Errorf("seed items: %w", err)
	} else if n > 0 {
		g.log.Info("synced item catalog", "items", n)
	}

	g.baseCtx = ctx
	if err := g.reconcileRuns(ctx); err != nil {
		g.log.Error("reconciling activities", "err", err)
	}
	if err := g.resumeDoubleOhayou(ctx); err != nil {
		g.log.Error("resuming the distributor", "err", err)
	}

	g.log.Info("events started")
	g.bot.Go(func() { g.catEvent(ctx) })
	return nil
}

func (g *Plugin) p() string              { return g.bot.Prefix() }
func (g *Plugin) say(target, msg string) { g.bot.Say(target, msg) }

func (g *Plugin) ctx() context.Context { return g.baseCtx }

// startOfDay returns midnight in the timezone's calendar day
func startOfDay(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func randNum(min, max int) int {
	if max <= min {
		return min
	}
	return min + rand.IntN(max-min+1)
}

func plural(n int, unit string) string {
	if n == 1 {
		return unit
	}
	return unit + "s"
}

// mustIdentify returns the standard "please identify" message to a user registered
// with _the bot_, but without having identified to the bot yet.
func (g *Plugin) mustIdentify(u *store.User) (string, bool) {
	if u.Registered && !g.bot.Identified(u.Username) {
		return u.Username + ": You must be identified with me to do that. Make sure " +
			"you are identified with the network and then type " + g.p() + "identify.", true
	}
	return "", false
}

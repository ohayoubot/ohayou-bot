// Package game is the "ohayou" game.
package game

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/store"
)

type Game struct {
	bot   *bot.Bot
	store store.Store
	log   *slog.Logger
	est   *time.Location

	fortunes []string

	baseCtx context.Context

	mu            sync.RWMutex
	identified    map[string]bool
	watchingNicks bool

	police *policeRegistry

	// event state
	evtMu        sync.Mutex // the event guard on state
	doubleOhayou bool
	canAdoptCat  bool
	catAdopt     chan string
}

func New(b *bot.Bot, st store.Store, fortunes []string, log *slog.Logger) (*Game, error) {
	est, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("load timezone: %w", err)
	}
	return &Game{
		bot:        b,
		store:      st,
		log:        log,
		est:        est,
		fortunes:   fortunes,
		baseCtx:    context.Background(),
		identified: map[string]bool{},
		police:     newPoliceRegistry(),
		catAdopt:   make(chan string),
	}, nil
}

func (g *Game) Register() {
	g.bot.HandleFunc("help", false, g.cmdHelp)
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
}

// Start resets state and starts the events.
func (g *Game) Start(ctx context.Context) {
	g.baseCtx = ctx
	if err := g.store.ResetAllStatus(ctx); err != nil {
		g.log.Error("reset status", "err", err)
	}
	g.log.Info("game events started")
	g.bot.Go(func() { g.catEvent(ctx) })
	g.bot.Go(func() { g.doubleOhayouEvent(ctx) })
}

func (g *Game) p() string              { return g.bot.Prefix() }
func (g *Game) say(target, msg string) { g.bot.Say(target, msg) }

func (g *Game) ctx() context.Context { return g.baseCtx }

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

func (g *Game) isIdentified(nick string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.identified[nick]
}

func (g *Game) setIdentified(nick string, v bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if v {
		g.identified[nick] = true
	} else {
		delete(g.identified, nick)
	}
}

// mustIdentify returns the standard "please identify" message to a user registered
// with _the bot_, but without having identified to the bot yet.
func (g *Game) mustIdentify(u *store.User) (string, bool) {
	if u.Registered && !g.isIdentified(u.Username) {
		return u.Username + ": You must be identified with me to do that. Make sure " +
			"you are identified with the network and then type " + g.p() + "identify.", true
	}
	return "", false
}

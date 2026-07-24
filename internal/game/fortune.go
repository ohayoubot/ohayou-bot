package game

import (
	"math/rand/v2"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// fortune is the item function for fortune-telling items.
func (g *Game) fortune(u *store.User, itm string) string {
	if usedToday(u.LastUsed[itm], g.est) {
		return "- here's today's fortune again: " + u.Fortune
	}
	if err := g.store.SetLastUsed(g.ctx(), u.Username, itm, time.Now().In(g.est)); err != nil {
		g.log.Error("set last used", "err", err)
	}
	return "- " + g.newFortune(u.Username)
}

// newFortune picks a random fortune, persists it as the user's current fortune,
// and returns it.
func (g *Game) newFortune(username string) string {
	if len(g.fortunes) == 0 {
		return "The future is unclear."
	}
	f := g.fortunes[rand.IntN(len(g.fortunes))]
	if err := g.store.SetFortune(g.ctx(), username, f); err != nil {
		g.log.Error("set fortune", "nick", username, "err", err)
	}
	return f
}

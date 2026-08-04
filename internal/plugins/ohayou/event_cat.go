package ohayou

import (
	"context"
	"time"
)

// catEvent periodically spawns a stray cat in every channel that players can
// adopt with a burger or pancake.
func (g *Plugin) catEvent(ctx context.Context) {
	for {
		delay := time.Duration(randNum(7200, 28800)) * time.Second // 2-8 hours
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}

		channels := g.bot.Channels()
		g.log.Info("event: stray cat", "channels", len(channels))
		for _, c := range channels {
			g.say(c, "A stray cat appears! "+g.p()+"use burger or "+g.p()+
				"use pancake to adopt it!")
		}
		g.waitCatAdopt(ctx)
	}
}

// waitCatAdopt opens the adoption window and awaits the first taker (or a
// timeout).
func (g *Plugin) waitCatAdopt(ctx context.Context) {
	g.setCanAdopt(true)
	defer g.setCanAdopt(false)

	select {
	case nick := <-g.catAdopt:
		user, err := g.store.GetUser(g.ctx(), nick)
		if err != nil {
			g.log.Error("cat adopt: get user", "nick", nick, "err", err)
			return
		}
		g.log.Info("event: cat adopted", "nick", user.Username)
		for _, c := range g.bot.Channels() {
			g.say(c, user.Username+" adopts the cat!")
		}
		if err := g.store.AddCat(g.ctx(), user.Username, 1); err != nil {
			g.log.Error("add cat", "nick", user.Username, "err", err)
		}
	case <-time.After(15 * time.Second):
		g.log.Info("event: cat wandered off (no taker)")
		for _, c := range g.bot.Channels() {
			g.say(c, "The cat wanders off and disappears...")
		}
	case <-ctx.Done():
	}
}

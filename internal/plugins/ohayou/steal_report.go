package ohayou

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// reportWait is how long the police hang around for an answer.
const reportWait = 60 * time.Second

// cmdReport answers a robbery victim who took the police up on their offer.
// It is a registered command like any other, so it is listed by !commands and
// subject to the ignore list.
func (g *Plugin) cmdReport(m *bot.Message) {
	if !g.offers.take(strings.ToLower(m.Nick)) {
		g.say(m.ReplyTo(), m.Nick+": nobody is waiting on a report from you.")
	}
}

// stationPolice offers a robbery victim police protection and waits (up to a
// minute) for them to !report. defense is the victim's equipped defense at the
// time of the theft.
func (g *Plugin) stationPolice(username string, defense int) {
	if !g.police.reserve(username) {
		return
	}

	reported, done, ok := g.offers.open(username)
	if !ok {
		return
	}
	defer done()

	g.say(username, "Ohayou Police here. Looks like you were just the victim of a "+
		"robbery. If you report it, we can station one of our officers nearby for a "+
		"couple of hours. It'll reduce the chance of it happening again. Type "+
		g.p()+"report if you're interested.")

	timer := time.NewTimer(reportWait)
	defer timer.Stop()

	select {
	case <-reported:
		g.say(username, "Alright, we'll watch over you for a few hours.")
		g.protect(username, defense)
	case <-timer.C:
		g.say(username, "Guess you're not interested. Good luck out there.")
		g.police.remove(username)
	case <-g.baseCtx.Done():
		g.police.remove(username)
	}
}

// policeTick is how often a guard weakens.
const policeTick = time.Hour

// protect puts a guard on a user and queues its first decay. The guard outlives
// the process: it was paid for with a robbery, so a deploy should not end it.
func (g *Plugin) protect(username string, defense int) {
	// The user's chance of being stolen from after their own defensive items.
	ohayouChance := stealOhayouSuccess - defense/9
	catChance := stealCatSuccess - defense/9

	// Protection starts at 90% of that chance and loses a quarter each hour.
	ohayou := int(float64(ohayouChance) * 0.9)
	cat := int(float64(catChance) * 0.9)
	g.police.set(username, guard{
		Ohayou:    ohayou,
		Cat:       cat,
		DecOhayou: ohayou/4 + 1,
		DecCat:    cat/4 + 1,
		Since:     time.Now(),
	})
	g.savePolice(g.ctx())

	if err := g.tasks.After(g.ctx(), taskPolice, username, policeTick, ""); err != nil {
		g.log.Error("queue police decay", "nick", username, "err", err)
	}
}

// runPoliceDecay weakens a guard and queues the next one, or says goodbye when
// there is nothing left of it.
func (g *Plugin) runPoliceDecay(ctx context.Context, t store.Task) error {
	username := t.Key

	still := g.police.decay(username, time.Now(), policeTick)
	g.savePolice(ctx)

	if !still {
		g.say(username, "Ohayou Police here. We're leaving the vicinity now. Good luck.")
		return nil
	}
	return g.tasks.After(ctx, taskPolice, username, policeTick, "")
}

// savePolice writes the guards down. A failure costs the protection on the next
// restart but nothing right now, so it is logged rather than returned.
func (g *Plugin) savePolice(ctx context.Context) {
	raw, err := g.police.dump()
	if err != nil {
		g.log.Error("serialising police guards", "err", err)
		return
	}
	if err := g.kv.Set(ctx, policeKey, raw); err != nil {
		g.log.Error("saving police guards", "err", err)
	}
}

// resumePolice restores the guards a previous run left behind and makes sure
// each still has a decay queued, so none of them can last forever.
func (g *Plugin) resumePolice(ctx context.Context) error {
	switch raw, err := g.kv.Get(ctx, policeKey); {
	case err == nil:
		if err := g.police.restore(raw); err != nil {
			return err
		}
	case !errors.Is(err, store.ErrNotFound):
		return err
	}

	pending, err := g.tasks.Pending(ctx)
	if err != nil {
		return err
	}
	queued := map[string]bool{}
	for _, t := range pending {
		if t.Kind == taskPolice {
			queued[t.Key] = true
		}
	}

	guarded := g.police.protectedUsers()
	for _, user := range guarded {
		if !queued[user] {
			if err := g.tasks.After(ctx, taskPolice, user, policeTick, ""); err != nil {
				return err
			}
		}
	}
	if len(guarded) > 0 {
		g.log.Info("restored police guards", "count", len(guarded))
	}
	return nil
}

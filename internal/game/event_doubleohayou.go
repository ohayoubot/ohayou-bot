package game

import (
	"context"
	"time"
)

// doubleOhayouEvent periodically "malfunctions" the ohayou distributor,
// temporarily multiplying rations, then repairs it.
func (g *Game) doubleOhayouEvent(ctx context.Context) {
	for {
		delay := time.Duration(randNum(43200, 129600)) * time.Second // 12-36 hours
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}

		active := time.Duration(randNum(2, 10)) * time.Minute
		g.log.Info("event: double ohayou on", "duration", active)
		for _, c := range g.bot.Channels() {
			g.say(c, "ERROR: Ohayou distributor is malfunctioning.")
		}
		g.setDouble(true)

		select {
		case <-time.After(active):
		case <-ctx.Done():
			g.setDouble(false)
			return
		}

		g.setDouble(false)
		g.log.Info("event: double ohayou off")
		for _, c := range g.bot.Channels() {
			g.say(c, "Technicians have fixed the ohayou distributor. "+
				"It should be working as normal now.")
		}
	}
}

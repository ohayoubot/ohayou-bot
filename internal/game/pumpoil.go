package game

import (
	"fmt"
	"time"
)

// pumpOil. wells is how many oil wells the user owns (multiplies).
func (g *Game) pumpOil(username string, wells int) {
	g.setStatus(username, "pumping", true)
	defer g.setStatus(username, "pumping", false)

	select {
	case <-time.After(6 * time.Hour):
	case <-g.baseCtx.Done():
		return
	}

	amt := randNum(1, 9) * wells
	if wells > 1 {
		g.say(username, fmt.Sprintf("Your oil wells pumped %d barrels of crude oil.", amt))
	} else {
		g.say(username, fmt.Sprintf("Your oil well pumped %d barrels of crude oil.", amt))
	}

	if err := g.store.AddOil(g.ctx(), username, amt); err != nil {
		g.log.Error("add oil", "nick", username, "err", err)
	}
}

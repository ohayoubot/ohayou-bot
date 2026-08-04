package ohayou

import (
	"fmt"
	"time"
)

// pumpingTime is how long an oil well run takes.
const pumpingTime = 6 * time.Hour

// pumpOil pays out a finished well run. wells is how many the user owned when
// it started.
func (g *Plugin) pumpOil(username string, wells int) {
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

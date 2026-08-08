package ohayou

import (
	"fmt"
	"strings"
	"time"
)

// metalChance maps a metal to its percent chance of appearing in a mining action
var metalChance = map[string]int{
	"aluminum": 100,
	"iron":     90,
	"titanium": 80,
	"copper":   60,
	"lead":     50,
	"tin":      40,
	"uranium":  30,
	"silver":   25,
	"platinum": 20,
	"gold":     15,
}

// miningTime is how long a quarry run takes.
const miningTime = 8 * time.Hour

// mine pays out a finished mining run. quarries is how many the user owned when
// it started, as a yield multiplier.
func (g *Plugin) mine(username string, quarries int) {
	yield := make(map[string]int)
	var sb strings.Builder
	sb.WriteString("You mined ")
	for metal, chance := range metalChance {
		if randNum(0, 100) < chance {
			amt := (1 + chance/10) * quarries
			yield[metal] = amt
			fmt.Fprintf(&sb, "%d %s, ", amt, metal)
		}
	}

	g.say(username, strings.TrimSuffix(sb.String(), ", "))
	if err := g.store.AddMetals(g.ctx(), username, yield); err != nil {
		g.log.Error("add metals", "nick", username, "err", err)
		return
	}
	if metal := rarest(yield); metal != "" {
		g.record(eventStrike, username, "", map[string]string{"metal": metal})
	}
}

// rarest is the least likely metal a run turned up, or "" when none of it is
// worth writing down. Aluminum every eight hours is not news.
func rarest(yield map[string]int) string {
	best, at := "", strikeWorthy+1
	for metal := range yield {
		if chance := metalChance[metal]; chance < at {
			best, at = metal, chance
		}
	}
	return best
}

// strikeWorthy is the percent chance at or below which a metal is a strike:
// uranium, silver, platinum, gold.
const strikeWorthy = 30

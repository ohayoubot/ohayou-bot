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

// mine start a mining action. quarries is how many quarries the user owns (as a yield multiplier)
func (g *Plugin) mine(username string, quarries int) {
	g.setStatus(username, "mining", true)
	defer g.setStatus(username, "mining", false)

	select {
	case <-time.After(8 * time.Hour):
	case <-g.baseCtx.Done():
		return
	}

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
	}
}

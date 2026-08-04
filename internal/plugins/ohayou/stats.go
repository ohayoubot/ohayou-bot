package ohayou

import (
	"fmt"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

func (g *Plugin) stats(u *store.User) {
	if msg, blocked := g.mustIdentify(u); blocked {
		g.say(u.Username, msg)
		return
	}

	var totalItems, itemsAddOhayous, totalItemsCost int
	for itm, amt := range u.Items {
		multiplier := 1
		if u.ItemMultiply[itm] != 0 {
			multiplier = u.ItemMultiply[itm]
		}
		item, err := g.store.GetItem(g.ctx(), itm)
		if err != nil {
			continue
		}
		totalItems += amt
		totalItemsCost += item.Price * amt
		itemsAddOhayous += (item.Add * amt) * multiplier
	}

	totalDefense := userDefense(u)
	defenseOhayous := int(100 * (1 - float64(stealOhayouSuccess-(totalDefense/9))/stealOhayouSuccess))
	defenseCats := int(100 * (1 - float64(stealCatSuccess-(totalDefense/9))/stealCatSuccess))

	if ohyBonus, catBonus, protected := g.police.bonus(u.Username); protected {
		defenseOhayous += int(100 * (float64(ohyBonus) / stealOhayouSuccess))
		defenseCats += int(100 * (float64(catBonus) / stealCatSuccess))
	}

	g.say(u.Username, fmt.Sprintf(
		"Ohayous: %d on hand. Ohayou'd %d times for %d total. Items: %d owned (%d ohayous spent), adding +%d to every ohayou.",
		u.Ohayous, u.TimesOhayoued, u.CumOhayous, totalItems, totalItemsCost,
		itemsAddOhayous))

	if u.Vault.Installed {
		g.say(u.Username, fmt.Sprintf("Vault: level %d, holding %d/%d ohayous.",
			u.Vault.Level+1, u.Vault.Ohayous, vaultCap(u.Vault.Level)))
	}

	defense := fmt.Sprintf("Defense: %d from %d equipped items",
		armorDefense(u), len(u.Equipped))
	if dogs := dogDefense(u); dogs > 0 {
		defense += fmt.Sprintf(" and %d from %d %s", dogs, u.Items["dog"],
			plural(u.Items["dog"], "dog"))
	}
	g.say(u.Username, fmt.Sprintf("%s, cutting ohayou theft by %d%% and cat theft by %d%%.",
		defense, defenseOhayous, defenseCats))

	g.say(u.Username, fmt.Sprintf(
		"Stealing: %d of %d attempts landed, %d ohayous taken. Probation: %d %s served. Robbed for %d ohayous.",
		u.StealSuccess, u.StealSuccess+u.StealFail, u.StolenOhayous,
		u.ProbationCount, plural(u.ProbationCount, "day"), u.OhayousStolen))
}

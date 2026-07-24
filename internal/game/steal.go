package game

import (
	"fmt"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// TODO: should this be a config element?
const (
	// percent chance thresholds for a successful steal
	stealOhayouSuccess = 25
	stealCatSuccess    = 25

	stealFineMin   = 5
	stealFinePct   = 0.16
	stealAmountPct = 0.07

	// minimum number of ohayous a user must have done to steal
	stealOhayouMin = 5
)

func stealFine(u *store.User) int   { return stealFineMin + int(float64(u.Ohayous)*stealFinePct) }
func stealAmount(u *store.User) int { return int(float64(u.Ohayous) * stealAmountPct) }

func userDefense(u *store.User) int {
	var defense int
	for _, item := range u.Equipped {
		defense += item.Defense
	}
	return defense
}

func (g *Game) stealFrom(thief, victim *store.User, channel, nickRaw, vicRaw string) {
	if msg, blocked := g.mustIdentify(thief); blocked {
		g.say(channel, msg)
		return
	}

	if thief.Probation.In(g.est).Format("200601021504") >=
		time.Now().In(g.est).Format("200601021504") {
		g.say(channel, fmt.Sprintf("%s: you are still on probation from your last "+
			"theft attempt. Your probation expires on %v EST.",
			nickRaw, thief.Probation.In(g.est).Format("Jan 2 15:04")))
		return
	}

	stealOhayouChance := randNum(0, 100)
	stealCatChance := 101 // impossible unless the victim has a cat
	if victim.Items["cat"] > 0 {
		stealCatChance = randNum(0, 100)
	}

	defenseBonus := userDefense(victim) / 9
	stealOhayouChance += defenseBonus
	stealCatChance += defenseBonus

	if ohyBonus, catBonus, protected := g.police.bonus(victim.Username); protected {
		stealOhayouChance += ohyBonus
		stealCatChance += catBonus
	}

	if thief.TimesOhayoued < stealOhayouMin {
		remaining := stealOhayouMin - thief.TimesOhayoued
		g.say(channel, fmt.Sprintf("%s: you haven't ohayou'd enough to do that yet! "+
			"Ohayou %d more %s.", nickRaw, remaining, plural(remaining, "time")))
		return
	}
	if thief.Ohayous < stealFineMin {
		g.say(channel, fmt.Sprintf("%s: you don't have enough ohayous to steal. "+
			"You need at least %d.", nickRaw, stealFineMin))
		return
	}

	fine := stealFine(thief)
	if victim.Ohayous == 0 {
		g.say(channel, fmt.Sprintf("%s attempts to steal from %s but %s doesn't have "+
			"any ohayous! %s is fined %d ohayous and placed on probation for 24 hours.",
			nickRaw, vicRaw, vicRaw, nickRaw, fine))
		g.failSteal(thief.Username, fine)
		return
	}

	ohayouOK := stealOhayouChance <= stealOhayouSuccess
	catOK := stealCatChance <= stealCatSuccess
	amount := stealAmount(victim)

	switch {
	case !ohayouOK && !catOK:
		g.say(channel, fmt.Sprintf("%s attempts to steal from %s but is caught! "+
			"%s is fined %d ohayous and is placed on probation for 24 hours.",
			nickRaw, vicRaw, nickRaw, fine))
		g.failSteal(thief.Username, fine)
	case ohayouOK && !catOK:
		g.say(channel, fmt.Sprintf("%s attempts to steal from %s and succeeds! "+
			"%s steals %d ohayous from %s.", nickRaw, vicRaw, nickRaw, amount, vicRaw))
		g.successSteal(thief.Username, victim.Username, 0, amount)
		g.goDo(func() { g.stationPolice(victim.Username, userDefense(victim)) })
	case !ohayouOK && catOK:
		g.say(channel, fmt.Sprintf("%s attempts to steal from %s and succeeds! "+
			"%s steals a cat from %s.", nickRaw, vicRaw, nickRaw, vicRaw))
		g.successSteal(thief.Username, victim.Username, 1, 0)
		g.goDo(func() { g.stationPolice(victim.Username, userDefense(victim)) })
	default: // both succeed
		g.say(channel, fmt.Sprintf("%s attempts to steal from %s and succeeds! "+
			"%s steals a cat and %d ohayous from %s.",
			nickRaw, vicRaw, nickRaw, amount, vicRaw))
		g.successSteal(thief.Username, victim.Username, 1, amount)
		g.goDo(func() { g.stationPolice(victim.Username, userDefense(victim)) })
	}
}

func (g *Game) failSteal(nick string, fine int) {
	probation := time.Now().Add(24 * time.Hour).In(g.est)
	if err := g.store.SaveFailSteal(g.ctx(), nick, fine, probation); err != nil {
		g.log.Error("save fail steal", "nick", nick, "err", err)
	}
}

func (g *Game) successSteal(thief, victim string, cat, ohy int) {
	if err := g.store.SaveSuccessSteal(g.ctx(), thief, victim, cat, ohy); err != nil {
		g.log.Error("save success steal", "thief", thief, "victim", victim, "err", err)
	}
}

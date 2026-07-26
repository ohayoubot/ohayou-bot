package game

import (
	"fmt"
	"time"
)

// catsPerAcre bounds how many cats a user can keep per acre. It must match the
// "cat" item's acrelimit in data/items.json so breeding respects the same cap
// that buying does.
const catsPerAcre = 20

// breedCat runs the multi-hour cat breeding activity for a user. cattery is the
// number of catteries the user owns (the litter multiplier).
func (g *Game) breedCat(username string, cattery int) {
	g.setStatus(username, "breeding", true)
	defer g.setStatus(username, "breeding", false)

	// 2 - 4 hours
	delay := time.Duration(randNum(7200, 14400)) * time.Second
	select {
	case <-time.After(delay):
	case <-g.baseCtx.Done():
		return
	}

	// reload so the cap and the dog roll below use the latest counts
	user, err := g.store.GetUser(g.ctx(), username)
	if err != nil {
		g.log.Error("breed reload", "nick", username, "err", err)
		return
	}

	if g.dogAttacksCat(user) {
		g.say(username, "While the cats were busy, your dog got in among them and "+
			"killed one! You are down a cat, "+username+".")
	}

	if randNum(0, 10) <= 3 {
		g.say(username, "Darn! Looks like your cats didn't mate, "+username+
			". Maybe next time!")
		return
	}

	// Respect the land-based cap.
	room := catsPerAcre*user.Items["acre"] - user.Items["cat"]
	if room <= 0 {
		g.say(username, "Your cats mated, but there's no room for kittens! Buy more "+
			"land before breeding again.")
		return
	}

	litter := randNum(2, 7) * cattery
	if litter > room {
		litter = room
	}
	g.say(username, fmt.Sprintf("Congratulations %s! Your cats successfully mated! "+
		"Amazingly, the cats were born instantaneously! You receive %d cats.",
		username, litter))
	if err := g.store.AddCat(g.ctx(), username, litter); err != nil {
		g.log.Error("add cat", "nick", username, "err", err)
	}
}

// setStatus is a logging wrapper around the store call.
func (g *Game) setStatus(username, action string, active bool) {
	if err := g.store.SetStatus(g.ctx(), username, action, active); err != nil {
		g.log.Error("set status", "nick", username, "action", action, "err", err)
	}
}

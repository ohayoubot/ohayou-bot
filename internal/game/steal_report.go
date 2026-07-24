package game

import (
	"strings"
	"time"

	irc "github.com/ohayoubot/go-ircevent"
)

// stationPolice offers a robbery victim police protection and waits (up to a
// minute) for them to !report. defense is the victim's equipped defense at the
// time of the theft.
func (g *Game) stationPolice(username string, defense int) {
	if !g.police.reserve(username) {
		return
	}

	g.say(username, "Ohayou Police here. Looks like you were just the victim of a "+
		"robbery. If you report it, we can station one of our officers nearby for a "+
		"couple of hours. It'll reduce the chance of it happening again. Type "+
		g.p()+"report if you're interested.")

	timer := time.NewTimer(60 * time.Second)
	defer timer.Stop()

	reported := make(chan struct{}, 1)
	id := g.bot.AddCallback("PRIVMSG", func(e *irc.Event) {
		fields := strings.Fields(e.Message())
		if len(fields) == 0 {
			return
		}
		if strings.ToLower(e.Nick) == username && fields[0] == g.p()+"report" {
			select {
			case reported <- struct{}{}:
			default:
			}
		}
	})
	defer g.bot.RemoveCallback("PRIVMSG", id)

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

// protect activates and slowly decays anti-theft protection for a user.
func (g *Game) protect(username string, defense int) {
	// The user's chance of being stolen from after their own defensive items.
	ohayouChance := stealOhayouSuccess - defense/9
	catChance := stealCatSuccess - defense/9

	// Protection starts at 90% of that chance and decays a quarter each hour.
	g.police.set(username, int(float64(ohayouChance)*0.9), int(float64(catChance)*0.9))
	decOhayou := int(float64(ohayouChance)*0.9)/4 + 1
	decCat := int(float64(catChance)*0.9)/4 + 1

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if !g.police.decay(username, decOhayou, decCat) {
				g.say(username, "Ohayou Police here. We're leaving the vicinity now. "+
					"Good luck.")
				return
			}
		case <-g.baseCtx.Done():
			g.police.remove(username)
			return
		}
	}
}

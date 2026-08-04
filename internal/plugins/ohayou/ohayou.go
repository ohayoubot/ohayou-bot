package ohayou

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

var adjectives = [...]string{
	"Great", "Superb", "Fantastic", "Amazing", "Marvelous", "Stunning",
	"Splendid", "Exquisite", "Impressive", "Outstanding", "Wonderful",
}

func (g *Plugin) Ohayou(nick string) string {
	ctx := g.ctx()

	ohayous := randNum(0, 6)
	var typeResponse string
	switch ohayous {
	case 0:
		typeResponse = "But not good enough. You get 0 ohayous."
	case 1:
		typeResponse = "You get 1 ohayou."
	case 6:
		typeResponse = "Wow! You get 6 ohayous!"
	default:
		typeResponse = fmt.Sprintf("You get %d ohayous!", ohayous)
	}

	user, err := g.store.GetUser(ctx, nick)
	if errors.Is(err, store.ErrNotFound) {
		const firstRation = 12
		if err := g.store.CreateUser(ctx, nick, firstRation); err != nil {
			g.log.Error("create user", "nick", nick, "err", err)
			return "Something went wrong creating your account. Try again."
		}
		return "Congratulations on your first ohayou " + nick + "!!! You get 12 " +
			"ohayous, which means you can buy your first cat! Type " + g.p() + "buy " +
			"cat to do so, " + g.p() + "item cat to see what cats do, and " + g.p() +
			"items to see what else is for sale. You can also type " + g.p() +
			"help ohayou to see other available commands."
	} else if err != nil {
		g.log.Error("get user", "nick", nick, "err", err)
		return "Something went wrong. Try again."
	}

	now := time.Now().In(g.est)
	if user.Last.In(g.est).Format("20060102") >= now.Format("20060102") {
		howLong := 24 - now.Hour()
		return fmt.Sprintf("You already got your ohayou ration today, %s. Try again "+
			"after midnight EST (in %d %s).", nick, howLong, plural(howLong, "hour"))
	}

	if msg, blocked := g.mustIdentify(user); blocked {
		return msg
	}

	itemOhayous := 0
	for itm, amt := range user.Items {
		itemMultiplier := 1
		if user.ItemMultiply[itm] != 0 {
			itemMultiplier = user.ItemMultiply[itm]
		}
		item, err := g.store.GetItem(ctx, itm)
		if err != nil {
			continue
		}
		itemOhayous += (item.Add * amt) * itemMultiplier
	}

	ration := ohayous + itemOhayous

	double := g.getDouble()
	if double {
		// scale the _ration_ by 1.3-2.1x
		factor := float64(randNum(13, 21)) / 10
		ration = int(float64(ration) * factor)
	}

	newTotal := user.Ohayous + ration
	// addedCum is the ration only so cum_ohayous tracks lifetime earnings
	err = g.store.SaveOhayou(ctx, nick, newTotal, ration, now, startOfDay(now, g.est))
	if errors.Is(err, store.ErrInsufficient) {
		// already ohayou'd or doing something sus
		return fmt.Sprintf("You already got your ohayou ration today, %s.", nick)
	}
	if err != nil {
		g.log.Error("save ohayou", "nick", nick, "err", err)
		return "Something went wrong saving your ohayous. Try again."
	}

	if double {
		return fmt.Sprintf("ERROR C0045: <%s> <%d> OHAYOUS DISPENSED",
			strings.ToUpper(nick), ration)
	}
	if itemOhayous == 0 {
		return fmt.Sprintf("%s ohayou %s!!! %s You have %d ohayous.",
			adjectives[randNum(0, len(adjectives)-1)], nick, typeResponse, newTotal)
	}
	return fmt.Sprintf("%s ohayou %s!!! %s Your items increased that to %d. "+
		"You have %d ohayous.",
		adjectives[randNum(0, len(adjectives)-1)], nick, typeResponse,
		ohayous+itemOhayous, newTotal)
}

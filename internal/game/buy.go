package game

import (
	"errors"
	"fmt"
	"math"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// freeAcre returns whether the user has at least amt acres not already consumed
// by acre-limited items (used for items that require their own acre). The acres
// each item type occupies must be summed: checking every type independently
// (and returning on the first that fits) only accounted for the single
// largest-consuming type, so a full plot could still host another quarry.
func (g *Game) freeAcre(u *store.User, amt int) bool {
	ctx := g.ctx()
	var usedAcres int
	for itm, uAmt := range u.Items {
		item, err := g.store.GetItem(ctx, itm)
		if err != nil || item.Acrelimit <= 0 {
			continue
		}
		usedAcres += int(math.Ceil(float64(uAmt) / float64(item.Acrelimit)))
	}
	return (u.Items["acre"] - usedAcres) >= amt
}

// buy attempts to purchase amt of itm for u and returns the result message.
func (g *Game) buy(u *store.User, itm string, amt int) string {
	ctx := g.ctx()

	item, err := g.store.GetItem(ctx, itm)
	if err != nil {
		return "I don't have that in stock."
	}
	if !item.Purchase {
		return "That's not for sale."
	}
	if amt <= 0 {
		return "That's not a valid quantity."
	}
	// check ffordability with division to avoid an overflow
	if item.Price > 0 && amt > u.Ohayous/item.Price {
		return "You can't afford that."
	}
	if item.Acrelimit > 0 && (u.Items[itm]+amt) > (item.Acrelimit*u.Items["acre"]) {
		return fmt.Sprintf("You need more land to purchase more of that! You can "+
			"only have %d %ss per acre and you have %d %ss and %d acre(s).",
			item.Acrelimit, itm, u.Items[itm], itm, u.Items["acre"])
	}
	if item.Limit > 0 && u.Items[itm] >= item.Limit {
		return fmt.Sprintf("You can't purchase any more of that. You can only have"+
			" %d %s", item.Limit, itm)
	}
	if item.Limit > 0 && u.Items[itm]+amt > item.Limit {
		return fmt.Sprintf("You can't purchase that much. You can only have"+
			" %d %s", item.Limit, itm)
	}
	if msg, blocked := g.mustIdentify(u); blocked {
		return msg
	}
	if item.NeedsAcre && !g.freeAcre(u, amt) {
		return u.Username + ": That item requires its own acre, and you do not have " +
			"an empty acre."
	}

	err = g.store.AddItem(ctx, u.Username, *item, amt)
	if errors.Is(err, store.ErrInsufficient) {
		// The snapshot said affordable but the atomic debit disagreed (a
		// concurrent spend drained the balance first).
		return "You can't afford that."
	}
	if err != nil {
		g.log.Error("add item", "nick", u.Username, "item", itm, "err", err)
		return "Something went wrong with your purchase. Try again."
	}

	ohayous_left := u.Ohayous - (item.Price * amt)
	return fmt.Sprintf("You purchased %d %s for %d ohayous. You have %d %s left.",
		amt, plural(amt, itm), item.Price*amt, ohayous_left, plural(ohayous_left, "ohayou"))
}

// findMax returns the largest quantity of itm the user can currently buy,
// respecting affordability and per-acre limits.
func (g *Game) findMax(u *store.User, itm string) int {
	item, err := g.store.GetItem(g.ctx(), itm)
	if err != nil {
		return 1
	}

	absMax := math.Floor(float64(u.Ohayous) / float64(item.Price))
	if absMax == 0 {
		return 1
	}

	if item.Acrelimit > 0 {
		itemLimit := item.Acrelimit * u.Items["acre"]
		if u.Items[itm] >= itemLimit {
			return 1
		}
		if (itemLimit - u.Items[itm]) < int(absMax) {
			return itemLimit - u.Items[itm]
		}
	}
	return int(absMax)
}

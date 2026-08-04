package ohayou

import (
	"strings"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// use applies an item's effect for a user and returns the announcement.
func (g *Plugin) use(u *store.User, nick, itm, on string) string {
	ctx := g.ctx()

	if msg, blocked := g.mustIdentify(u); blocked {
		return msg
	}

	item, err := g.store.GetItem(ctx, itm)
	if err != nil {
		return itm + " isn't an item. Type " + g.p() + "items to look at what's" +
			" available, and " + g.p() + "inventory to see what items you have."
	}
	if u.Items[item.Name] == 0 {
		return "You don't have any of that."
	}
	if !item.Useable {
		return item.Name + " is a passive item. It can't be used."
	}

	if item.Consume {
		if err := g.store.ConsumeItem(ctx, u.Username, item.Name); err != nil {
			g.log.Error("consume item", "nick", u.Username, "item", item.Name, "err", err)
		}
	}

	if item.Name == "vault" && u.Vault.Installed {
		return ""
	}

	var extra string
	if item.HasFunction != "" {
		extra = g.runItemFunc(item.HasFunction, u, item.Name)
	}

	if item.HasFunction == "adoptCat" && g.getCanAdopt() {
		return nick + " offers the cat a " + item.Name + "..."
	}

	return nick + " " + strings.ReplaceAll(item.Effect, "%s", on) + extra
}

// runItemFunc dispatches an item's special function by name, i.e. maps the name in items.json
// to an actual function. this is manually maintained.
func (g *Plugin) runItemFunc(name string, u *store.User, itm string) string {
	switch name {
	case "goldenRooster":
		return g.goldenRooster(u, itm)
	case "adoptCat":
		return g.adoptCat(u)
	case "fortune":
		return g.fortune(u, itm)
	case "makeVault":
		return g.makeVault(u)
	case "upgradeVault":
		return g.upgradeVault(u)
	case "attemptBreedCat":
		return g.attemptBreedCat(u)
	case "startMining":
		return g.startMining(u)
	case "startPumping":
		return g.startPumping(u)
	case "walkDog":
		return g.walkDog(u, itm)
	default:
		return ""
	}
}

func (g *Plugin) goldenRooster(u *store.User, itm string) string {
	if usedToday(u.LastUsed[itm], g.est) {
		return " but the rooster has already crowed today."
	}
	if err := g.store.SetLastUsed(g.ctx(), u.Username, itm, time.Now().In(g.est)); err != nil {
		g.log.Error("set last used", "err", err)
	}
	if err := g.store.ResetLast(g.ctx(), u.Username); err != nil {
		g.log.Error("reset last", "err", err)
	}
	return " and shortly thereafter feels good enough to " + g.p() + "ohayou again."
}

func (g *Plugin) adoptCat(u *store.User) string {
	if g.getCanAdopt() {
		select {
		case g.catAdopt <- u.Username:
		default:
		}
	}
	return ""
}

func (g *Plugin) makeVault(u *store.User) string {
	if err := g.store.InstallVault(g.ctx(), u.Username); err != nil {
		g.log.Error("install vault", "nick", u.Username, "err", err)
	}
	return ""
}

func (g *Plugin) attemptBreedCat(u *store.User) string {
	if u.Items["cat"] < 2 {
		return " but doesn't have any cats to breed! What are you doing! You need " +
			"at least two cats to do that."
	}
	if u.Status["breeding"] {
		return " but already has cats in there! You must wait until they are finished."
	}
	g.bot.Go(func() { g.breedCat(u.Username, u.Items["cattery"]) })
	return " for a few hours."
}

func (g *Plugin) startMining(u *store.User) string {
	if u.Status["mining"] {
		return " but is already mining! Wait until it's finished and try again."
	}
	g.bot.Go(func() { g.mine(u.Username, u.Items["quarry"]) })
	return " for a few hours."
}

func (g *Plugin) startPumping(u *store.User) string {
	if u.Status["pumping"] {
		return " but is already pumping oil! Wait until it's finished and try again."
	}
	g.bot.Go(func() { g.pumpOil(u.Username, u.Items["oilwell"]) })
	return " for a few hours."
}

// usedToday reports whether ts falls on the current EST calendar day.
func usedToday(ts time.Time, est *time.Location) bool {
	return ts.In(est).Format("20060102") >= time.Now().In(est).Format("20060102")
}

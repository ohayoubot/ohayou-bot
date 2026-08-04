package ohayou

import (
	"errors"
	"fmt"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

const (
	dogDefenseEach = 12 // per dog per acre
	dogDefenseMax  = 36 // max possible contribution
	dogAttackEach  = 5  // percent per dog to kill a cat
	dogAttackMax   = 25 // cap cat killing
	dogDigNothing  = 45 // percent dig up nothing
	dogDigRare     = 95 // roll needed to find gold
)

var digMetals = [...]string{"aluminum", "iron", "copper", "tin", "lead"}

func dogDefense(u *store.User) int {
	defense := u.Items["dog"] * dogDefenseEach
	if defense > dogDefenseMax {
		return dogDefenseMax
	}
	return defense
}

func (g *Plugin) dogAttacksCat(u *store.User) bool {
	if u.Items["dog"] == 0 || u.Items["cat"] == 0 {
		return false
	}

	chance := u.Items["dog"] * dogAttackEach
	if chance > dogAttackMax {
		chance = dogAttackMax
	}
	if randNum(1, 100) > chance {
		return false
	}

	if err := g.store.RemoveCat(g.ctx(), u.Username, 1); err != nil {
		// cat went somewhere else (stolen, say) in a race
		if !errors.Is(err, store.ErrInsufficient) {
			g.log.Error("dog attack", "nick", u.Username, "err", err)
		}
		return false
	}
	u.Items["cat"]--
	return true
}

func (g *Plugin) walkDog(u *store.User, itm string) string {
	if usedToday(u.LastUsed[itm], g.est) {
		return " but it has already had its walk today."
	}
	if err := g.store.SetLastUsed(g.ctx(), u.Username, itm, time.Now().In(g.est)); err != nil {
		g.log.Error("set last used", "nick", u.Username, "err", err)
	}

	roll := randNum(1, 100)
	if roll <= dogDigNothing {
		return " and it comes back with nothing but a muddy stick."
	}

	metal, amt := digMetals[randNum(0, len(digMetals)-1)], randNum(1, 2)*u.Items["dog"]
	if roll > dogDigRare {
		metal, amt = "gold", u.Items["dog"]
	}
	if err := g.store.AddMetals(g.ctx(), u.Username, map[string]int{metal: amt}); err != nil {
		g.log.Error("dog dig", "nick", u.Username, "err", err)
		return " and it digs a hole, but you can't make out what's in it."
	}
	return fmt.Sprintf(" and it digs up %d %s, which goes in with your metals!",
		amt, metal)
}

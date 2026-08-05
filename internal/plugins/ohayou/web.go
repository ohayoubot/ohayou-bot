package ohayou

import (
	"math"
	"sort"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// What the website may be told about a player. The two tiers are computed here
// rather than in a worker so the game decides what leaves it, and so the rules
// stay next to the ones they are derived from.

// Plot is what anyone may see. Its field list is the boundary between the game
// and the internet: web_test pins the marshalled keys, so a field added to
// store.User cannot reach the site by being carried along.
type Plot struct {
	Account string `json:"account"`
	Nick    string `json:"nick"`
	Acres   int    `json:"acres"`
	// Land is what occupies those acres, by item name. Anything that defends is
	// left out: a public map of who has no dog is a list of who to rob. What is
	// left out looks like empty land, so nothing can be inferred from the total.
	Land map[string]int `json:"land"`
	// Wealth is a band rather than a number, and comes from lifetime earnings
	// rather than the balance, so it ranks players without telling a thief what
	// is worth taking today.
	Wealth string `json:"wealth"`
	// Rations is how many days they have collected, which is the closest thing
	// the schema has to how long they have played.
	Rations int `json:"rations"`
}

// PrivatePlot is what one player may see about themselves, and is served only
// against a session holding the matching account.
type PrivatePlot struct {
	Account    string            `json:"account"`
	Nick       string            `json:"nick"`
	Ohayous    int               `json:"ohayous"`
	Cumulative int               `json:"cumulative"`
	Items      map[string]int    `json:"items"`
	Metals     map[string]int    `json:"metals"`
	Equipped   map[string]string `json:"equipped"`
	Defense    int               `json:"defense"`
	Vault      *VaultView        `json:"vault,omitempty"`
	Probation  int64             `json:"probation,omitempty"`
	Fortune    string            `json:"fortune"`
	// Running are the activities still counting down, which is the thing the
	// site can tell a player that a channel line cannot.
	Running []Run `json:"running"`
}

type VaultView struct {
	Level   int `json:"level"`
	Ohayous int `json:"ohayous"`
	Cap     int `json:"cap"`
}

// Run is an activity that has not finished yet.
type Run struct {
	Kind string `json:"kind"`
	Due  int64  `json:"due"`
}

// wealthBands are checked in order, so each below is the ceiling of its band.
var wealthBands = []struct {
	below int
	name  string
}{
	{500, "newcomer"},
	{2500, "settler"},
	{10000, "landowner"},
	{50000, "industrialist"},
	{200000, "magnate"},
	{math.MaxInt, "tycoon"},
}

func wealth(cumulative int) string {
	for _, band := range wealthBands {
		if cumulative < band.below {
			return band.name
		}
	}
	return wealthBands[len(wealthBands)-1].name
}

// publishable is the consent check made before a plot is computed at all, so a
// user who has not agreed is never rendered, not merely filtered later. An
// account is required as well: without one there is no identity on the site
// that a nick change cannot move.
func publishable(u *store.User) bool {
	return u.Web == store.VisibilityPublic && u.Account != ""
}

func (g *Plugin) publicPlot(u *store.User) Plot {
	return Plot{
		Account: u.Account,
		Nick:    u.Username,
		Acres:   u.Items["acre"],
		Land:    g.land(u),
		Wealth:  wealth(u.CumOhayous),
		Rations: u.TimesOhayoued,
	}
}

// land is the items that take up room, which is what a map can draw. The
// catalog decides both halves: an acre limit means it occupies land, and any
// defense at all keeps it off the public map.
func (g *Plugin) land(u *store.User) map[string]int {
	ctx := g.ctx()
	out := map[string]int{}
	for itm, amt := range u.Items {
		if amt <= 0 {
			continue
		}
		item, err := g.store.GetItem(ctx, itm)
		if err != nil || item.Acrelimit <= 0 || item.Defense > 0 {
			continue
		}
		out[itm] = amt
	}
	return out
}

// privatePlot takes the pending runs rather than reading them, so publishing a
// thousand players costs one query for all of them instead of one each.
func (g *Plugin) privatePlot(u *store.User, runs map[string]time.Time) PrivatePlot {
	p := PrivatePlot{
		Account:    u.Account,
		Nick:       u.Username,
		Ohayous:    u.Ohayous,
		Cumulative: u.CumOhayous,
		Items:      map[string]int{},
		Metals:     map[string]int{},
		Equipped:   map[string]string{},
		Defense:    userDefense(u),
		Fortune:    u.Fortune,
		Running:    []Run{},
	}
	for itm, amt := range u.Items {
		if amt > 0 {
			p.Items[itm] = amt
		}
	}
	for metal, amt := range u.Quarry.Metals {
		if amt > 0 {
			p.Metals[metal] = amt
		}
	}
	for category, item := range u.Equipped {
		p.Equipped[category] = item.Name
	}
	if u.Vault.Installed {
		// Level+1 is what !stats shows, and the two must not disagree.
		p.Vault = &VaultView{
			Level:   u.Vault.Level + 1,
			Ohayous: u.Vault.Ohayous,
			Cap:     vaultCap(u.Vault.Level),
		}
	}
	if u.Probation.After(time.Now()) {
		p.Probation = u.Probation.Unix()
	}
	for kind, due := range runs {
		p.Running = append(p.Running, Run{Kind: kind, Due: due.Unix()})
	}
	sort.Slice(p.Running, func(i, j int) bool { return p.Running[i].Kind < p.Running[j].Kind })
	return p
}

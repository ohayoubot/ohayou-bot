package ohayou

import (
	"math"
	"sort"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// What the website may be told about a player. Computed here rather than in a
// worker so the game decides what leaves it.

// Plot is what anyone may see. The field list is the boundary between the game
// and the internet: web_test pins the marshalled keys, so a field added to
// store.User cannot reach the site by being carried along.
//
// Every player has one. A plot carries a name only when its owner said it
// could; the rest are the same shape with the identifying half left out.
type Plot struct {
	// ID is the services account for a named plot, and a salted id otherwise.
	ID string `json:"id"`
	// Nick is empty unless the owner agreed to be named.
	Nick  string `json:"nick"`
	Named bool   `json:"named"`
	// Flag is a deer from the gallery. Empty on an unnamed plot: a chosen
	// picture identifies its owner as well as a nick does.
	Flag  string `json:"flag"`
	Acres int    `json:"acres"`
	// Land is what occupies those acres, and is empty on an unnamed plot: a
	// distinctive set of buildings names you as well as a nick does. Anything
	// that defends is left out either way, since a map of who has no dog is a
	// list of who to rob. What is left out reads as empty land.
	Land map[string]int `json:"land"`
	// Wealth is a band, from lifetime earnings rather than the balance: it ranks
	// players without saying what is worth taking today.
	Wealth string `json:"wealth"`
	// Rations is days collected, the closest the schema has to how long they
	// have played.
	Rations int `json:"rations"`
}

// PrivatePlot is what one player may see about themselves, served only against
// a session holding the matching account.
//
// Every column is always present: omitempty would drop a zero, and the site
// requires the column, so a player with no probation would be refused.
type PrivatePlot struct {
	Account    string            `json:"account"`
	Nick       string            `json:"nick"`
	Ohayous    int               `json:"ohayous"`
	Cumulative int               `json:"cumulative"`
	Items      map[string]int    `json:"items"`
	Metals     map[string]int    `json:"metals"`
	Equipped   map[string]string `json:"equipped"`
	Defense    int               `json:"defense"`
	Vault      *VaultView        `json:"vault"`
	Probation  int64             `json:"probation"`
	Fortune    string            `json:"fortune"`
	// Running are the activities still counting down.
	Running []Run `json:"running"`
}

// Event is one line of the chronicle as the site is given it. Actor and Subject
// are empty for anyone whose plot carries no name, so the feed says as much
// about a player as the map does and no more.
type Event struct {
	ID      int64             `json:"id"`
	TS      int64             `json:"ts"`
	Kind    string            `json:"kind"`
	Actor   string            `json:"actor"`
	Subject string            `json:"subject"`
	Detail  map[string]string `json:"detail"`
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

// wealthBands are checked in order, so each below is its band's ceiling.
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

// named reports whether a plot carries its holder's nick. Opting out is the
// only thing that takes it off: a player who has never been asked is named.
func named(u *store.User) bool {
	return u.Web != store.VisibilityHidden
}

// claimable reports whether the site can tie this plot to whoever signs in.
// That needs the services account, which is the identity a nick change cannot
// move; the nick over the gate is a display name and needs nothing.
func claimable(u *store.User) bool {
	return named(u) && u.Account != ""
}

// publicPlot names the plot. id is what it is keyed on when the bot has no
// account for the holder: their nick is theirs to display, but a session is
// never resolved against it.
func (g *Plugin) publicPlot(u *store.User, id string) Plot {
	if u.Account != "" {
		id = u.Account
	}
	return Plot{
		ID:      id,
		Nick:    u.Username,
		Named:   true,
		Flag:    u.Flag,
		Acres:   u.Items["acre"],
		Land:    g.land(u),
		Wealth:  wealth(u.CumOhayous),
		Rations: u.TimesOhayoued,
	}
}

// anonymousPlot is how much land somebody holds and roughly what they have
// earned, with nothing that says who they are.
func (g *Plugin) anonymousPlot(u *store.User, id string) Plot {
	return Plot{
		ID:      id,
		Named:   false,
		Acres:   u.Items["acre"],
		Land:    map[string]int{},
		Wealth:  wealth(u.CumOhayous),
		Rations: u.TimesOhayoued,
	}
}

// land is what takes up room. The catalog decides both halves: an acre limit
// means it occupies land, and any defence keeps it off the public map.
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
// thousand players costs one query rather than a thousand.
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
		// Level+1 is what !stats shows; the two must not disagree.
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

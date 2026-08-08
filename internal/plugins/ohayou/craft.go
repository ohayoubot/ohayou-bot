package ohayou

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// recipe describes how to craft one buildable thing from mined resources.
// metals are consumed from the user's mined metals, items from their inventory
// (oil barrels and intermediate parts), plus an optional ohayou cost.
type recipe struct {
	name    string
	amount  int
	ohayous int
	metals  map[string]int
	items   map[string]int
	desc    string
}

// recipeList is the crafting tech tree, ordered from raw intermediates up to
// advanced ohayou-generating buildings. Buildings (those with a passive bonus)
// must also exist in the item catalog with a matching "add" value.
var recipeList = []recipe{
	// Intermediate parts (no passive income; used to build things).
	{name: "gear", amount: 1, metals: map[string]int{"iron": 2}, desc: "a mechanical gear"},
	{name: "circuit", amount: 1, metals: map[string]int{"copper": 3}, desc: "an electronic circuit"},
	{name: "plastic", amount: 1, items: map[string]int{"oilbarrel": 2}, desc: "a bar of plastic"},
	{name: "steelplate", amount: 1, metals: map[string]int{"iron": 5}, desc: "a steel plate"},

	// Buildings (generate passive ohayous every ration; land-limited).
	{name: "workshop", amount: 1, ohayous: 500,
		items: map[string]int{"gear": 10},
		desc:  "a workshop (+3 ohayous per ration, 5 per acre)"},
	{name: "factory", amount: 1, ohayous: 2000,
		items: map[string]int{"gear": 20, "circuit": 10, "steelplate": 5},
		desc:  "a factory (+10 ohayous per ration, 2 per acre)"},
	{name: "refinery", amount: 1, ohayous: 5000,
		metals: map[string]int{"uranium": 2},
		items:  map[string]int{"steelplate": 5, "circuit": 10, "plastic": 5},
		desc:   "a refinery (+30 ohayous per ration, 1 per acre)"},
}

var recipes = func() map[string]recipe {
	m := make(map[string]recipe, len(recipeList))
	for _, r := range recipeList {
		m[r.name] = r
	}
	return m
}()

func (g *Plugin) cmdBuild(m *bot.Message) {
	to := m.ReplyTo()

	if !m.HasArgs() {
		names := make([]string, len(recipeList))
		for i, r := range recipeList {
			names[i] = r.name
		}
		g.say(to, "Craft parts and buildings from mined metals and oil. Buildings "+
			"generate ohayous every ration. Craftable: "+strings.Join(names, ", ")+
			". Type "+g.p()+"build <thing> to build it, or "+g.p()+"recipe <thing> "+
			"to see what it needs.")
		return
	}

	rec, ok := recipes[strings.ToLower(m.Arg(1))]
	if !ok {
		g.say(to, "You can't build that. Type "+g.p()+"build to see what's craftable.")
		return
	}

	user, ok := g.requireUser(m, to)
	if !ok {
		return
	}
	if msg, blocked := g.mustIdentify(user); blocked {
		g.say(to, msg)
		return
	}

	if missing := missingResources(user, rec); missing != "" {
		g.say(to, "You don't have enough to build "+rec.name+". You still need: "+missing+".")
		return
	}
	if user.Ohayous < rec.ohayous {
		g.say(to, fmt.Sprintf("Building %s costs %d ohayous and you can't afford it.",
			rec.name, rec.ohayous))
		return
	}

	// Land check for buildings that occupy acres.
	if item, err := g.store.GetItem(g.ctx(), rec.name); err == nil && item.Acrelimit > 0 {
		if user.Items[rec.name]+rec.amount > item.Acrelimit*user.Items["acre"] {
			g.say(to, fmt.Sprintf("%s: you need more land to place that. You can only "+
				"have %d %ss per acre.", user.Username, item.Acrelimit, rec.name))
			return
		}
	}

	err := g.store.Build(g.ctx(), user.Username, rec.metals, rec.items, rec.ohayous, rec.name, rec.amount)
	if errors.Is(err, store.ErrInsufficient) {
		// The snapshot had the resources but the atomic build didn't (a
		// concurrent build consumed them first).
		g.say(to, "You don't have enough to build "+rec.name+" right now. Try again.")
		return
	}
	if err != nil {
		g.log.Error("build", "nick", user.Username, "thing", rec.name, "err", err)
		g.say(to, "Something went wrong building that. Try again.")
		return
	}
	// Only what takes up land, the same rule the map draws by: a gear is not a
	// thing that happened to the countryside.
	if item, err := g.store.GetItem(g.ctx(), rec.name); err == nil && item.Acrelimit > 0 {
		g.record(eventBuild, user.Username, "", map[string]string{"thing": rec.name})
	}

	g.say(to, fmt.Sprintf("%s built %d %s! (%s)", user.Username, rec.amount, rec.name, rec.desc))
}

func (g *Plugin) cmdRecipe(m *bot.Message) {
	to := m.ReplyTo()
	if !m.HasArgs() {
		g.say(to, "Usage: "+g.p()+"recipe <thing> -- shows what a craftable needs.")
		return
	}
	rec, ok := recipes[strings.ToLower(m.Arg(1))]
	if !ok {
		g.say(to, "That isn't craftable. Type "+g.p()+"build to see what is.")
		return
	}
	g.say(to, fmt.Sprintf("%s: %s. Requires %s.", rec.name, rec.desc, recipeCost(rec)))
}

// missingResources returns a human-readable list of the shortfalls preventing a
// build, or "" if the user has everything (ohayous aside).
func missingResources(u *store.User, rec recipe) string {
	var short []string
	for metal, need := range rec.metals {
		if have := u.Quarry.Metals[metal]; have < need {
			short = append(short, fmt.Sprintf("%d %s", need-have, metal))
		}
	}
	for item, need := range rec.items {
		if have := u.Items[item]; have < need {
			short = append(short, fmt.Sprintf("%d %s", need-have, item))
		}
	}
	sort.Strings(short)
	return strings.Join(short, ", ")
}

// recipeCost renders a full recipe's inputs for display.
func recipeCost(rec recipe) string {
	var parts []string
	if rec.ohayous > 0 {
		parts = append(parts, fmt.Sprintf("%d ohayous", rec.ohayous))
	}
	for metal, n := range rec.metals {
		parts = append(parts, fmt.Sprintf("%d %s", n, metal))
	}
	for item, n := range rec.items {
		parts = append(parts, fmt.Sprintf("%d %s", n, item))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

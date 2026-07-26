package game

import (
	"strings"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
)

type helpTopic struct {
	name    string
	summary string
	lines   []string
}

func helpTopics(p string) []helpTopic {
	return []helpTopic{
		{"basics", "the daily ration and how ohayous work", []string{
			"Ohayous are the currency. Say " + p + "ohayou once a day to collect your ration. Skip a day and any streak resets; random events can double it or drop a stray cat.",
			"Spend ohayous in the shop, sink them into industry, or gamble them on " + p + "steal. Check where you stand with " + p + "stats and " + p + "inventory.",
		}},
		{"shop", "buying, using and wearing items", []string{
			"Browse with " + p + "items <category> and read one up close with " + p + "item <name>. Buy with " + p + "buy <item> [n], or " + p + "buy <item> max to spend what you can.",
			"Some items you " + p + "use (like a fortunecookie or a vault); armor you " + p + "equip and " + p + "unequip. " + p + "inventory lists what you own.",
		}},
		{"industry", "acres, mining, oil and building factories", []string{
			"Buy " + p + "acre plots for room, then a quarry (metals) and an oilwell (oil), each on its own acre. Run them with " + p + "use quarry and " + p + "use oilwell for a few hours.",
			"Craft metals and oil into parts and buildings with " + p + "build (check " + p + "recipe <thing>): gear/circuit/steelplate/plastic, then workshop +3, factory +10, refinery +30 ohayous per ration.",
		}},
		{"animals", "cats, dogs, breeding and cat events", []string{
			"Cats add 1 to every ration (max 20 per acre) and can be stolen. catnip doubles what all your cats earn. They live in the animals category.",
			"Breed them with a cattery on its own acre: " + p + "use cattery. When a stray wanders in, lure it with " + p + "use burger or " + p + "use pancake.",
			"A dog adds 3 to every ration (one per acre) and guards the place, making you harder to rob. " + p + "use dog walks it once a day and it may dig up metals. Beware that a dog around breeding cats sometimes kills one.",
		}},
		{"stealing", "robbing others and defending yourself", []string{
			"Try " + p + "steal <user> for a shot at their on-hand ohayous or a cat. Get caught and you pay a fine and sit on probation, earning nothing until it clears.",
			"Wear armor (helmet, gloves, vest) with " + p + "equip to defend, and keep a dog around. Thieves only reach loose ohayous, so bank the rest: " + p + "use vault, then " + p + "deposit and " + p + "withdraw (once a day).",
			"Robbed? The Ohayou Police will PM you; type " + p + "report to have them guard you for a few hours and cut the odds of it happening again.",
		}},
		{"account", "protecting your stash with registration", []string{
			"Registering ties your assets to your NickServ account so nobody else on your nick can touch them. Type " + p + "register for the details, then " + p + "register yes.",
			"Once registered you must " + p + "identify after you connect or change nick before most commands will work.",
		}},
		{"status", "checking your standing", []string{
			p + "stats gives a full breakdown in PM, " + p + "inventory lists your items and ohayous, " + p + "quarry shows mined metals, and " + p + "top ranks the richest players.",
		}},
	}
}

var helpAlias = map[string]string{
	"ohayou":    "basics",
	"buy":       "shop",
	"items":     "shop",
	"item":      "shop",
	"use":       "shop",
	"equip":     "shop",
	"unequip":   "shop",
	"acre":      "industry",
	"quarry":    "industry",
	"oilwell":   "industry",
	"build":     "industry",
	"recipe":    "industry",
	"cat":       "animals",
	"cats":      "animals",
	"cattery":   "animals",
	"catnip":    "animals",
	"breed":     "animals",
	"dog":       "animals",
	"dogs":      "animals",
	"steal":     "stealing",
	"vault":     "stealing",
	"armor":     "stealing",
	"deposit":   "stealing",
	"withdraw":  "stealing",
	"register":  "account",
	"identify":  "account",
	"stats":     "status",
	"inventory": "status",
	"top":       "status",
}

// TODO: make this automatic?
var commandList = []string{
	"ohayou", "buy", "items", "item", "use", "equip", "unequip", "build",
	"recipe", "quarry", "inventory", "stats", "top", "steal", "report",
	"deposit", "withdraw", "register", "identify",
}

func (g *Game) cmdHelp(m *bot.Message) {
	to := replyTarget(m)
	p := g.p()
	topics := helpTopics(p)

	if m.HasArgs() {
		want := strings.ToLower(m.Args[1])
		if alias, ok := helpAlias[want]; ok {
			want = alias
		}
		for _, t := range topics {
			if t.name == want {
				for _, line := range t.lines {
					g.say(to, line)
				}
				return
			}
		}
		g.say(to, "No help for \""+m.Args[1]+"\". Type "+p+"help to see the topics.")
		return
	}

	g.say(to, "Ohayou is a daily economy game: say "+p+"ohayou each day to earn ohayous, "+
		"then spend them on land, animals, industry, and thievery.")
	g.say(to, "Read up on a topic with "+p+"help <topic>. Topics: "+strings.Join(topicIndex(topics), ", ")+".")
	g.say(to, "Every command: "+g.prefixed(commandList)+".")
}

// topicIndex returns just the topic names in order
func topicIndex(topics []helpTopic) []string {
	names := make([]string, len(topics))
	for i, t := range topics {
		names[i] = t.name
	}
	return names
}

// prefixed joins names into a comma list with the command prefix on each.
func (g *Game) prefixed(names []string) string {
	p := g.p()
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = p + n
	}
	return strings.Join(out, ", ")
}

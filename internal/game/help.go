package game

import "github.com/ohayoubot/ohayou-bot/internal/bot"

// helpTopics is what the game teaches the bot's !help. Aliases are the commands
// and nouns each topic covers, so !help steal reaches the topic about it.
func helpTopics(p string) []bot.Topic {
	return []bot.Topic{
		{Name: "basics", Summary: "the daily ration and how ohayous work",
			Aliases: []string{"ohayou", "game"},
			Lines: []string{
				"Ohayous are the currency. Say " + p + "ohayou once a day to collect your ration. Skip a day and any streak resets; random events can double it or drop a stray cat.",
				"Spend ohayous in the shop, sink them into industry, or gamble them on " + p + "steal. Check where you stand with " + p + "stats and " + p + "inventory.",
			}},
		{Name: "shop", Summary: "buying, using and wearing items",
			Aliases: []string{"buy", "items", "item", "use", "equip", "unequip"},
			Lines: []string{
				"Browse with " + p + "items <category> and read one up close with " + p + "item <name>. Buy with " + p + "buy <item> [n], or " + p + "buy <item> max to spend what you can.",
				"Some items you " + p + "use (like a fortunecookie or a vault); armor you " + p + "equip and " + p + "unequip. " + p + "inventory lists what you own.",
			}},
		{Name: "industry", Summary: "acres, mining, oil and building factories",
			Aliases: []string{"acre", "quarry", "oilwell", "build", "recipe"},
			Lines: []string{
				"Buy " + p + "acre plots for room, then a quarry (metals) and an oilwell (oil), each on its own acre. Run them with " + p + "use quarry and " + p + "use oilwell for a few hours.",
				"Craft metals and oil into parts and buildings with " + p + "build (check " + p + "recipe <thing>): gear/circuit/steelplate/plastic, then workshop +3, factory +10, refinery +30 ohayous per ration.",
			}},
		{Name: "animals", Summary: "cats, dogs, breeding and cat events",
			Aliases: []string{"cat", "cats", "cattery", "catnip", "breed", "dog", "dogs"},
			Lines: []string{
				"Cats add 1 to every ration (max 20 per acre) and can be stolen. catnip doubles what all your cats earn. They live in the animals category.",
				"Breed them with a cattery on its own acre: " + p + "use cattery. When a stray wanders in, lure it with " + p + "use burger or " + p + "use pancake.",
				"A dog adds 3 to every ration (one per acre) and guards the place, making you harder to rob. " + p + "use dog walks it once a day and it may dig up metals. Beware that a dog around breeding cats sometimes kills one.",
			}},
		{Name: "stealing", Summary: "robbing others and defending yourself",
			Aliases: []string{"steal", "vault", "armor", "deposit", "withdraw", "report"},
			Lines: []string{
				"Try " + p + "steal <user> for a shot at their on-hand ohayous or a cat. Get caught and you pay a fine and sit on probation, earning nothing until it clears.",
				"Wear armor (helmet, gloves, vest) with " + p + "equip to defend, and keep a dog around. Thieves only reach loose ohayous, so bank the rest: " + p + "use vault, then " + p + "deposit and " + p + "withdraw (once a day).",
				"Robbed? The Ohayou Police will PM you; type " + p + "report to have them guard you for a few hours and cut the odds of it happening again.",
			}},
		{Name: "account", Summary: "protecting your stash with registration",
			Aliases: []string{"register", "identify"},
			Lines: []string{
				"Registering ties your assets to your NickServ account so nobody else on your nick can touch them. Type " + p + "register for the details, then " + p + "register yes.",
				"Once registered you must " + p + "identify after you connect or change nick before most commands will work.",
			}},
		{Name: "status", Summary: "checking your standing",
			Aliases: []string{"stats", "inventory", "top"},
			Lines: []string{
				p + "stats gives a full breakdown in PM, " + p + "inventory lists your items and ohayous, " + p + "quarry shows mined metals, and " + p + "top ranks the richest players.",
			}},
	}
}

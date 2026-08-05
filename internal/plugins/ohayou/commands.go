package ohayou

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// requireUser loads the command sender, sending the standard "you haven't
// ohayou'd" message to failTo when they have no account.
func (g *Plugin) requireUser(m *bot.Message, failTo string) (*store.User, bool) {
	user, err := g.store.GetUser(g.ctx(), strings.ToLower(m.Nick))
	if errors.Is(err, store.ErrNotFound) {
		g.say(failTo, "You can't do that because you haven't ohayou'd yet! Type "+
			g.p()+"ohayou to get your first ration.")
		return nil, false
	}
	if err != nil {
		g.log.Error("get user", "nick", m.Nick, "err", err)
		g.say(failTo, "Something went wrong. Try again.")
		return nil, false
	}
	return user, true
}

func (g *Plugin) cmdOhayou(m *bot.Message) {
	if !m.FromChannel() {
		g.say(m.Nick, "You can only do that in a channel I'm in.")
		return
	}
	g.say(m.Target, g.Ohayou(strings.ToLower(m.Nick)))
}

func (g *Plugin) cmdBuy(m *bot.Message) {
	if !m.FromChannel() {
		g.say(m.Nick, "You must buy things in a channel I'm in.")
		return
	}
	if !m.HasArgs() {
		g.say(m.Target, "Usage: "+g.p()+"buy <item> will buy you one <item>. "+g.p()+
			"buy <item> 3 will buy you 3 of <item>, if you can afford it.")
		return
	}
	user, ok := g.requireUser(m, m.Target)
	if !ok {
		return
	}

	itm := strings.ToLower(m.Arg(1))
	if itm == "ohayou" { // just for fun
		g.say(m.Target, fmt.Sprintf("You purchased %d ohayous for %d ohayous. You "+
			"have %d ohayous left.", user.Ohayous, user.Ohayous, user.Ohayous))
		return
	}

	if len(m.Args) <= 2 {
		g.say(m.Target, g.buy(user, itm, 1))
		return
	}

	qty := strings.ToLower(m.Arg(2))
	if qty == "max" {
		g.say(m.Target, g.buy(user, itm, g.findMax(user, itm)))
		return
	}
	amt, err := strconv.Atoi(qty)
	if err != nil {
		g.say(m.Target, "You didn't give a valid quantity. Usage: "+g.p()+"buy "+
			"<item> will buy you one <item>. "+g.p()+"buy <item> 3 will buy you 3 "+
			"of <item>, if you can afford it.")
		return
	}
	g.say(m.Target, g.buy(user, itm, amt))
}

func (g *Plugin) cmdUse(m *bot.Message) {
	if !m.FromChannel() {
		g.say(m.Nick, "You can only do that in a channel I'm in.")
		return
	}
	if !m.HasArgs() {
		g.say(m.Target, "Type "+g.p()+"use <item> to use an item. Type "+g.p()+
			"inventory to see what items you have, or "+g.p()+"items to see what "+
			"items you can "+g.p()+"buy.")
		return
	}
	user, ok := g.requireUser(m, m.Target)
	if !ok {
		return
	}

	itm := strings.ToLower(m.Arg(1))
	on := "somebody"
	if len(m.Args) > 2 {
		on = strings.ToLower(m.Arg(2))
	}
	g.say(m.Target, g.use(user, m.Nick, itm, on))
}

func (g *Plugin) cmdSteal(m *bot.Message) {
	if !m.FromChannel() {
		g.say(m.Nick, "You can only do that in a channel I'm in.")
		return
	}
	if !m.HasArgs() {
		g.say(m.Target, "Attempts to steal from someone. Usage: "+g.p()+"steal "+
			"<user>. Has penalties if you are caught!")
		return
	}
	user, ok := g.requireUser(m, m.Target)
	if !ok {
		return
	}

	target := strings.ToLower(m.Arg(1))
	if user.Username == target {
		g.say(m.Target, "Are you dumb? You can't steal from yourself!")
		return
	}

	victim, err := g.store.GetUser(g.ctx(), target)
	if errors.Is(err, store.ErrNotFound) {
		g.say(m.Target, "You can't steal from "+m.Arg(1)+" because "+m.Arg(1)+
			" has never ohayou'd!")
		return
	}
	if err != nil {
		g.log.Error("get victim", "nick", target, "err", err)
		return
	}
	g.bot.Go(func() { g.stealFrom(user, victim, m.Target, m.Nick, m.Arg(1)) })
}

func (g *Plugin) cmdEquip(m *bot.Message) {
	to := m.ReplyTo()
	user, ok := g.requireUser(m, to)
	if !ok {
		return
	}
	if !m.HasArgs() {
		g.say(to, "Type "+g.p()+"equip <item> to equip <item>. You can only have one "+
			"item equipped per slot, unless otherwise noted.")
		return
	}
	g.say(to, g.equip(user, strings.ToLower(m.Arg(1))))
}

func (g *Plugin) cmdUnequip(m *bot.Message) {
	to := m.ReplyTo()
	user, ok := g.requireUser(m, to)
	if !ok {
		return
	}
	if !m.HasArgs() {
		g.say(to, "Unequips an equipped item. Usage: "+g.p()+"unequip <item> -- "+
			"unequips <item>")
		return
	}
	g.say(to, g.unequip(user, strings.ToLower(m.Arg(1))))
}

func (g *Plugin) cmdItems(m *bot.Message) {
	to := m.ReplyTo()
	if !m.HasArgs() {
		cats, err := g.store.Categories(g.ctx())
		if err != nil {
			g.log.Error("categories", "err", err)
		}
		g.say(to, "Type "+g.p()+"items <category> to get a list of items by category. "+
			"Categories: "+strings.Join(cats, ", ")+".")
		return
	}

	items, err := g.store.ItemsByCategory(g.ctx(), strings.ToLower(m.Arg(1)))
	if err != nil {
		g.log.Error("items by category", "err", err)
		return
	}
	for _, it := range items {
		g.say(m.Nick, formatItemLine(it))
	}
}

func (g *Plugin) cmdItem(m *bot.Message) {
	to := m.ReplyTo()
	if !m.HasArgs() {
		g.say(to, "Gives information about a specific item. Usage: "+g.p()+
			"item <itemname>")
		return
	}

	item, err := g.store.GetItem(g.ctx(), strings.ToLower(m.Arg(1)))
	if err != nil {
		g.say(to, "I don't carry that item.")
		return
	}
	if !item.Purchase {
		g.say(to, fmt.Sprintf("%s: %s. Cannot be purchased.", item.Name, item.Desc))
		return
	}

	info := fmt.Sprintf("%s: %s - Price: %d ohayous.", item.Name, item.Desc, item.Price)
	if item.Consume {
		info += " Consumed when used."
	}
	if item.Defense > 0 {
		info += fmt.Sprintf(" Adds %d defense.", item.Defense)
	}
	if item.Limit > 0 {
		info += fmt.Sprintf(" Limited to %d.", item.Limit)
	}
	if item.Acrelimit > 0 {
		info += fmt.Sprintf(" Limited to %d per acre.", item.Acrelimit)
	}
	g.say(to, info)
}

func (g *Plugin) cmdDeposit(m *bot.Message) {
	to := m.ReplyTo()
	if !m.HasArgs() {
		g.say(to, "Deposits ohayous to your vault. Usage: "+g.p()+"deposit <num> -- "+
			"deposits <num> ohayous. Your vault can only be opened once per day due "+
			"to its security protocol.")
		return
	}
	user, ok := g.requireUser(m, to)
	if !ok {
		return
	}
	amt, err := strconv.Atoi(m.Arg(1))
	if err != nil {
		g.say(to, "You didn't give a valid quantity. Usage: "+g.p()+"deposit <num> "+
			"will deposit <num> ohayous to your vault.")
		return
	}
	g.say(to, g.deposit(user, amt))
}

func (g *Plugin) cmdWithdraw(m *bot.Message) {
	to := m.ReplyTo()
	if !m.HasArgs() {
		g.say(to, "Withdraws ohayous from your vault. Usage: "+g.p()+"withdraw <num> "+
			"-- withdraws <num> ohayous. Your vault can only be opened once per day "+
			"due to its security protocol.")
		return
	}
	user, ok := g.requireUser(m, to)
	if !ok {
		return
	}
	amt, err := strconv.Atoi(m.Arg(1))
	if err != nil {
		g.say(to, "You didn't give a valid quantity. Usage: "+g.p()+"withdraw <num> "+
			"will withdraw <num> ohayous from your vault.")
		return
	}
	g.say(to, g.withdraw(user, amt))
}

func (g *Plugin) cmdStats(m *bot.Message) {
	to := m.ReplyTo()
	user, ok := g.requireUser(m, to)
	if !ok {
		return
	}
	g.bot.Go(func() { g.stats(user) })
}

func (g *Plugin) cmdInventory(m *bot.Message) {
	user, ok := g.requireUser(m, m.Nick)
	if !ok {
		return
	}
	if msg, blocked := g.mustIdentify(user); blocked {
		g.say(user.Username, msg)
		return
	}
	if len(user.Items) == 0 {
		g.say(m.Nick, "You don't have any items yet. Keep saving!")
		return
	}

	inv := fmt.Sprintf("You have: %d ohayous, ", user.Ohayous)
	if user.Vault.Installed {
		inv += fmt.Sprintf("a Level %d vault (%d/%d ohayous), ",
			user.Vault.Level+1, user.Vault.Ohayous, vaultCap(user.Vault.Level))
	}
	for _, amt := range amountsDesc(user.Items) {
		for _, itm := range namesWithAmount(user.Items, amt) {
			switch {
			case amt == 0, itm == "vault":
				continue
			case amt > 1:
				inv += fmt.Sprintf("%d %ss, ", amt, itm)
			default:
				inv += fmt.Sprintf("%d %s, ", amt, itm)
			}
		}
	}
	g.say(m.Nick, strings.TrimSuffix(inv, ", "))
}

func (g *Plugin) cmdQuarry(m *bot.Message) {
	user, ok := g.requireUser(m, m.Nick)
	if !ok {
		return
	}
	if msg, blocked := g.mustIdentify(user); blocked {
		g.say(user.Username, msg)
		return
	}
	if user.Items["quarry"] == 0 && len(user.Quarry.Metals) == 0 {
		g.say(m.Nick, "You don't have any quarries yet. Keep saving!")
		return
	}

	inv := "You have no quarries, but you have these metals: "
	if user.Items["quarry"] > 0 {
		inv = fmt.Sprintf("You have %d quarries and have mined these metals: ",
			user.Items["quarry"])
	}
	for _, amt := range amountsDesc(user.Quarry.Metals) {
		if amt == 0 {
			continue
		}
		for _, metal := range namesWithAmount(user.Quarry.Metals, amt) {
			inv += fmt.Sprintf("%d %s, ", amt, metal)
		}
	}
	g.say(m.Nick, strings.TrimSuffix(inv, ", "))
}

// cmdTerritory reads and sets whether a user's holdings may leave irc. What the
// public tier promises here is what web.go's Plot is allowed to carry. The name
// is not !web because that is the bot's, and signs you in to the whole site.
func (g *Plugin) cmdTerritory(m *bot.Message) {
	to := m.ReplyTo()
	user, ok := g.requireUser(m, to)
	if !ok {
		return
	}
	if msg, blocked := g.mustIdentify(user); blocked {
		g.say(to, msg)
		return
	}

	want := user.Web
	switch strings.ToLower(m.Arg(1)) {
	case "":
		g.say(to, m.Nick+": "+webState(user.Web))
		g.say(to, "Public means your nick, your land and what you have built on it, "+
			"and roughly how much you have earned. Never your ohayous on hand, your "+
			"vault, or your defences. "+g.p()+"territory on to appear, "+g.p()+
			"territory off to stay out.")
		return
	case "on", "yes", "public":
		want = store.VisibilityPublic
	case "off", "no", "hidden":
		want = store.VisibilityHidden
	default:
		g.say(to, "Usage: "+g.p()+"territory on, "+g.p()+"territory off, or "+g.p()+"territory to see where you stand.")
		return
	}

	if want == user.Web {
		g.say(to, m.Nick+": "+webState(user.Web))
		return
	}
	if err := g.store.SetVisibility(g.ctx(), user.Username, want); err != nil {
		g.log.Error("set visibility", "nick", user.Username, "err", err)
		g.say(to, "Something went wrong saving that. Try again.")
		return
	}
	g.say(to, m.Nick+": "+webState(want))
}

func webState(v store.Visibility) string {
	switch v {
	case store.VisibilityPublic:
		return "Your territory appears on the website."
	case store.VisibilityHidden:
		return "Your territory stays off the website."
	default:
		return "You have not said either way, so your territory stays off the website."
	}
}

func (g *Plugin) cmdRegister(m *bot.Message) {
	if !m.HasArgs() {
		g.say(m.Nick, "Registering allows you to protect your ohayou assets. After "+
			"you are registered, you will be required to identify with the bot prior "+
			"to using most of its commands. Changing your nickname will also require "+
			"you to again identify.")
		g.say(m.Nick, "Type '"+g.p()+"register yes' to register your nickname. Your "+
			"nickname must be registered with the network for this to work, and you "+
			"must be identified.")
		g.say(m.Nick, "To identify with the bot whenever you log on, you must type "+
			g.p()+"identify")
		return
	}
	to := m.ReplyTo()
	user, ok := g.requireUser(m, to)
	if !ok {
		return
	}
	if strings.ToLower(m.Arg(1)) == "yes" {
		if user.Registered {
			g.say(to, m.Nick+": You are already registered.")
			return
		}
		g.register(user)
	}
}

func (g *Plugin) cmdIdentify(m *bot.Message) {
	to := m.ReplyTo()
	user, ok := g.requireUser(m, to)
	if !ok {
		return
	}
	if !user.Registered {
		g.say(to, "You can't do that because you're not registered yet! Type "+g.p()+
			"register to be PM'd information about registering.")
		return
	}
	if g.bot.Identified(user.Username) {
		g.say(to, user.Username+": You are already identified.")
		return
	}
	g.identify(user, to)
}

func (g *Plugin) cmdTop(m *bot.Message) {
	top, err := g.store.Top(g.ctx(), 5)
	if err != nil {
		g.log.Error("top", "err", err)
		return
	}
	if len(top) == 0 {
		g.say(m.Target, "Nobody has ohayou'd yet!")
		return
	}
	parts := make([]string, len(top))
	for i, u := range top {
		parts[i] = fmt.Sprintf("%s (%d)", u.Username, u.Ohayous)
	}
	g.say(m.Target, "Top ohayou holders: "+strings.Join(parts, ", "))
}

func formatItemLine(item store.Item) string {
	if item.Acrelimit > 0 {
		return fmt.Sprintf("%s: %s - Price: %d ohayous. Limited to %d per acre.",
			item.Name, item.Desc, item.Price, item.Acrelimit)
	}
	return fmt.Sprintf("%s - %d ohayous - %s", item.Name, item.Price, item.Desc)
}

// amountsDesc returns the distinct values in counts, sorted high to low.
func amountsDesc(counts map[string]int) []int {
	seen := map[int]bool{}
	var amounts []int
	for _, v := range counts {
		if !seen[v] {
			seen[v] = true
			amounts = append(amounts, v)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(amounts)))
	return amounts
}

// namesWithAmount returns the keys in counts whose value equals amt.
func namesWithAmount(counts map[string]int, amt int) []string {
	var names []string
	for k, v := range counts {
		if v == amt {
			names = append(names, k)
		}
	}
	return names
}

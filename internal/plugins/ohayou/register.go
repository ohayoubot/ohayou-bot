package ohayou

import (
	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// register the user with the game
func (g *Plugin) register(u *store.User) {
	account, err := g.bot.Account(g.ctx(), u.Username)
	if err != nil {
		g.log.Error("register whois", "nick", u.Username, "err", err)
		g.say(u.Username, "I couldn't check with the network just now. Try again in a moment.")
		return
	}
	if account == "" {
		g.log.Info("register denied: not identified with network", "nick", u.Username)
		g.say(u.Username, "Your nick isn't registered or you aren't identified "+
			"with NickServ. You must do both before you can register")
		return
	}
	g.log.Info("register success", "nick", u.Username)
	g.say(u.Username, "Successfully registered! Type "+g.p()+"identify to "+
		"identify yourself with the bot.")
	if err := g.store.SetRegister(g.ctx(), u.Username, true); err != nil {
		g.log.Error("set register", "nick", u.Username, "err", err)
	}
	g.rememberAccount(u.Username, account)
}

// rememberAccount records the account a nick just proved. Not worth failing a
// register or an identify over: nothing reads it yet but the website.
func (g *Plugin) rememberAccount(nick, account string) {
	if account == "" {
		return
	}
	if err := g.store.SetAccount(g.ctx(), nick, account); err != nil {
		g.log.Error("record account", "nick", nick, "err", err)
	}
}

// identify verifies a registered user is identified with the network and, if
// so, marks them identified with the bot until they change nick or quit.
func (g *Plugin) identify(u *store.User, to string) {
	account, err := g.bot.Identify(g.ctx(), u.Username)
	if err != nil {
		g.log.Error("identify whois", "nick", u.Username, "err", err)
		g.say(to, u.Username+": I couldn't check with the network just now. Try again in a moment.")
		return
	}
	if account == "" {
		g.log.Info("identify denied: not identified with network", "nick", u.Username)
		g.say(to, u.Username+": You must be identified with the network to "+
			"identify with me.")
		return
	}
	g.log.Info("identify success", "nick", u.Username)
	g.say(to, u.Username+": You are now identified with the bot. Changing nicks "+
		"or logging off will remove this.")
	g.rememberAccount(u.Username, account)
}

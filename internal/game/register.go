package game

import (
	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// register the user with the game
func (g *Game) register(u *store.User) {
	registered, err := g.bot.Verify(g.ctx(), u.Username)
	if err != nil {
		g.log.Error("register whois", "nick", u.Username, "err", err)
		g.say(u.Username, "I couldn't check with the network just now. Try again in a moment.")
		return
	}
	if !registered {
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
}

// identify verifies a registered user is identified with the network and, if
// so, marks them identified with the bot until they change nick or quit.
func (g *Game) identify(u *store.User, to string) {
	registered, err := g.bot.Identify(g.ctx(), u.Username)
	if err != nil {
		g.log.Error("identify whois", "nick", u.Username, "err", err)
		g.say(to, u.Username+": I couldn't check with the network just now. Try again in a moment.")
		return
	}
	if !registered {
		g.log.Info("identify denied: not identified with network", "nick", u.Username)
		g.say(to, u.Username+": You must be identified with the network to "+
			"identify with me.")
		return
	}
	g.log.Info("identify success", "nick", u.Username)
	g.say(to, u.Username+": You are now identified with the bot. Changing nicks "+
		"or logging off will remove this.")
}

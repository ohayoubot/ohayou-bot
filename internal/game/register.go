package game

import (
	"strings"

	irc "github.com/thoj/go-ircevent"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// whoisRegistered does a WHOIS and returns whether the nick is registered
func (g *Game) whoisRegistered(nick string, onDone func(registered bool)) {
	registered := new(bool)
	// registered
	g.bot.AddCallback("307", func(e *irc.Event) { *registered = true })
	// identifier (hence registered)
	g.bot.AddCallback("318", func(e *irc.Event) {
		g.bot.ClearCallback("307")
		g.bot.ClearCallback("318")
		onDone(*registered)
	})
	g.bot.Whois(nick)
}

// register the user with the game
func (g *Game) register(u *store.User) {
	g.whoisRegistered(u.Username, func(registered bool) {
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
	})
}

// identify verifies a registered user is identified with the network and, if
// so, marks them identified with the bot until they change nick or quit.
func (g *Game) identify(u *store.User, to string) {
	g.whoisRegistered(u.Username, func(registered bool) {
		if !registered {
			g.log.Info("identify denied: not identified with network", "nick", u.Username)
			g.say(to, u.Username+": You must be identified with the network to "+
				"identify with me.")
			return
		}
		g.log.Info("identify success", "nick", u.Username)
		g.say(to, u.Username+": You are now identified with the bot. Changing nicks "+
			"or logging off will remove this.")
		g.startWatchingNicks()
		g.setIdentified(u.Username, true)
	})
}

// startWatchingNicks sets NICK/QUIT handlers that will drop a user's
// identified status when they change nick or disconnect.
func (g *Game) startWatchingNicks() {
	g.mu.Lock()
	if g.watchingNicks {
		g.mu.Unlock()
		return
	}
	g.watchingNicks = true
	g.mu.Unlock()

	drop := func(e *irc.Event) {
		nick := strings.ToLower(e.Nick)
		if g.isIdentified(nick) {
			g.log.Debug("identity dropped", "nick", nick, "reason", e.Code)
		}
		g.setIdentified(nick, false)
	}
	g.bot.AddCallback("NICK", drop)
	g.bot.AddCallback("QUIT", drop)
}

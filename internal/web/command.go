package web

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/bot/ratelimit"
)

// whoisWait bounds the lookup. The bot's resolver has its own ceiling; this is
// so a wedged one cannot pin a command handler forever.
const whoisWait = 15 * time.Second

// linkTTL is how long a minted link stays good. Long enough to move from a
// terminal to a browser, short enough that one left in a scrollback is dead.
const linkTTL = 5 * time.Minute

// cooldown is the gap a nick leaves between links, which is also what stops a
// repeated command from being a WHOIS each time.
const cooldown = time.Minute

// Site hands out links to the website.
type Site struct {
	bot    *bot.Bot
	minter *Minter
	log    *slog.Logger
	url    string
	scopes Scope
	minted *ratelimit.Limiter
}

// Install claims !web, when there is a site to send anyone to and something for
// them to do there. Returns whether it did, so the caller can say why not.
func Install(b *bot.Bot, m *Minter, log *slog.Logger, url string, scopes Scope) bool {
	if m == nil || url == "" || scopes == 0 {
		return false
	}

	s := &Site{
		bot: b, minter: m, log: log, url: url, scopes: scopes,
		minted: ratelimit.New(cooldown),
	}
	b.HandleFunc("web", false, s.command)
	b.Help(bot.Topic{
		Name:    "web",
		Summary: "logging in to the website",
		Aliases: []string{"site", "login"},
		Lines: []string{
			"Type " + b.Prefix() + "web and I will PM you a link that signs you in, " +
				"good once and for " + linkTTL.String() + ".",
			"You must be identified with services, and we must share a channel. " +
				"One link covers everything on the site you can use.",
		},
	})
	return true
}

// Link is where a grant is redeemed. The token rides in the fragment, which
// never reaches the server's logs.
func (s *Site) Link(grant string) string {
	return strings.TrimSuffix(s.url, "#") + "#" + grant
}

func (s *Site) command(m *bot.Message) {
	to := m.ReplyTo()

	// The cooldown comes before the lookup, not after: without it every
	// repetition of the command is another WHOIS at the server.
	if wait, ok := s.minted.Claim(strings.ToLower(m.Nick)); !ok {
		s.bot.Say(to, m.Nick+": you just got a link. Try again in "+wait.Round(time.Second).String()+".")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), whoisWait)
	defer cancel()

	who, err := s.bot.WhoisInfo(ctx, m.Nick)
	if err != nil {
		s.log.Error("whois", "nick", m.Nick, "err", err)
		s.bot.Say(to, m.Nick+": I couldn't check with the network just now. Try again in a moment.")
		return
	}
	if !who.Identified() {
		s.log.Info("web refused", "reason", "not identified", "nick", m.Nick)
		s.bot.Say(to, m.Nick+": you need to be identified with services first.")
		return
	}

	channels := Channels(s.bot.SharedWith(who.Channels))
	if len(channels) == 0 {
		s.log.Info("web refused", "reason", "no shared channels",
			"nick", m.Nick, "account", who.Account)
		s.bot.Say(to, m.Nick+": we aren't in a channel together, so I don't know you well enough.")
		return
	}

	token, id, err := s.minter.Mint(Grant{
		Account:  who.Account,
		Nick:     m.Nick,
		Channels: channels,
		Scopes:   s.scopes,
		TTL:      linkTTL,
	})
	if err != nil {
		s.log.Error("mint", "nick", m.Nick, "err", err)
		s.bot.Say(to, m.Nick+": something went wrong making your link.")
		return
	}

	// Always privately, whatever the command arrived on. A grant said out loud
	// is a session for whoever reads the channel first.
	s.bot.Say(m.Nick, "Sign in here, good once and for "+linkTTL.String()+": "+s.Link(token))
	if m.FromChannel() {
		s.bot.Say(m.Target, m.Nick+": check your PMs.")
	}

	s.log.Info("web grant minted", "nick", m.Nick, "account", who.Account,
		"scopes", s.scopes, "grant", id)
}

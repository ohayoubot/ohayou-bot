// Package drop hands out signed links to the upload site and announces what
// comes back.
package drop

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// whoisWait bounds the lookup. The bot's resolver has its own ceiling; this is
// so a wedged one cannot pin a command handler forever.
const whoisWait = 15 * time.Second

// maxTracked caps the cooldown map so a channel full of strangers cannot grow
// it without bound.
const maxTracked = 1000

type Plugin struct {
	bot   *bot.Bot
	cfg   config.DropConfig
	log   *slog.Logger
	store store.Store

	now func() time.Time

	mu     sync.Mutex
	minted map[string]time.Time // lowercased nick -> when it last got a link
}

func New(b *bot.Bot, cfg config.DropConfig, st store.Store) *Plugin {
	return &Plugin{
		bot:    b,
		cfg:    cfg,
		log:    b.Logger().With("plugin", "drop"),
		store:  st,
		now:    time.Now,
		minted: map[string]time.Time{},
	}
}

func (p *Plugin) Register() {
	p.bot.HandleFunc("upload", false, p.cmdUpload)
}

func (p *Plugin) cmdUpload(m *bot.Message) {
	to := replyTo(m)

	// The cooldown comes before the lookup, not after: without it every
	// repetition of the command is another WHOIS at the server.
	if wait, ok := p.claim(m.Nick); !ok {
		p.bot.Say(to, m.Nick+": you just got a link. Try again in "+humanWait(wait)+".")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), whoisWait)
	defer cancel()

	who, err := p.bot.WhoisInfo(ctx, m.Nick)
	if err != nil {
		p.log.Error("whois", "nick", m.Nick, "err", err)
		p.bot.Say(to, m.Nick+": I couldn't check with the network just now. Try again in a moment.")
		return
	}

	if !who.Identified() {
		p.log.Info("upload refused", "reason", "not identified", "nick", m.Nick)
		p.bot.Say(to, m.Nick+": you need to be identified with services first.")
		return
	}

	channels := p.shared(who.Channels)
	if len(channels) == 0 {
		p.log.Info("upload refused", "reason", "no shared channels",
			"nick", m.Nick, "account", who.Account)
		p.bot.Say(to, m.Nick+": we aren't in a channel together, so there's nowhere to post.")
		return
	}

	jti, err := newJTI()
	if err != nil {
		p.log.Error("mint", "nick", m.Nick, "err", err)
		p.bot.Say(to, m.Nick+": something went wrong making your link.")
		return
	}

	token, err := mint(p.cfg.Secret, grant{
		A: who.Account,
		N: m.Nick,
		C: channels,
		E: expiry(p.now(), p.cfg.GrantWait()),
		J: jti,
	})
	if err != nil {
		p.log.Error("mint", "nick", m.Nick, "err", err)
		p.bot.Say(to, m.Nick+": something went wrong making your link.")
		return
	}

	// Always privately, whatever the command arrived on. A grant said out loud
	// is a session for whoever reads the channel first.
	p.bot.Say(m.Nick, "Upload here, good once and for "+humanWait(p.cfg.GrantWait())+": "+p.cfg.Link(token))
	if m.FromChannel() {
		p.bot.Say(m.Target, m.Nick+": check your PMs.")
	}

	p.log.Info("grant minted", "nick", m.Nick, "account", who.Account,
		"channels", strings.Join(channels, " "), "jti", jti)
}

// shared returns the channels both the asker and the bot are in, spelled the
// way the bot has them: the name travels into the grant and then into a queued
// line, so it should be the one the bot joined, not the one WHOIS echoed.
func (p *Plugin) shared(theirs []string) []string {
	mine := map[string]string{}
	for _, name := range p.bot.Channels() {
		mine[strings.ToLower(name)] = name
	}

	var out []string
	seen := map[string]bool{}
	for _, name := range theirs {
		key := strings.ToLower(name)
		canonical, ok := mine[key]
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, canonical)
		if len(out) == maxChannels {
			break
		}
	}
	return out
}

// claim takes this nick's turn, reporting how long is left when it cannot.
func (p *Plugin) claim(nick string) (time.Duration, bool) {
	key := strings.ToLower(nick)
	now := p.now()

	p.mu.Lock()
	defer p.mu.Unlock()

	if last, ok := p.minted[key]; ok {
		if wait := p.cfg.CooldownWait() - now.Sub(last); wait > 0 {
			return wait, false
		}
	}
	if len(p.minted) >= maxTracked {
		p.forgetOld(now)
	}
	p.minted[key] = now
	return 0, true
}

// forgetOld drops entries whose cooldown has run out. Called with mu held.
func (p *Plugin) forgetOld(now time.Time) {
	for key, last := range p.minted {
		if now.Sub(last) >= p.cfg.CooldownWait() {
			delete(p.minted, key)
		}
	}
}

func replyTo(m *bot.Message) string {
	if m.FromChannel() {
		return m.Target
	}
	return m.Nick
}

func humanWait(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	return d.Round(time.Minute).String()
}

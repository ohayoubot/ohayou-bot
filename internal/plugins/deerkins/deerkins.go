// Package deerkins paints the deerkins art gallery into irc.
package deerkins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/bot/access"
	"github.com/ohayoubot/ohayou-bot/internal/bot/irctext"
	"github.com/ohayoubot/ohayou-bot/internal/bot/ratelimit"
	"github.com/ohayoubot/ohayou-bot/internal/d1"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
)

const (
	defaultDeer = "deer"
	maxNameLen  = 48
	maxTextLen  = 64
	countTTL    = 5 * time.Minute
	// chatterWait spaces out the replies that aren't art, per asker.
	chatterWait = 15 * time.Second
)

type Plugin struct {
	bot *bot.Bot
	cfg Config
	log *slog.Logger
	db  *gallery

	banNicks    access.Set
	banHosts    access.Set
	banChannels access.Set

	// rule and hosts are the privileged list, projected to what access.Find
	// takes: a lowercased nick mapped to the host it must come from.
	rule  access.Rule
	hosts map[string]string

	roll func(int) int

	// paint serializes the multi-line output so two deer can't interleave.
	paint sync.Mutex

	used  *ratelimit.Limiter // when a target last got a deer
	spoke *ratelimit.Limiter // when a key last got a reply that isn't art

	mu      sync.Mutex
	last    map[string]sighting // the deer a target last saw
	latest  sighting            // the last deer anywhere
	count   int
	counted time.Time
}

// sighting is what !prevdeer reports.
type sighting struct {
	deer    string
	creator string
	nick    string
	mods    []string
	seen    bool
}

func New() *Plugin { return &Plugin{roll: rand.IntN, last: map[string]sighting{}} }

func (p *Plugin) Name() string { return "deerkins" }

func (p *Plugin) Register(deps plugin.Deps) error {
	p.bot, p.log = deps.Bot, deps.Log
	p.db = newGallery(d1.APIBase, p.cfg.AccountID, p.cfg.DatabaseID, p.cfg.APIToken, p.cfg.RequestTimeout())
	p.banNicks = access.NewSet(p.cfg.IgnoreNicks)
	p.banHosts = access.NewSet(p.cfg.IgnoreHosts)
	p.banChannels = access.NewSet(p.cfg.IgnoreChannels)
	p.rule = access.Rule{ByNick: p.cfg.MatchNick(), ByHost: p.cfg.MatchHost()}
	p.hosts = privilegedHosts(p.cfg.Privileged)
	p.used = ratelimit.New(p.cfg.Wait())
	p.spoke = ratelimit.New(chatterWait)

	p.bot.HandleFunc("deerme", false, p.cmdDeerme)
	p.bot.HandleFunc("prevdeer", false, p.cmdPrevDeer)

	prefix := p.bot.Prefix()
	p.bot.Help(bot.Topic{
		Name:    "deer",
		Summary: "painting the deerkins gallery into the channel",
		Aliases: []string{"deerme", "deerkins", "prevdeer"},
		Lines: []string{
			prefix + "deerme walks a deer, " + prefix + "deerme random or " +
				prefix + "deerme latest takes what you are given, and " +
				prefix + "prevdeer says what walked last.",
			"Stack modifiers before a pipe, like " + prefix +
				"deerme iu|senordeer. Type " + prefix +
				"deerme help modifiers for the list, or " + prefix +
				"deerme help for how long until the next one.",
			"Draw your own at " + p.cfg.Editor,
		},
	})
	p.log.Info("enabled", "database", p.cfg.DatabaseID, "editor", p.cfg.Editor)
	return nil
}

func (p *Plugin) cmdDeerme(m *bot.Message) {
	if field := p.bannedBy(m); field != "" {
		p.log.Info("deer refused", "reason", "banned", "match", field,
			"nick", m.Nick, "host", m.Host, "target", m.Target)
		return
	}

	user, privileged := p.privilegedFor(m)

	// Deer walk in channels. A privileged nick may summon one in private.
	if !m.FromChannel() && !privileged {
		p.log.Debug("ignoring deerme in private", "nick", m.Nick)
		return
	}

	to := m.ReplyTo()
	mods, name := splitRequest(m.Rest(1))
	name = sanitiseName(name)

	switch name {
	case "help":
		if p.maySpeak("help:" + m.Nick) {
			p.sayHelp(m, to)
		}
		return
	case "help modifiers":
		if p.maySpeak("help:" + m.Nick) {
			p.sayModifiers(m)
		}
		return
	case "":
		name = defaultDeer
	}

	if wait, timeout, ok := p.claim(to, m.Nick, name, mods, user, privileged); !ok {
		// Only the first refusal is answered. Repeating it would turn asking
		// too often into the flood the timeout exists to prevent.
		if p.maySpeak("wait:" + m.Nick) {
			p.bot.Say(m.Nick, p.waitMessage(wait, timeout, user, privileged))
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.RequestTimeout())
	defer cancel()

	art, err := p.fetch(ctx, name)
	if err != nil {
		// Release the slot on a short cooldown: a typo shouldn't cost the
		// channel a full timeout
		p.penalise(to)
		if errors.Is(err, errNoDeer) {
			p.bot.Say(to, "404: Deer Not Found. Go to "+p.cfg.Editor+" and create it.")
			return
		}
		p.log.Error("deer lookup", "deer", name, "err", err)
		p.bot.Say(to, "The deer are not answering right now.")
		return
	}

	rows, used := applyMods(normalise(art.Kinskode), mods, p.roll)
	lines := toIRC(clamp(rows, p.cfg.MaxLines), lineBudget(to))

	deer, creator := sanitiseText(art.Deer), sanitiseText(art.Creator)
	if creator == "" {
		creator = "n/a"
	}

	p.paint.Lock()
	for _, line := range lines {
		p.bot.Say(to, line)
	}
	// Say who drew it when the requester couldn't know, and whenever 'x' picked
	// the modifiers for them.
	if name == "random" || name == "latest" || isX(mods) {
		p.bot.Say(to, deer+" by "+creator+modsSuffix(used))
	}
	p.paint.Unlock()

	p.log.Info("deer walked", "deer", deer, "target", to, "nick", m.Nick,
		"lines", len(lines), "mods", used)
	p.remember(to, sighting{deer: deer, creator: creator, nick: m.Nick, mods: used, seen: true})
}

func (p *Plugin) cmdPrevDeer(m *bot.Message) {
	if p.bannedBy(m) != "" {
		return
	}
	to := m.ReplyTo()
	if !p.maySpeak("prev:" + to) {
		return
	}

	p.mu.Lock()
	last, ok := p.last[to]
	if !ok {
		last = p.latest
	}
	p.mu.Unlock()

	if !last.seen {
		p.bot.Say(to, "No deer has been sighted yet!")
		return
	}
	suffix := ""
	if len(last.mods) > 0 {
		suffix = " (with the following mods: " + strings.Join(last.mods, ", ") + ")"
	}
	p.bot.Say(to, "The previous deer to walk the earth was "+last.deer+" by "+last.creator+suffix)
}

func (p *Plugin) fetch(ctx context.Context, name string) (*row, error) {
	switch name {
	case "random":
		return p.db.random(ctx)
	case "latest":
		return p.db.latest(ctx)
	default:
		return p.db.byName(ctx, name)
	}
}

// claim takes the target's deer slot. When it is still warm it returns how long
// is left and the timeout that decided it, which is what the refusal quotes.
func (p *Plugin) claim(target, nick, name string, mods []byte, user User, privileged bool) (wait, timeout time.Duration, ok bool) {
	p.mu.Lock()
	timeout = p.timeoutFor(target, nick, name, mods)
	p.mu.Unlock()

	if privileged && user.Timeout != nil {
		timeout = time.Duration(*user.Timeout) * time.Second
	}
	wait, ok = p.used.ClaimFor(target, timeout)
	return wait, timeout, ok
}

func (p *Plugin) timeoutFor(target, nick, name string, mods []byte) time.Duration {
	timeout := p.cfg.Wait()
	last, ok := p.last[target]
	if !ok {
		return timeout
	}
	repeatNick := strings.EqualFold(last.nick, nick)
	repeatDeer := name != "random" && strings.EqualFold(last.deer, name) && !isX(mods)
	if repeatNick || repeatDeer {
		return time.Duration(float64(timeout) * p.cfg.TimeoutPunish)
	}
	return timeout
}

func (p *Plugin) maySpeak(key string) bool {
	_, ok := p.spoke.Claim(key)
	return ok
}

// penalise puts a target on the shorter miss cooldown.
func (p *Plugin) penalise(target string) {
	p.used.Delay(target, p.cfg.MissWait())
}

func (p *Plugin) remember(target string, s sighting) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.last[target] = s
	p.latest = s
}

// waitMessage explains a refusal. timeout is the one claim actually applied,
// which is not always the configured one: a privileged nick has its own, and a
// repeated nick or deer is punished with a longer one.
func (p *Plugin) waitMessage(wait, timeout time.Duration, user User, privileged bool) string {
	seconds := int(wait.Seconds()) + 1
	normal := p.cfg.Timeout
	if privileged && user.Timeout != nil {
		switch {
		case *user.Timeout < normal:
			return fmt.Sprintf("You are privileged! You have a LOWER timeout than others (-%d seconds), which is like %d seconds from now, bro.",
				normal-*user.Timeout, seconds)
		case *user.Timeout > normal:
			return fmt.Sprintf("You have somehow been punished! You have a HIGHER timeout than others (+%d seconds), which is like %d seconds from now, bro.",
				*user.Timeout-normal, seconds)
		}
	}
	if applied := int(timeout.Round(time.Second).Seconds()); applied > normal {
		return fmt.Sprintf("Deer called, but deer not so fast :( Asking again yourself, or for the same deer, stretches the usual %d seconds to %d, which is like %d seconds from now, bro.",
			normal, applied, seconds)
	}
	return fmt.Sprintf("Deer called, but deer not so fast :( It only walks the earth every %d seconds, which is like %d seconds from now, bro.",
		normal, seconds)
}

func (p *Plugin) sayHelp(m *bot.Message, to string) {
	prefix := p.bot.Prefix()
	status := "Ready to deer!"

	if wait := p.used.Until(to); wait > 0 {
		status = strconv.Itoa(int(wait.Seconds())+1) + " seconds until deer."
	}

	total := ""
	if n, err := p.total(); err != nil {
		p.log.Warn("deer count", "err", err)
	} else {
		total = fmt.Sprintf("(%d deer total) ", n)
	}

	p.bot.Say(m.Nick, boldCode+"How to deer:"+boldCode+" Type "+prefix+
		"deerme <mods>|<deer> to deer, "+prefix+"deerme random or "+prefix+
		"deerme latest to take what you are given, or "+prefix+
		"deerme help modifiers for the available mods. "+total+
		boldCode+"Status: "+boldCode+status+" "+
		boldCode+"Create your own: "+boldCode+p.cfg.Editor)
}

func (p *Plugin) sayModifiers(m *bot.Message) {
	mods := make([]string, 0, len(modifierOrder))
	for _, c := range modifierOrder {
		name := modifierNames[c]
		if c == 'x' {
			name = "a random pile of the others"
		}
		mods = append(mods, string(c)+"(="+name+")")
	}
	p.bot.Say(m.Nick, boldCode+"Available modifiers: "+boldCode+strings.Join(mods, ", ")+
		". Stack them before a pipe, like "+p.bot.Prefix()+"deerme iu|senordeer.")
}

// total returns the size of the gallery, cached so repeated help requests don't
// each become a query.
func (p *Plugin) total() (int, error) {
	p.mu.Lock()
	if time.Since(p.counted) < countTTL {
		n := p.count
		p.mu.Unlock()
		return n, nil
	}
	p.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.RequestTimeout())
	defer cancel()
	n, err := p.db.count(ctx)
	if err != nil {
		return 0, err
	}

	p.mu.Lock()
	p.count, p.counted = n, time.Now()
	p.mu.Unlock()
	return n, nil
}

// bannedBy returns which field matched the ignore lists, or "".
func (p *Plugin) bannedBy(m *bot.Message) string {
	switch {
	case p.banNicks.Has(m.Nick):
		return "nick"
	case p.banHosts.Has(m.Host):
		return "host"
	case m.FromChannel() && p.banChannels.Has(m.Target):
		return "channel"
	}
	return ""
}

// privilegedFor matches the sender against the privileged list on whichever of
// nick and host the config asks for. Listing both (the default) means both must
// match, the same bar the bot's admin commands use.
func (p *Plugin) privilegedFor(m *bot.Message) (User, bool) {
	who := p.rule.Find(p.hosts, m.Nick, m.Host)
	if !who.OK {
		if who.Listed {
			p.log.Warn("privileged deer denied: host mismatch",
				"nick", m.Nick, "gotHost", m.Host, "wantHost", who.WantHost)
		}
		return User{}, false
	}
	return p.cfg.Privileged[who.Key], true
}

// privilegedHosts is the privileged list keyed the way access.Find reads it.
func privilegedHosts(users map[string]User) map[string]string {
	hosts := make(map[string]string, len(users))
	for nick, user := range users {
		hosts[strings.ToLower(nick)] = user.Host
	}
	return hosts
}

// lineBudget is how many bytes of art fit on one line to this target.
func lineBudget(target string) int {
	if budget := irctext.LineBudget(target); budget > len(resetCode) {
		return budget
	}
	return len(resetCode)
}

func isX(mods []byte) bool { return len(mods) == 1 && mods[0] == 'x' }

func modsSuffix(used []string) string {
	if len(used) == 0 {
		return ""
	}
	return " (" + strings.Join(used, ", ") + ")"
}

// sanitiseName reduces a request to what the gallery accepts as a name, which
// is also all the query needs to match on.
func sanitiseName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteByte(' ')
		}
	}
	return head(strings.Join(strings.Fields(b.String()), " "), maxNameLen)
}

// sanitiseText makes a value read from the database safe to put in a credit
// line, and short enough to leave room for the rest of it. The web app already
// filters what Clean strips, but art migrated from the old gallery predates
// that.
func sanitiseText(s string) string {
	return irctext.Truncate(irctext.Clean(s), maxTextLen)
}

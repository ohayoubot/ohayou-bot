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
	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/d1"
)

const (
	defaultDeer = "deer"
	maxNameLen  = 48
	maxTextLen  = 64
	countTTL    = 5 * time.Minute
	// chatterWait spaces out the replies that aren't art, per asker.
	chatterWait = 15 * time.Second
	maxTracked  = 1000
	// ircLineLimit is the protocol's 512 bytes for a whole line, including the
	// "PRIVMSG <target> :" the bot writes and the trailing CRLF.
	ircLineLimit = 512
)

type Plugin struct {
	bot *bot.Bot
	cfg config.DeerkinsConfig
	log *slog.Logger
	db  *gallery

	banNicks    map[string]bool
	banHosts    map[string]bool
	banChannels map[string]bool

	roll func(int) int

	// paint serializes the multi-line output so two deer can't interleave.
	paint sync.Mutex

	mu      sync.Mutex
	used    map[string]time.Time // when a target last got a deer
	spoke   map[string]time.Time // when a key last got a reply that isn't art
	last    map[string]sighting  // the deer a target last saw
	latest  sighting             // the last deer anywhere
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

func New(b *bot.Bot, cfg config.DeerkinsConfig) *Plugin {
	return &Plugin{
		bot:         b,
		cfg:         cfg,
		log:         b.Logger().With("plugin", "deerkins"),
		db:          newGallery(d1.APIBase, cfg.AccountID, cfg.DatabaseID, cfg.APIToken, cfg.RequestTimeout()),
		banNicks:    lowerSet(cfg.IgnoreNicks),
		banHosts:    lowerSet(cfg.IgnoreHosts),
		banChannels: lowerSet(cfg.IgnoreChannels),
		roll:        rand.IntN,
		used:        map[string]time.Time{},
		spoke:       map[string]time.Time{},
		last:        map[string]sighting{},
	}
}

func (p *Plugin) Register() {
	p.bot.HandleFunc("deerme", false, p.cmdDeerme)
	p.bot.HandleFunc("prevdeer", false, p.cmdPrevDeer)
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

	to := replyTo(m)
	mods, name := splitRequest(strings.Join(m.Args[1:], " "))
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
	to := replyTo(m)
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
func (p *Plugin) claim(target, nick, name string, mods []byte, user config.DeerkinsUser, privileged bool) (wait, timeout time.Duration, ok bool) {
	now := time.Now()

	p.mu.Lock()
	defer p.mu.Unlock()

	timeout = p.timeoutFor(target, nick, name, mods)
	if privileged && user.Timeout != nil {
		timeout = time.Duration(*user.Timeout) * time.Second
	}
	if ready := p.used[target].Add(timeout); now.Before(ready) {
		return ready.Sub(now), timeout, false
	}
	p.used[target] = now
	return 0, timeout, true
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
	now := time.Now()

	p.mu.Lock()
	defer p.mu.Unlock()

	if now.Sub(p.spoke[key]) < chatterWait {
		return false
	}
	// Keys are nicks and channels, so the map only grows if someone keeps
	// changing nick. Drop the expired ones when it gets big.
	if len(p.spoke) > maxTracked {
		for k, when := range p.spoke {
			if now.Sub(when) >= chatterWait {
				delete(p.spoke, k)
			}
		}
	}
	p.spoke[key] = now
	return true
}

// penalise puts a target on the shorter miss cooldown.
func (p *Plugin) penalise(target string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.used[target] = time.Now().Add(p.cfg.MissWait() - p.cfg.Wait())
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
func (p *Plugin) waitMessage(wait, timeout time.Duration, user config.DeerkinsUser, privileged bool) string {
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

	p.mu.Lock()
	ready := p.used[to].Add(p.cfg.Wait())
	p.mu.Unlock()
	if wait := time.Until(ready); wait > 0 {
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
	case p.banNicks[strings.ToLower(m.Nick)]:
		return "nick"
	case p.banHosts[strings.ToLower(m.Host)]:
		return "host"
	case m.FromChannel() && p.banChannels[strings.ToLower(m.Target)]:
		return "channel"
	}
	return ""
}

// privilegedFor matches the sender against the privileged list on whichever of
// nick and host the config asks for. Listing both (the default) means both must
// match, the same bar the bot's admin commands use, since a nick alone is
// trivially borrowed.
func (p *Plugin) privilegedFor(m *bot.Message) (config.DeerkinsUser, bool) {
	byNick, byHost := p.cfg.MatchNick(), p.cfg.MatchHost()
	if !byNick && !byHost {
		return config.DeerkinsUser{}, false
	}

	if !byNick {
		for _, user := range p.cfg.Privileged {
			if user.Host != "" && strings.EqualFold(user.Host, m.Host) {
				return user, true
			}
		}
		return config.DeerkinsUser{}, false
	}

	user, ok := p.cfg.Privileged[strings.ToLower(m.Nick)]
	if !ok {
		return config.DeerkinsUser{}, false
	}
	if byHost && !strings.EqualFold(user.Host, m.Host) {
		p.log.Warn("privileged deer denied: host mismatch",
			"nick", m.Nick, "gotHost", m.Host, "wantHost", user.Host)
		return config.DeerkinsUser{}, false
	}
	return user, true
}

// replyTo is the channel a command came from, or the sender for a private one.
func replyTo(m *bot.Message) string {
	if m.FromChannel() {
		return m.Target
	}
	return m.Nick
}

// lineBudget is how many bytes of art fit on one line to this target.
func lineBudget(target string) int {
	budget := ircLineLimit - len("PRIVMSG ") - len(target) - len(" :") - len("\r\n")
	if budget < len(resetCode) {
		return len(resetCode)
	}
	return budget
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

// sanitiseText strips anything that could steer the irc line (colour codes,
// CR, LF, bidi overrides) out of a value read from the database. The web app
// already filters these, but art migrated from the old gallery predates that.
func sanitiseText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToValidUTF8(s, "") {
		switch {
		case r == '\r', r == '\n', r == '\t':
			b.WriteByte(' ') // keep the words apart
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
		default:
			b.WriteRune(r)
		}
	}
	out := []rune(strings.Join(strings.Fields(b.String()), " "))
	if len(out) > maxTextLen {
		out = out[:maxTextLen]
	}
	return string(out)
}

func lowerSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
			set[v] = true
		}
	}
	return set
}

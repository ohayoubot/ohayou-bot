// Package youtube names the videos linked in a channel. It watches the lines
// nobody addressed to the bot, and when one carries a youtube link it says what
// the video is called and who made it.
package youtube

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/config"
)

const ircLineLimit = 512

// maxTracked caps the seen maps so a busy network cannot grow them without
// bound.
const maxTracked = 1000

type Plugin struct {
	bot *bot.Bot
	cfg config.YouTubeConfig
	log *slog.Logger
	api *client

	banChannels map[string]bool

	now func() time.Time

	mu    sync.Mutex
	spoke map[string]time.Time // target -> when it last got a preview
	said  map[string]time.Time // target+id -> when that video was last named
}

func New(b *bot.Bot, cfg config.YouTubeConfig) *Plugin {
	return &Plugin{
		bot:         b,
		cfg:         cfg,
		log:         b.Logger().With("plugin", "youtube"),
		api:         newClient(oembedBase, cfg.RequestTimeout()),
		banChannels: lowerSet(cfg.IgnoreChannels),
		now:         time.Now,
		spoke:       map[string]time.Time{},
		said:        map[string]time.Time{},
	}
}

func (p *Plugin) Register() { p.bot.Watch(p.onLine) }

func (p *Plugin) onLine(m *bot.Message) {
	// A private message arrives addressed to the bot's own nick, which is no
	// use as somewhere to reply or as a key to hold a cooldown against.
	to := m.Target
	if !m.FromChannel() {
		to = m.Nick
	}

	if p.banChannels[strings.ToLower(to)] {
		return
	}

	ids := videoIDs(m.Text, p.cfg.MaxLinks)
	if len(ids) == 0 {
		return
	}

	// The channel's turn is taken before anything is fetched, not after: a
	// burst of links should cost one round of lookups, whatever order the
	// watchers happen to run in.
	if !p.claim(to) {
		p.log.Debug("preview skipped", "reason", "cooldown", "target", to)
		return
	}

	for _, id := range ids {
		if !p.fresh(to, id) {
			p.log.Debug("preview skipped", "reason", "already said", "target", to, "id", id)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), p.cfg.RequestTimeout())
		v, err := p.api.lookup(ctx, id)
		cancel()

		if err != nil {
			// A video youtube won't describe is not worth a line in the
			// channel; anything else is worth a line in the log.
			if !errors.Is(err, errNoVideo) {
				p.log.Warn("looking up a video", "id", id, "err", err)
			} else {
				p.log.Debug("no such video", "id", id)
			}
			continue
		}

		p.bot.Say(to, line(to, v))
		p.log.Info("preview", "id", id, "target", to, "nick", m.Nick, "title", v.Title)
	}
}

// claim takes the target's turn, reporting whether it was free.
func (p *Plugin) claim(target string) bool {
	key := strings.ToLower(target)
	now := p.now()

	p.mu.Lock()
	defer p.mu.Unlock()

	if last, ok := p.spoke[key]; ok && now.Sub(last) < p.cfg.CooldownWait() {
		return false
	}
	if len(p.spoke) >= maxTracked {
		forgetOld(p.spoke, now, p.cfg.CooldownWait())
	}
	p.spoke[key] = now
	return true
}

// fresh reports whether this video is worth naming in this target again, and
// records that it was.
func (p *Plugin) fresh(target, id string) bool {
	key := strings.ToLower(target) + " " + id
	now := p.now()

	p.mu.Lock()
	defer p.mu.Unlock()

	if last, ok := p.said[key]; ok && now.Sub(last) < p.cfg.RepeatWait() {
		return false
	}
	if len(p.said) >= maxTracked {
		forgetOld(p.said, now, p.cfg.RepeatWait())
	}
	p.said[key] = now
	return true
}

// forgetOld drops entries whose window has run out. Called with mu held.
func forgetOld(seen map[string]time.Time, now time.Time, window time.Duration) {
	for key, last := range seen {
		if now.Sub(last) >= window {
			delete(seen, key)
		}
	}
}

// line is what the channel sees.
func line(target string, v video) string {
	msg := "YouTube: " + clean(v.Title)
	if author := clean(v.Author); author != "" {
		msg += " (" + author + ")"
	}
	return fit(target, msg)
}

// clean flattens a title into something safe to send: one line, no control
// characters, no runs of whitespace. A title is whatever its uploader typed,
// including the colour codes and newlines that would otherwise let them write
// the rest of the bot's output for it.
func clean(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		switch {
		// Newlines and tabs are whitespace before they are control characters.
		// they separate words, so they leave a space behind rather than
		// joining what they sat between.
		case unicode.IsSpace(r):
			space = b.Len() > 0
		case r == utf8.RuneError, unicode.IsControl(r):
			continue
		default:
			if space {
				b.WriteRune(' ')
				space = false
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fit trims a message to what will survive the trip to the target as one line.
func fit(target, msg string) string {
	budget := ircLineLimit - len("PRIVMSG "+target+" :") - len("\r\n")
	if budget < 1 {
		return ""
	}
	if len(msg) <= budget {
		return msg
	}

	const ellipsis = "..."
	cut := budget - len(ellipsis)
	if cut < 1 {
		return msg[:budget]
	}
	// Never split a rune in half.
	for cut > 0 && !utf8.RuneStart(msg[cut]) {
		cut--
	}
	return strings.TrimRight(msg[:cut], " ") + ellipsis
}

func lowerSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[strings.ToLower(strings.TrimSpace(n))] = true
	}
	return set
}

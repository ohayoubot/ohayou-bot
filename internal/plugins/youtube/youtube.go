// Package youtube names the videos linked in a channel. It watches the lines
// nobody addressed to the bot, and when one carries a youtube link it says what
// the video is called and who made it.
package youtube

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/bot/irctext"
	"github.com/ohayoubot/ohayou-bot/internal/bot/ratelimit"
	"github.com/ohayoubot/ohayou-bot/internal/config"
)

type Plugin struct {
	bot *bot.Bot
	cfg config.YouTubeConfig
	log *slog.Logger
	api *client

	banChannels map[string]bool

	now func() time.Time

	spoke *ratelimit.Limiter // when a target last got a preview
	said  *ratelimit.Limiter // when a video was last named in a target
}

func New(b *bot.Bot, cfg config.YouTubeConfig) *Plugin {
	p := &Plugin{
		bot:         b,
		cfg:         cfg,
		log:         b.Logger().With("plugin", "youtube"),
		api:         newClient(oembedBase, cfg.RequestTimeout()),
		banChannels: lowerSet(cfg.IgnoreChannels),
		now:         time.Now,
		spoke:       ratelimit.New(cfg.CooldownWait()),
		said:        ratelimit.New(cfg.RepeatWait()),
	}
	// Read through p.now so a test can wind the clock after construction.
	clock := func() time.Time { return p.now() }
	p.spoke.Now, p.said.Now = clock, clock
	return p
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
	_, ok := p.spoke.Claim(strings.ToLower(target))
	return ok
}

// fresh reports whether this video is worth naming in this target again, and
// records that it was.
func (p *Plugin) fresh(target, id string) bool {
	_, ok := p.said.Claim(strings.ToLower(target) + " " + id)
	return ok
}

// line is what the channel sees.
func line(target string, v video) string {
	msg := "YouTube: " + irctext.Clean(v.Title)
	if author := irctext.Clean(v.Author); author != "" {
		msg += " (" + author + ")"
	}
	return irctext.Fit(target, msg)
}

func lowerSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[strings.ToLower(strings.TrimSpace(n))] = true
	}
	return set
}

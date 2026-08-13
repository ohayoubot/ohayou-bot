package ohayou

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// The chronicle: what the game remembers of itself. Written where a mutation
// has already succeeded, read by !news and published to the site.
const (
	eventSettle = "settle" // first ration ever
	eventLand   = "land"   // acres bought
	eventBuild  = "build"  // a building raised
	eventStrike = "strike" // a rare metal out of a quarry run
	eventSteal  = "steal"  // a robbery that landed
	eventCaught = "caught" // one that did not
	eventCat    = "cat"    // the stray taken in
	eventFlag   = "flag"   // a deer run up or struck
	eventDouble = "double" // the distributor malfunctioned
)

const (
	// eventLog is how many rows the bot keeps.
	eventLog = 500
	// eventFeed is how many of them the site is given.
	eventFeed = 200
	// newsLines is how many lines !news spends in a channel.
	newsLines = 3
	// newsWidth is the room one of them has, well inside an irc message.
	newsWidth = 390
)

// record appends to the chronicle. Fire and forget: every call site has already
// changed the game, and a lost line is not worth failing that.
func (g *Plugin) record(kind, actor, subject string, detail map[string]string) {
	e := store.Event{
		TS:      time.Now(),
		Kind:    kind,
		Actor:   strings.ToLower(actor),
		Subject: strings.ToLower(subject),
		Detail:  detail,
	}
	if err := g.store.RecordEvent(g.ctx(), e, eventLog); err != nil {
		g.log.Error("recording an event", "kind", kind, "actor", actor, "err", err)
	}
}

// bands are checked in order, so each below is its band's ceiling. Lifted from
// wealthBands: an exact haul says what a plot is worth taking today.
var bands = []struct {
	below int
	name  string
}{
	{25, "a handful"},
	{100, "a pocketful"},
	{500, "a purse"},
	{2000, "a sack"},
	{1 << 30, "a haul"},
}

func band(n int) string {
	for _, b := range bands {
		if n < b.below {
			return b.name
		}
	}
	return bands[len(bands)-1].name
}

// phrase is one event as a sentence, without its full stop. Empty when the
// event is one this bot has no words for, which a caller drops.
func phrase(e store.Event) string {
	who := e.Actor
	if who == "" {
		who = "somebody"
	}
	switch e.Kind {
	case eventSettle:
		return who + " settled, taking their first ration"
	case eventLand:
		n, _ := strconv.Atoi(e.Detail["acres"])
		return fmt.Sprintf("%s bought %d %s", who, n, plural(n, "acre"))
	case eventBuild:
		return who + " raised a " + e.Detail["thing"]
	case eventStrike:
		return who + " struck " + e.Detail["metal"] + " in the quarry"
	case eventSteal:
		return fmt.Sprintf("%s robbed %s of %s", who, e.Subject, e.Detail["took"])
	case eventCaught:
		return who + " was caught robbing " + e.Subject + ", fined and put on probation"
	case eventCat:
		return who + " took in the stray cat"
	case eventFlag:
		if e.Detail["deer"] == "" {
			return who + " struck their flag"
		}
		return who + " ran up the deer named " + e.Detail["deer"]
	case eventDouble:
		return "the ohayou distributor malfunctioned"
	}
	return ""
}

// ago is how long since, in the one unit that reads.
func ago(since time.Duration) string {
	switch {
	case since < time.Minute:
		return "just now"
	case since < time.Hour:
		return fmt.Sprintf("%dm", int(since.Minutes()))
	case since < 24*time.Hour:
		return fmt.Sprintf("%dh", int(since.Hours()))
	default:
		return fmt.Sprintf("%dd", int(since.Hours()/24))
	}
}

func (g *Plugin) cmdNews(m *bot.Message) {
	to := m.ReplyTo()
	if m.HasArgs() {
		g.newsAbout(to, strings.ToLower(m.Arg(1)), m.Arg(1))
		return
	}

	events, err := g.store.RecentEvents(g.ctx(), eventLog)
	if err != nil {
		g.log.Error("reading the chronicle", "err", err)
		g.say(to, "I can't remember just now. Try again in a moment.")
		return
	}
	if len(events) == 0 {
		g.say(to, "Nothing has happened yet.")
		return
	}

	lines := pack(events, time.Now(), newsLines, newsWidth)
	g.say(to, "Lately: "+lines[0])
	for _, line := range lines[1:] {
		g.say(to, line)
	}
	if url := g.chronicleURL(); url != "" {
		g.say(to, "The rest of it: "+url)
	}
}

func (g *Plugin) newsAbout(to, nick, raw string) {
	events, err := g.store.EventsAbout(g.ctx(), nick, eventLog)
	if err != nil {
		g.log.Error("reading the chronicle", "nick", nick, "err", err)
		g.say(to, "I can't remember just now. Try again in a moment.")
		return
	}
	if len(events) == 0 {
		g.say(to, "Nothing on file for "+raw+".")
		return
	}
	g.say(to, raw+": "+pack(events, time.Now(), 1, newsWidth)[0])
}

// pack lays events into at most max lines of width, newest first, dropping the
// ones that do not fit. Always returns at least one line.
func pack(events []store.Event, now time.Time, max, width int) []string {
	var lines []string
	var line string

	for _, e := range events {
		said := phrase(e)
		if said == "" {
			continue
		}
		part := said + " (" + ago(now.Sub(e.TS)) + ")."
		switch {
		case line == "":
			line = part
		case len(line)+1+len(part) <= width:
			line += " " + part
		default:
			lines = append(lines, line)
			if len(lines) == max {
				return lines
			}
			line = part
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return []string{"Nothing worth writing down."}
	}
	return lines
}

// chronicleURL is where the whole of it is, when there is a site.
func (g *Plugin) chronicleURL() string {
	return g.pageURL("/ohayou/lately")
}

// handbookURL is the written-out game, when there is a site.
func (g *Plugin) handbookURL() string {
	return g.pageURL("/ohayou/help")
}

// pageURL hangs a path off the configured site, or returns "" when there is none.
func (g *Plugin) pageURL(path string) string {
	if g.cfg.SiteURL == "" {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSuffix(g.cfg.SiteURL, "#"), "/") + path
}

// chronicle is the published tier: the newest events, with an actor named only
// where their plot is. The same rule the map runs on, so a player drawn as
// Anonymous is not named by the feed beside it.
func (g *Plugin) chronicle(events []store.Event, vis map[string]store.Visibility) []Event {
	shown := func(nick string) string {
		if nick == "" || vis[nick] == store.VisibilityHidden {
			return ""
		}
		return nick
	}

	out := make([]Event, 0, len(events))
	for _, e := range events {
		if phrase(e) == "" {
			continue
		}
		// A robbery names its victim only if they are on the map under their
		// own name, the same as its thief.
		out = append(out, Event{
			ID:      e.ID,
			TS:      e.TS.Unix(),
			Kind:    e.Kind,
			Actor:   shown(e.Actor),
			Subject: shown(e.Subject),
			Detail:  detailOf(e),
		})
		if len(out) == eventFeed {
			break
		}
	}
	return out
}

// detailOf is never nil, so an empty one marshals to {} rather than null.
func detailOf(e store.Event) map[string]string {
	out := map[string]string{}
	for k, v := range e.Detail {
		out[k] = v
	}
	return out
}

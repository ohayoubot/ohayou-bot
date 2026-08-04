package bot

import (
	"sort"
	"strings"

	"github.com/ohayoubot/ohayou-bot/internal/bot/irctext"
)

// Topic is one subject !help can explain. Plugins register their own during
// Register, when the command prefix is known and can be written into Lines.
type Topic struct {
	// Name is what !help <name> answers to, lowercased.
	Name string
	// Summary is the one-line description in the index.
	Summary string
	// Lines are what !help <name> says, one message each.
	Lines []string
	// Aliases are other words that reach this topic, such as the commands it
	// covers, so !help steal finds the topic about stealing.
	Aliases []string
}

// Help registers topics. Later registrations override earlier ones, and an
// alias never displaces a real topic name.
func (b *Bot) Help(topics ...Topic) {
	for _, t := range topics {
		t.Name = strings.ToLower(t.Name)
		b.topics = append(b.topics, t)
		for _, alias := range t.Aliases {
			b.topicAlias[strings.ToLower(alias)] = t.Name
		}
	}
}

// topic resolves a word to a topic, following aliases.
func (b *Bot) topic(want string) (Topic, bool) {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, t := range b.topics {
		if t.Name == want {
			return t, true
		}
	}
	if name, ok := b.topicAlias[want]; ok {
		for _, t := range b.topics {
			if t.Name == name {
				return t, true
			}
		}
	}
	return Topic{}, false
}

func (b *Bot) registerHelpCommands() {
	b.HandleFunc("help", false, b.cmdHelp)
	b.HandleFunc("commands", false, b.cmdCommands)
}

func (b *Bot) cmdHelp(m *Message) {
	to := m.ReplyTo()
	p := b.cfg.CommandPrefix

	if m.HasArgs() {
		if t, ok := b.topic(m.Rest(1)); ok {
			for _, line := range t.Lines {
				b.Say(to, line)
			}
			return
		}
		b.Say(to, "No help for \""+m.Rest(1)+"\". Type "+p+"help to see the topics.")
		return
	}

	if len(b.topics) == 0 {
		b.Say(to, "I have no help to give. Type "+p+"commands to see what I answer to.")
		return
	}
	b.Say(to, "Type "+p+"help <topic> to read up on one, or "+p+
		"commands for every command I answer to.")
	for _, line := range b.helpIndex(to) {
		b.Say(to, line)
	}
}

// helpIndex packs "name - summary" into as few lines as the target allows,
// rather than spending a line on each of them.
func (b *Bot) helpIndex(target string) []string {
	budget := irctext.LineBudget(target)

	var lines []string
	var sb strings.Builder
	for _, t := range b.topics {
		entry := t.Name
		if t.Summary != "" {
			entry += " (" + t.Summary + ")"
		}
		switch {
		case sb.Len() == 0:
			sb.WriteString("Topics: " + entry)
		case sb.Len()+len("; ")+len(entry) > budget:
			lines = append(lines, sb.String())
			sb.Reset()
			sb.WriteString(entry)
		default:
			sb.WriteString("; " + entry)
		}
	}
	if sb.Len() > 0 {
		lines = append(lines, sb.String())
	}
	return lines
}

// cmdCommands lists what the bot answers to, read off the command table itself
// rather than a list somebody has to remember to update.
func (b *Bot) cmdCommands(m *Message) {
	b.Say(m.ReplyTo(), "Commands: "+strings.Join(b.commandNames(m.Admin), " "))
}

// commandNames is every command the asker may run, prefixed and sorted. Admin
// commands are listed only to admins, since they are no use to anyone else.
func (b *Bot) commandNames(admin bool) []string {
	names := make([]string, 0, len(b.commands))
	for name, cmd := range b.commands {
		if cmd.Admin && !admin {
			continue
		}
		names = append(names, b.cfg.CommandPrefix+name)
	}
	sort.Strings(names)
	return names
}

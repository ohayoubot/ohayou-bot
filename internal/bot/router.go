package bot

import (
	"strings"

	irc "github.com/ohayoubot/go-ircevent"
)

func (b *Bot) onPrivmsg(e *irc.Event) {
	text := e.Message()
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return
	}

	host, isAdmin := b.cfg.Admins[strings.ToLower(e.Nick)]
	admin := isAdmin && host == e.Host

	m := &Message{
		Prefix: b.cfg.CommandPrefix,
		Text:   text,
		Args:   fields,
		Target: e.Arguments[0],
		Nick:   e.Nick,
		Host:   e.Host,
		Admin:  admin,
	}

	cmd, ok := b.lookup(fields[0])
	if !ok {
		// Not addressed to the bot. Watchers get the line instead, on the same
		// terms as a command: nothing from an ignored nick reaches them.
		if !b.isIgnored(e.Nick) {
			for _, w := range b.watchers {
				go w(m)
			}
		}
		return
	}
	m.Command = cmd.Name

	if cmd.Admin && !admin {
		// A configured admin whose current host doesn't
		// match the mask is the most likely issue. log it
		if isAdmin {
			b.log.Warn("admin command denied: host mismatch",
				"command", cmd.Name, "nick", e.Nick, "gotHost", e.Host, "wantHost", host)
		}
		return
	}

	// Non-admin commands from ignored users are dropped.
	if !admin && b.isIgnored(e.Nick) {
		b.log.Debug("ignored", "nick", e.Nick)
		return
	}

	b.log.Debug("dispatch", "command", cmd.Name, "nick", e.Nick, "target", e.Arguments[0], "admin", admin)

	go cmd.Handler(m)
}

// lookup resolves the first word of a message to a command.
func (b *Bot) lookup(word string) (Command, bool) {
	prefix := b.cfg.CommandPrefix
	if prefix != "" && !strings.HasPrefix(word, prefix) {
		return Command{}, false
	}
	cmd, ok := b.commands[strings.ToLower(strings.TrimPrefix(word, prefix))]
	return cmd, ok
}

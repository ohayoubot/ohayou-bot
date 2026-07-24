package bot

import (
	"strings"

	irc "github.com/thoj/go-ircevent"
)

func (b *Bot) onPrivmsg(e *irc.Event) {
	fields := strings.Fields(e.Message())
	if len(fields) == 0 {
		return
	}

	prefix := b.cfg.CommandPrefix
	word := fields[0]
	if prefix != "" && !strings.HasPrefix(word, prefix) {
		return
	}
	name := strings.ToLower(strings.TrimPrefix(word, prefix))

	cmd, ok := b.commands[name]
	if !ok {
		return
	}

	host, isAdmin := b.cfg.Admins[strings.ToLower(e.Nick)]
	admin := isAdmin && host == e.Host
	if cmd.Admin && !admin {
		// A configured admin whose current host doesn't
		// match the mask is the most likely issue. log it
		if isAdmin {
			b.log.Warn("admin command denied: host mismatch",
				"command", name, "nick", e.Nick, "gotHost", e.Host, "wantHost", host)
		}
		return
	}

	// Non-admin commands from ignored users are dropped.
	if !admin && b.isIgnored(e.Nick) {
		b.log.Debug("ignored", "nick", e.Nick)
		return
	}

	b.log.Debug("dispatch", "command", name, "nick", e.Nick, "target", e.Arguments[0], "admin", admin)

	m := &Message{
		Prefix:  prefix,
		Command: name,
		Args:    fields,
		Target:  e.Arguments[0],
		Nick:    e.Nick,
		Host:    e.Host,
		Admin:   admin,
	}
	go cmd.Handler(m)
}

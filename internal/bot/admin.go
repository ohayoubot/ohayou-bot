package bot

import "strings"

func (b *Bot) registerAdminCommands() {
	admin := func(name string, h Handler) { b.Handle(Command{Name: name, Admin: true, Handler: h}) }

	// say #chan hello   -> into #chan
	// say hello         -> into the channel the command came from
	admin("say", func(m *Message) {
		if !m.HasArgs() {
			return
		}
		if isChannel(m.Args[1]) {
			b.Say(m.Args[1], strings.Join(m.Args[2:], " "))
		} else {
			b.Say(m.Target, strings.Join(m.Args[1:], " "))
		}
	})

	// pm <target> hello
	admin("pm", func(m *Message) {
		if len(m.Args) > 2 {
			b.Say(m.Args[1], strings.Join(m.Args[2:], " "))
		}
	})

	// join #channel [key]
	admin("join", func(m *Message) {
		if m.HasArgs() && isChannel(m.Args[1]) {
			b.log.Info("admin join", "by", m.Nick, "channel", m.Args[1])
			b.conn.Join(strings.Join(m.Args[1:], " "))
			b.addChannel(m.Args[1])
		}
	})

	// part            -> the current channel
	// part #channel
	admin("part", func(m *Message) {
		target := m.Target
		if m.HasArgs() && isChannel(m.Args[1]) {
			target = m.Args[1]
		}
		if isChannel(target) {
			b.log.Info("admin part", "by", m.Nick, "channel", target)
			b.conn.Part(target)
			b.removeChannel(target)
		}
	})

	// notice <target> hello
	admin("notice", func(m *Message) {
		if len(m.Args) > 2 {
			b.Notice(m.Args[1], strings.Join(m.Args[2:], " "))
		}
	})

	// me hello          -> action in the current channel
	// me #channel hello
	admin("me", func(m *Message) {
		if !m.HasArgs() {
			return
		}
		if isChannel(m.Args[1]) {
			b.Action(m.Args[1], strings.Join(m.Args[2:], " "))
		} else {
			b.Action(m.Target, strings.Join(m.Args[1:], " "))
		}
	})

	// kick <user> [reason]
	// kick #channel <user> [reason]
	admin("kick", func(m *Message) {
		if !m.HasArgs() {
			return
		}
		if isChannel(m.Args[1]) {
			if len(m.Args) > 2 {
				b.log.Info("admin kick", "by", m.Nick, "channel", m.Args[1], "target", m.Args[2])
				b.conn.Kick(m.Args[2], m.Args[1], strings.Join(m.Args[3:], " "))
			}
		} else {
			b.log.Info("admin kick", "by", m.Nick, "channel", m.Target, "target", m.Args[1])
			b.conn.Kick(m.Args[1], m.Target, strings.Join(m.Args[2:], " "))
		}
	})

	// identify   -> identify with NickServ
	admin("identify", func(m *Message) {
		if b.cfg.NickPW != "" {
			b.Say("NickServ", "identify "+b.cfg.NickPW)
		}
	})

	// channels   -> list joined channels
	admin("channels", func(m *Message) {
		b.Say(m.Target, strings.Join(b.Channels(), " "))
	})

	// ignore <user> [reason]
	admin("ignore", func(m *Message) {
		if !m.HasArgs() {
			return
		}
		reason := "No reason given."
		if len(m.Args) > 2 {
			reason = strings.Join(m.Args[2:], " ")
		}
		b.addIgnore(m.Args[1], reason)
		b.log.Info("admin ignore", "by", m.Nick, "nick", m.Args[1], "reason", reason)
		b.Say(m.Target, "Added "+m.Args[1]+" to ignore list. Reason: "+reason)
	})

	// ignorelist
	admin("ignorelist", func(m *Message) {
		var sb strings.Builder
		for n, r := range b.ignoreSnapshot() {
			sb.WriteString("[ " + n + ", Reason: " + r + "] ")
		}
		b.Say(m.Nick, sb.String())
	})

	// unignore <user>
	admin("unignore", func(m *Message) {
		if !m.HasArgs() {
			return
		}
		if b.removeIgnore(m.Args[1]) {
			b.log.Info("admin unignore", "by", m.Nick, "nick", m.Args[1])
			b.Say(m.Target, "Unignored "+m.Args[1]+".")
		} else {
			b.Say(m.Target, m.Args[1]+" is not ignored.")
		}
	})
}

func isChannel(s string) bool { return strings.HasPrefix(s, "#") }

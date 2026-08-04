package bot

import "strings"

func (b *Bot) registerAdminCommands() {
	admin := func(name string, h Handler) {
		b.Handle(Command{Name: name, Admin: true, Handler: h})
	}

	// say #chan hello   -> into #chan
	// say hello         -> into the channel the command came from
	admin("say", func(m *Message) {
		if !m.HasArgs() {
			return
		}
		if isChannel(m.Arg(1)) {
			b.Say(m.Arg(1), m.Rest(2))
		} else {
			b.Say(m.Target, m.Rest(1))
		}
	})

	// pm <target> hello
	admin("pm", func(m *Message) {
		if len(m.Args) > 2 {
			b.Say(m.Arg(1), m.Rest(2))
		}
	})

	// join #channel [key]
	admin("join", func(m *Message) {
		if m.HasArgs() && isChannel(m.Arg(1)) {
			b.log.Info("admin join", "by", m.Nick, "channel", m.Arg(1))
			b.conn.Join(m.Rest(1))
			b.addChannel(m.Arg(1))
		}
	})

	// part            -> the current channel
	// part #channel
	admin("part", func(m *Message) {
		target := m.Target
		if m.HasArgs() && isChannel(m.Arg(1)) {
			target = m.Arg(1)
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
			b.Notice(m.Arg(1), m.Rest(2))
		}
	})

	// me hello          -> action in the current channel
	// me #channel hello
	admin("me", func(m *Message) {
		if !m.HasArgs() {
			return
		}
		if isChannel(m.Arg(1)) {
			b.Action(m.Arg(1), m.Rest(2))
		} else {
			b.Action(m.Target, m.Rest(1))
		}
	})

	// kick <user> [reason]
	// kick #channel <user> [reason]
	admin("kick", func(m *Message) {
		if !m.HasArgs() {
			return
		}
		if isChannel(m.Arg(1)) {
			if len(m.Args) > 2 {
				b.log.Info("admin kick", "by", m.Nick, "channel", m.Arg(1), "target", m.Arg(2))
				b.conn.Kick(m.Arg(2), m.Arg(1), m.Rest(3))
			}
		} else {
			b.log.Info("admin kick", "by", m.Nick, "channel", m.Target, "target", m.Arg(1))
			b.conn.Kick(m.Arg(1), m.Target, m.Rest(2))
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
			reason = m.Rest(2)
		}
		b.addIgnore(m.Arg(1), reason)
		b.log.Info("admin ignore", "by", m.Nick, "nick", m.Arg(1), "reason", reason)
		b.Say(m.Target, "Added "+m.Arg(1)+" to ignore list. Reason: "+reason)
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
		if b.removeIgnore(m.Arg(1)) {
			b.log.Info("admin unignore", "by", m.Nick, "nick", m.Arg(1))
			b.Say(m.Target, "Unignored "+m.Arg(1)+".")
		} else {
			b.Say(m.Target, m.Arg(1)+" is not ignored.")
		}
	})
}

func isChannel(s string) bool { return strings.HasPrefix(s, "#") }

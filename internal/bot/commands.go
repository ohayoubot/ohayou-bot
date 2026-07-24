package bot

func (b *Bot) registerBotCommands() {
	cmd := func(name string, h Handler) {
		b.Handle(Command{Name: name, Admin: false, Handler: h})
	}

	cmd("code", func(m *Message) {
		b.Say(m.Target, "Repository: https://github.com/ohayoubot/ohayou-bot")
	})
}

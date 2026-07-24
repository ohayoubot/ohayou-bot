// Package catfact is a small plugin that replies with a random cat fact.
package catfact

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
)

const api = "https://catfact.ninja/fact"

type Plugin struct {
	bot    *bot.Bot
	client *http.Client
}

func New(b *bot.Bot) *Plugin {
	return &Plugin{
		bot:    b,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *Plugin) Register() {
	p.bot.HandleFunc("cat", false, p.handle)
	p.bot.HandleFunc("catfact", false, p.handle)
}

func (p *Plugin) handle(m *bot.Message) {
	p.bot.Say(m.Target, p.fact())
}

func (p *Plugin) fact() string {
	resp, err := p.client.Get(api)
	if err != nil {
		return "Couldn't reach the cat fact API."
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("Couldn't reach the cat fact API: %s", resp.Status)
	}

	var cf struct {
		Fact string `json:"fact"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cf); err != nil || cf.Fact == "" {
		return "There was a problem fetching a cat fact."
	}
	return cf.Fact
}

package ohayou

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
	"github.com/ohayoubot/ohayou-bot/internal/plugins/ohayou/seed"
)

// Config is the game block. The game needs no credentials, only its data files,
// so it is on unless the operator says otherwise.
type Config struct {
	Enabled *bool `json:"enabled"`
	// DataDir holds items.json and fortunes.txt.
	DataDir string `json:"dataDir"`
	// Timezone is the calendar the daily ration runs on. A ration is once per
	// day somewhere, and this says where.
	Timezone string `json:"timezone"`
	// DatabaseID is the game's own D1 database, the one the projection is
	// published into. Set it to take requests from the website; without it the
	// game still publishes, it just does not listen. AccountID defaults to the
	// shared cloudflare block.
	AccountID  string `json:"accountId"`
	DatabaseID string `json:"databaseId"`
	// APIToken comes from the environment, never from a block.
	APIToken string `json:"-"`
	// RequestTimeoutMS bounds a single D1 request.
	RequestTimeoutMS int `json:"requestTimeout"`
}

// RequestTimeout bounds a single D1 request.
func (c Config) RequestTimeout() time.Duration { return config.MS(c.RequestTimeoutMS) }

// Configure reads the data files, so a game that cannot find its catalog says
// so at startup rather than answering every command with an error.
func (p *Plugin) Configure(pc plugin.Config) (bool, error) {
	c := Config{}
	if len(pc.Block) > 0 {
		if err := json.Unmarshal(pc.Block, &c); err != nil {
			return false, fmt.Errorf("game config: %w", err)
		}
	}
	if !config.On(c.Enabled, true) {
		return false, nil
	}

	if c.AccountID == "" {
		c.AccountID = pc.Cloudflare.AccountID
	}
	if c.APIToken == "" {
		c.APIToken = pc.Cloudflare.APIToken
	}
	if c.RequestTimeoutMS == 0 {
		c.RequestTimeoutMS = 10000
	}
	if c.DataDir == "" {
		c.DataDir = "data"
	}
	if c.Timezone == "" {
		c.Timezone = "America/New_York"
	}
	// Checked here rather than only at Register, so a name the zone database
	// does not have is caught by -check instead of at startup.
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return false, fmt.Errorf("timezone %q: %w", c.Timezone, err)
	}

	items, err := seed.LoadItems(filepath.Join(c.DataDir, "items.json"))
	if err != nil {
		return false, err
	}
	fortunes, err := seed.LoadFortunes(filepath.Join(c.DataDir, "fortunes.txt"))
	if err != nil {
		return false, err
	}

	p.cfg, p.items, p.fortunes = c, items, fortunes
	return true, nil
}

package ohayou

import (
	"encoding/json"
	"fmt"
	"path/filepath"

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
}

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

	if c.DataDir == "" {
		c.DataDir = "data"
	}
	if c.Timezone == "" {
		c.Timezone = "America/New_York"
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

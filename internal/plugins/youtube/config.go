package youtube

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
)

// Config is the youtube block. The plugin reads youtube's public oembed
// endpoint, so unlike the others it needs no account, key or quota, and is
// therefore on unless the operator says otherwise.
type Config struct {
	Enabled *bool `json:"enabled"`
	// MaxLinks caps how many videos one message may name, so a paste of a
	// playlist's worth of links costs a channel a line or two and no more.
	MaxLinks int `json:"maxLinks"`
	// Cooldown is the seconds a channel waits between previews.
	Cooldown int `json:"cooldown"`
	// Repeat is the seconds before the same video is worth naming again in the
	// same channel.
	Repeat int `json:"repeat"`
	// RequestTimeoutMS bounds a single oembed request.
	RequestTimeoutMS int      `json:"requestTimeout"`
	IgnoreChannels   []string `json:"ignoreChannels"`
}

func (c Config) CooldownWait() time.Duration   { return config.Secs(c.Cooldown) }
func (c Config) RepeatWait() time.Duration     { return config.Secs(c.Repeat) }
func (c Config) RequestTimeout() time.Duration { return config.MS(c.RequestTimeoutMS) }

// Configure fills in the defaults. There is nothing to authenticate, so the
// only way to get this wrong is a nonsense number.
func (p *Plugin) Configure(pc plugin.Config) (bool, error) {
	c := Config{}
	if len(pc.Block) > 0 {
		if err := json.Unmarshal(pc.Block, &c); err != nil {
			return false, fmt.Errorf("youtube config: %w", err)
		}
	}
	if !config.On(c.Enabled, true) {
		return false, nil
	}

	if c.MaxLinks == 0 {
		c.MaxLinks = 2
	}
	if c.Cooldown == 0 {
		c.Cooldown = 10
	}
	if c.Repeat == 0 {
		c.Repeat = 600
	}
	if c.RequestTimeoutMS == 0 {
		c.RequestTimeoutMS = 8000
	}

	if c.MaxLinks < 1 {
		return false, fmt.Errorf("maxLinks must be at least 1")
	}
	if c.Cooldown < 0 || c.Repeat < 0 || c.RequestTimeoutMS < 1 {
		return false, fmt.Errorf("cooldown, repeat and requestTimeout must be positive")
	}

	p.cfg = c
	return true, nil
}

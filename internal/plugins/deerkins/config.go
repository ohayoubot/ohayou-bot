package deerkins

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
)

// Config is the deerkins block. The plugin reads pixel art out of a cloudflare
// D1 database over the HTTP API and paints it into a channel, so it stays off
// until it has somewhere to read from.
type Config struct {
	Enabled *bool `json:"enabled"`
	// AccountID, DatabaseID and APIToken default to the shared cloudflare block.
	AccountID  string `json:"accountId"`
	DatabaseID string `json:"databaseId"`
	APIToken   string `json:"apiToken"`
	// Editor is the url of the drawing app, quoted in help and 404s.
	Editor string `json:"editor"`
	// Timeout is the seconds a channel must wait between deer.
	Timeout int `json:"timeout"`
	// TimeoutPunish multiplies Timeout when the same nick or the same deer
	// comes up twice in a row.
	TimeoutPunish float64 `json:"timeoutPunish"`
	// MissTimeout is the (shorter) seconds to wait after a lookup that found
	// nothing, so typos don't cost a channel the full Timeout.
	MissTimeout int `json:"missTimeout"`
	// MaxLines caps how many lines one deer may paint.
	MaxLines int `json:"maxLines"`
	// RequestTimeoutMS bounds a single D1 request.
	RequestTimeoutMS int `json:"requestTimeout"`
	// PrivilegedMatch is which of "nick" and "host" must match for Privileged
	// to apply. Both are required when both are listed.
	PrivilegedMatch []string        `json:"privilegedMatch"`
	Privileged      map[string]User `json:"privileged"`
	IgnoreNicks     []string        `json:"ignoreNicks"`
	IgnoreHosts     []string        `json:"ignoreHosts"`
	IgnoreChannels  []string        `json:"ignoreChannels"`
}

// User is a nick that deers on easier terms than everyone else.
type User struct {
	Host string `json:"host"`
	// Timeout overrides the channel timeout in seconds when set. Zero or
	// negative means no wait at all.
	Timeout *int `json:"timeout"`
}

// Wait is the normal spacing between deer in a channel.
func (c Config) Wait() time.Duration { return config.Secs(c.Timeout) }

// MissWait is the spacing after a deer that could not be fetched.
func (c Config) MissWait() time.Duration { return config.Secs(c.MissTimeout) }

// RequestTimeout bounds a single D1 request.
func (c Config) RequestTimeout() time.Duration { return config.MS(c.RequestTimeoutMS) }

// MatchNick and MatchHost report which fields privileged entries are keyed on.
func (c Config) MatchNick() bool { return c.matches("nick") }
func (c Config) MatchHost() bool { return c.matches("host") }

func (c Config) matches(field string) bool {
	for _, f := range c.PrivilegedMatch {
		if strings.EqualFold(strings.TrimSpace(f), field) {
			return true
		}
	}
	return false
}

// Configure borrows what the shared cloudflare block knows and fills in the
// defaults.
func (p *Plugin) Configure(pc plugin.Config) (bool, error) {
	c := Config{}
	if len(pc.Block) > 0 {
		if err := json.Unmarshal(pc.Block, &c); err != nil {
			return false, fmt.Errorf("deerkins config: %w", err)
		}
	}

	if c.AccountID == "" {
		c.AccountID = pc.Cloudflare.AccountID
	}
	if c.DatabaseID == "" {
		c.DatabaseID = pc.Cloudflare.DatabaseID
	}
	if c.APIToken == "" {
		c.APIToken = pc.Cloudflare.APIToken
	}

	if !config.On(c.Enabled, c.AccountID != "" && c.DatabaseID != "" && c.APIToken != "") {
		return false, nil
	}
	switch {
	case c.AccountID == "":
		return false, fmt.Errorf("accountId is required")
	case c.DatabaseID == "":
		return false, fmt.Errorf("databaseId is required")
	case c.APIToken == "":
		return false, fmt.Errorf("apiToken (or OHAYOU_CF_API_TOKEN) is required")
	}

	if c.Editor == "" {
		c.Editor = "https://hemera.day/deerkins/"
	}
	if c.Timeout == 0 {
		c.Timeout = 300
	}
	if c.TimeoutPunish == 0 {
		c.TimeoutPunish = 1.7
	}
	if c.MissTimeout == 0 {
		c.MissTimeout = 15
	}
	if c.MaxLines == 0 {
		c.MaxLines = 30
	}
	if c.RequestTimeoutMS == 0 {
		c.RequestTimeoutMS = 10000
	}
	if len(c.PrivilegedMatch) == 0 {
		c.PrivilegedMatch = []string{"nick", "host"}
	}

	if c.TimeoutPunish < 1 {
		return false, fmt.Errorf("timeoutPunish must be at least 1")
	}
	if c.Timeout < 0 || c.MissTimeout < 0 || c.MaxLines < 1 || c.RequestTimeoutMS < 1 {
		return false, fmt.Errorf("timeout, missTimeout, maxLines and requestTimeout must be positive")
	}

	privileged := make(map[string]User, len(c.Privileged))
	for nick, user := range c.Privileged {
		privileged[strings.ToLower(nick)] = user
	}
	c.Privileged = privileged

	p.cfg = c
	return true, nil
}

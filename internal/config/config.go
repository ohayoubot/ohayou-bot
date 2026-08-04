// Package config loads the bot's json config file.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Nick          string            `json:"nick"`
	User          string            `json:"user"`
	NickPW        string            `json:"nickPw"`
	Server        string            `json:"server"`
	Port          int               `json:"port"`
	TLS           bool              `json:"tls"`
	SASL          SASLConfig        `json:"sasl"`
	VHost         VHostConfig       `json:"vhost"`
	Channels      []string          `json:"channels"`
	Debug         bool              `json:"debug"`
	Verbose       bool              `json:"verbose"`
	FloodProtect  bool              `json:"floodProtect"`
	FloodDelayMS  int               `json:"floodDelay"` // minimum ms between outbound messages
	Admins        map[string]string `json:"admins"`
	CommandPrefix string            `json:"commandPrefix"`
	IgnoreList    map[string]string `json:"ignoreList"`
	Database      string            `json:"database"` // path to the sqlite .db file
	Cloudflare    Cloudflare        `json:"cloudflare"`
	// Plugins holds each plugin's block unparsed. The bot does not know what is
	// in them; the plugin that owns one reads it.
	Plugins map[string]json.RawMessage `json:"plugins"`
}

// Cloudflare is the D1 database the plugins that need one share. A plugin may
// override any of it in its own block once it wants a database of its own.
type Cloudflare struct {
	AccountID  string `json:"accountId"`
	DatabaseID string `json:"databaseId"`
	// APIToken needs the D1:Read permission on the account. OHAYOU_CF_API_TOKEN
	// overrides it so the token need not be written to disk.
	APIToken string `json:"apiToken"`
}

// On resolves an optional toggle against what it defaults to when unset.
func On(flag *bool, byDefault bool) bool {
	if flag != nil {
		return *flag
	}
	return byDefault
}

// Secs and MS turn the integers a config file carries into durations.
func Secs(n int) time.Duration { return time.Duration(n) * time.Second }
func MS(n int) time.Duration   { return time.Duration(n) * time.Millisecond }

type SASLConfig struct {
	Enabled   *bool  `json:"enabled"`
	Login     string `json:"login"`
	Password  string `json:"password"`
	Mechanism string `json:"mechanism"` // PLAIN (default) or EXTERNAL
}

// Configured returns whether SASL looks configured: PLAIN needs a login and
// password, EXTERNAL needs only the mechanism
func (s SASLConfig) Configured() bool {
	if strings.EqualFold(s.Mechanism, "EXTERNAL") {
		return true
	}
	return s.Login != "" && s.Password != ""
}

// Use returns whether SASL should be used. An explicit Enabled wins; otherwise
// SASL is used by default whenever it is configured.
func (s SASLConfig) Use() bool {
	if s.Enabled != nil {
		return *s.Enabled
	}
	return s.Configured()
}

// Mech returns the SASL mechanism. default PLAIN.
func (s SASLConfig) Mech() string {
	if s.Mechanism == "" {
		return "PLAIN"
	}
	return strings.ToUpper(s.Mechanism)
}

type VHostConfig struct {
	Enabled *bool `json:"enabled"`
	// Service is the HostServ-style service to message (default "HostServ").
	Service string `json:"service"`
	// Command activates the vhost (default "ON"). Sent after login, before join.
	Command string `json:"command"`
	// TimeoutMS bounds how long to wait for the 396 host-hidden confirmation
	// before giving up (default 10000). The bot won't join without it.
	TimeoutMS int `json:"timeout"`
}

// Use returns whether the vhost gate should run.
func (v VHostConfig) Use(server string) bool {
	if v.Enabled != nil {
		return *v.Enabled
	}
	return strings.Contains(strings.ToLower(server), "rizon")
}

// VHostEnabled returns whether the vhost gate should run for this config.
func (c *Config) VHostEnabled() bool { return c.VHost.Use(c.Server) }

// VHostTimeout is how long to wait for the 396 host-hidden confirmation.
func (c *Config) VHostTimeout() time.Duration {
	return time.Duration(c.VHost.TimeoutMS) * time.Millisecond
}

// FloodDelay returns the configured minimum spacing between messages, or zero
// when flood protection is disabled.
func (c *Config) FloodDelay() time.Duration {
	if !c.FloodProtect {
		return 0
	}
	return time.Duration(c.FloodDelayMS) * time.Millisecond
}

// Load reads and validates the configuration at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Nick == "" {
		return nil, fmt.Errorf("config: nick is required")
	}
	if cfg.Server == "" {
		return nil, fmt.Errorf("config: server is required")
	}
	if cfg.User == "" {
		cfg.User = cfg.Nick
	}
	if cfg.Port == 0 {
		if cfg.TLS {
			cfg.Port = 6697
		} else {
			cfg.Port = 6667
		}
	}
	if cfg.SASL.Use() {
		switch cfg.SASL.Mech() {
		case "PLAIN":
			if cfg.SASL.Login == "" || cfg.SASL.Password == "" {
				return nil, fmt.Errorf("config: sasl PLAIN requires login and password")
			}
		case "EXTERNAL":
			// Certificate-based; credentials come from the TLS client cert.
		default:
			return nil, fmt.Errorf("config: sasl mechanism %q unsupported (want PLAIN or EXTERNAL)", cfg.SASL.Mechanism)
		}
	}
	if cfg.VHostEnabled() {
		if cfg.VHost.Service == "" {
			cfg.VHost.Service = "HostServ"
		}
		if cfg.VHost.Command == "" {
			cfg.VHost.Command = "ON"
		}
		if cfg.VHost.TimeoutMS == 0 {
			cfg.VHost.TimeoutMS = 10000
		}
	}
	if cfg.Database == "" {
		cfg.Database = "ohayoubot.db"
	}
	if token := os.Getenv("OHAYOU_CF_API_TOKEN"); token != "" {
		cfg.Cloudflare.APIToken = token
	}
	// Normalize nicks to lower case for admins
	admins := make(map[string]string, len(cfg.Admins))
	for nick, host := range cfg.Admins {
		admins[strings.ToLower(nick)] = host
	}
	cfg.Admins = admins
	if cfg.IgnoreList == nil {
		cfg.IgnoreList = map[string]string{}
	}
	return cfg, nil
}

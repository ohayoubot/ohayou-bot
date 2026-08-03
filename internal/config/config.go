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
	Deerkins      DeerkinsConfig    `json:"deerkins"`
	Drop          DropConfig        `json:"drop"`
}

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

// DeerkinsConfig configures the deerkins plugin, which reads pixel art out of
// a cloudflare D1 database over the HTTP API and paints it into a channel.
type DeerkinsConfig struct {
	Enabled    *bool  `json:"enabled"`
	AccountID  string `json:"accountId"`
	DatabaseID string `json:"databaseId"`
	// APIToken needs the D1:Read permission on the account. The
	// DEERKINS_API_TOKEN environment variable overrides it so the token need
	// not be written to disk.
	APIToken string `json:"apiToken"`
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
	PrivilegedMatch []string                `json:"privilegedMatch"`
	Privileged      map[string]DeerkinsUser `json:"privileged"`
	IgnoreNicks     []string                `json:"ignoreNicks"`
	IgnoreHosts     []string                `json:"ignoreHosts"`
	IgnoreChannels  []string                `json:"ignoreChannels"`
}

// DropConfig configures the drop plugin, which hands a user a signed link to
// the upload site and announces what they upload.
type DropConfig struct {
	Enabled *bool `json:"enabled"`
	// AccountID and DatabaseID default to the deerkins block, whose database
	// currently holds the upload tables too. Set them to split the two apart.
	AccountID  string `json:"accountId"`
	DatabaseID string `json:"databaseId"`
	// APIToken needs D1:Read and nothing more: the worker makes every write.
	// OHAYOU_DROP_TOKEN overrides it, and it falls back to the deerkins token
	// while both plugins read one database.
	APIToken string `json:"apiToken"`
	// Secret signs the grant links. It has no config field on purpose, so it
	// cannot be committed by accident: it comes from OHAYOU_DROP_SECRET only.
	// The worker holds the same value as DROP_HMAC_SECRET, and both sides key
	// on its utf-8 bytes rather than decoding the hex.
	Secret string `json:"-"`
	// URL is the upload site, e.g. "https://hemera.day/drop/".
	URL string `json:"url"`
	// GrantTTL is how many seconds a link stays good for. The worker refuses
	// anything reaching more than maxGrantTTL ahead, so this cannot exceed it.
	GrantTTL int `json:"grantTtl"`
	// PollSeconds is how often to ask D1 for uploads to announce.
	PollSeconds int `json:"poll"`
	// Cooldown is the seconds a nick waits between links.
	Cooldown int `json:"cooldown"`
	// RequestTimeoutMS bounds a single D1 request.
	RequestTimeoutMS int `json:"requestTimeout"`
}

// maxGrantTTL matches the ceiling the worker enforces when it verifies a grant.
const maxGrantTTL = 900

// Configured returns whether the operator asked for the plugin at all. The
// database it reads is not part of the question: missing that is an error from
// loadDrop, not a plugin that quietly never answers.
func (d DropConfig) Configured() bool {
	return d.Secret != "" && d.URL != ""
}

// Use returns whether the drop plugin should be registered. An explicit Enabled
// wins; otherwise it comes up whenever it is configured.
func (d DropConfig) Use() bool {
	if d.Enabled != nil {
		return *d.Enabled
	}
	return d.Configured()
}

// GrantWait is how long a minted link stays valid.
func (d DropConfig) GrantWait() time.Duration {
	return time.Duration(d.GrantTTL) * time.Second
}

// PollWait is the gap between checks for new uploads.
func (d DropConfig) PollWait() time.Duration {
	return time.Duration(d.PollSeconds) * time.Second
}

// CooldownWait is the gap a nick must leave between links.
func (d DropConfig) CooldownWait() time.Duration {
	return time.Duration(d.Cooldown) * time.Second
}

// RequestTimeout bounds a single D1 request.
func (d DropConfig) RequestTimeout() time.Duration {
	return time.Duration(d.RequestTimeoutMS) * time.Millisecond
}

// Link is the url to send a user, with the grant in the fragment. A fragment
// never reaches the server's logs.
func (d DropConfig) Link(grant string) string {
	return strings.TrimSuffix(d.URL, "#") + "#" + grant
}

// DeerkinsUser is a nick that deers on easier terms than everyone else.
type DeerkinsUser struct {
	Host string `json:"host"`
	// Timeout overrides the channel timeout in seconds when set. Zero or
	// negative means no wait at all.
	Timeout *int `json:"timeout"`
}

// Configured returns whether the plugin has everything it needs to reach D1.
func (d DeerkinsConfig) Configured() bool {
	return d.AccountID != "" && d.DatabaseID != "" && d.APIToken != ""
}

// Use returns whether the deerkins plugin should be registered. An explicit
// Enabled wins; otherwise it comes up whenever it is configured.
func (d DeerkinsConfig) Use() bool {
	if d.Enabled != nil {
		return *d.Enabled
	}
	return d.Configured()
}

// Wait is the normal spacing between deer in a channel.
func (d DeerkinsConfig) Wait() time.Duration {
	return time.Duration(d.Timeout) * time.Second
}

// MissWait is the spacing after a deer that could not be fetched.
func (d DeerkinsConfig) MissWait() time.Duration {
	return time.Duration(d.MissTimeout) * time.Second
}

// RequestTimeout bounds a single D1 request.
func (d DeerkinsConfig) RequestTimeout() time.Duration {
	return time.Duration(d.RequestTimeoutMS) * time.Millisecond
}

// MatchNick and MatchHost report which fields privileged entries are keyed on.
func (d DeerkinsConfig) MatchNick() bool { return d.matches("nick") }
func (d DeerkinsConfig) MatchHost() bool { return d.matches("host") }

func (d DeerkinsConfig) matches(field string) bool {
	for _, f := range d.PrivilegedMatch {
		if strings.EqualFold(strings.TrimSpace(f), field) {
			return true
		}
	}
	return false
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
	if err := cfg.loadDeerkins(); err != nil {
		return nil, err
	}
	if err := cfg.loadDrop(); err != nil {
		return nil, err
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

// loadDeerkins applies the environment override and fills in the defaults. A
// half-configured plugin is an error rather than a plugin that quietly never
// answers.
func (c *Config) loadDeerkins() error {
	d := &c.Deerkins
	if token := os.Getenv("DEERKINS_API_TOKEN"); token != "" {
		d.APIToken = token
	}
	if !d.Use() {
		return nil
	}
	switch {
	case d.AccountID == "":
		return fmt.Errorf("config: deerkins accountId is required")
	case d.DatabaseID == "":
		return fmt.Errorf("config: deerkins databaseId is required")
	case d.APIToken == "":
		return fmt.Errorf("config: deerkins apiToken (or DEERKINS_API_TOKEN) is required")
	}
	if d.Editor == "" {
		d.Editor = "https://hemera.day/deerkins/"
	}
	if d.Timeout == 0 {
		d.Timeout = 300
	}
	if d.TimeoutPunish == 0 {
		d.TimeoutPunish = 1.7
	}
	if d.MissTimeout == 0 {
		d.MissTimeout = 15
	}
	if d.MaxLines == 0 {
		d.MaxLines = 30
	}
	if d.RequestTimeoutMS == 0 {
		d.RequestTimeoutMS = 10000
	}
	if len(d.PrivilegedMatch) == 0 {
		d.PrivilegedMatch = []string{"nick", "host"}
	}
	if d.TimeoutPunish < 1 {
		return fmt.Errorf("config: deerkins timeoutPunish must be at least 1")
	}
	if d.Timeout < 0 || d.MissTimeout < 0 || d.MaxLines < 1 || d.RequestTimeoutMS < 1 {
		return fmt.Errorf("config: deerkins timeout, missTimeout, maxLines and requestTimeout must be positive")
	}
	privileged := make(map[string]DeerkinsUser, len(d.Privileged))
	for nick, user := range d.Privileged {
		privileged[strings.ToLower(nick)] = user
	}
	d.Privileged = privileged
	return nil
}

// loadDrop applies the environment, borrows what the deerkins block already
// knows about the database, and fills in the defaults. As with deerkins, a
// half-configured plugin is an error rather than one that quietly never
// answers: a nick asking for a link and getting silence is worse than a
// refusal at startup.
func (c *Config) loadDrop() error {
	d := &c.Drop
	d.Secret = os.Getenv("OHAYOU_DROP_SECRET")
	if token := os.Getenv("OHAYOU_DROP_TOKEN"); token != "" {
		d.APIToken = token
	}

	// One database for now, so an unset field means "wherever deerkins reads".
	if d.AccountID == "" {
		d.AccountID = c.Deerkins.AccountID
	}
	if d.DatabaseID == "" {
		d.DatabaseID = c.Deerkins.DatabaseID
	}
	if d.APIToken == "" {
		d.APIToken = c.Deerkins.APIToken
	}

	if !d.Use() {
		return nil
	}
	switch {
	case d.Secret == "":
		return fmt.Errorf("config: drop needs OHAYOU_DROP_SECRET")
	case d.URL == "":
		return fmt.Errorf("config: drop url is required")
	case d.AccountID == "":
		return fmt.Errorf("config: drop accountId is required")
	case d.DatabaseID == "":
		return fmt.Errorf("config: drop databaseId is required")
	case d.APIToken == "":
		return fmt.Errorf("config: drop apiToken (or OHAYOU_DROP_TOKEN) is required")
	}

	if d.GrantTTL == 0 {
		d.GrantTTL = 300
	}
	if d.PollSeconds == 0 {
		d.PollSeconds = 10
	}
	if d.Cooldown == 0 {
		d.Cooldown = 60
	}
	if d.RequestTimeoutMS == 0 {
		d.RequestTimeoutMS = 10000
	}

	if d.GrantTTL < 30 || d.GrantTTL > maxGrantTTL {
		return fmt.Errorf("config: drop grantTtl must be between 30 and %d seconds", maxGrantTTL)
	}
	// Polling faster than this buys a second of latency and spends the D1 read
	// budget on empty answers.
	if d.PollSeconds < 5 {
		return fmt.Errorf("config: drop poll must be at least 5 seconds")
	}
	if d.Cooldown < 0 || d.RequestTimeoutMS < 1 {
		return fmt.Errorf("config: drop cooldown and requestTimeout must be positive")
	}
	return nil
}

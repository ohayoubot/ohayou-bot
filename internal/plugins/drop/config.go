package drop

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
	"github.com/ohayoubot/ohayou-bot/internal/web"
)

// Config is the drop block. The plugin hands a user a signed link to the upload
// site and announces what they upload.
type Config struct {
	Enabled *bool `json:"enabled"`
	// AccountID and DatabaseID default to the shared cloudflare block.
	AccountID  string `json:"accountId"`
	DatabaseID string `json:"databaseId"`
	// APIToken comes from the environment, never from a block.
	APIToken string `json:"-"`
	// URL is the upload site, e.g. "https://hemera.day/drop/".
	URL string `json:"url"`
	// ImageBase is where the bucket is served, e.g. "https://img.hemera.day".
	// It must match the worker's PUBLIC_IMAGE_BASE: the two are the same value
	// held in two places, and disagreeing means announcing dead links.
	ImageBase string `json:"imageBase"`
	// GrantTTL is how many seconds a link stays good for. The worker refuses
	// anything reaching further ahead than web.MaxTTL, so this cannot exceed it.
	GrantTTL int `json:"grantTtl"`
	// PollSeconds is how often to ask D1 for uploads to announce.
	PollSeconds int `json:"poll"`
	// Cooldown is the seconds a nick waits between links.
	Cooldown int `json:"cooldown"`
	// RequestTimeoutMS bounds a single D1 request.
	RequestTimeoutMS int `json:"requestTimeout"`
}

// GrantWait is how long a minted link stays valid.
func (c Config) GrantWait() time.Duration { return config.Secs(c.GrantTTL) }

// PollWait is the gap between checks for new uploads.
func (c Config) PollWait() time.Duration { return config.Secs(c.PollSeconds) }

// CooldownWait is the gap a nick must leave between links.
func (c Config) CooldownWait() time.Duration { return config.Secs(c.Cooldown) }

// RequestTimeout bounds a single D1 request.
func (c Config) RequestTimeout() time.Duration { return config.MS(c.RequestTimeoutMS) }

// Link is the url to send a user, with the grant in the fragment. A fragment
// never reaches the server's logs.
func (c Config) Link(grant string) string {
	return strings.TrimSuffix(c.URL, "#") + "#" + grant
}

// Image is where an uploaded object is served from.
func (c Config) Image(key string) string {
	return strings.TrimRight(c.ImageBase, "/") + "/" + key
}

// Configure borrows the shared cloudflare block and the site's secret, and
// fills in the defaults.
func (p *Plugin) Configure(pc plugin.Config) (bool, error) {
	c := Config{}
	if len(pc.Block) > 0 {
		if err := json.Unmarshal(pc.Block, &c); err != nil {
			return false, fmt.Errorf("drop config: %w", err)
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

	// The database it reads is not part of the question: missing that is an
	// error below, not a plugin that quietly never answers.
	if !config.On(c.Enabled, pc.Web.Secret != "" && c.URL != "") {
		return false, nil
	}
	switch {
	case pc.Web.Secret == "":
		return false, fmt.Errorf("needs OHAYOU_WEB_SECRET")
	case c.URL == "":
		return false, fmt.Errorf("url is required")
	case c.ImageBase == "":
		return false, fmt.Errorf("imageBase is required")
	case c.AccountID == "":
		return false, fmt.Errorf("accountId is required")
	case c.DatabaseID == "":
		return false, fmt.Errorf("databaseId is required")
	case c.APIToken == "":
		return false, fmt.Errorf("OHAYOU_CF_API_TOKEN is required")
	}

	if c.GrantTTL == 0 {
		c.GrantTTL = 300
	}
	if c.PollSeconds == 0 {
		c.PollSeconds = 10
	}
	if c.Cooldown == 0 {
		c.Cooldown = 60
	}
	if c.RequestTimeoutMS == 0 {
		c.RequestTimeoutMS = 10000
	}

	if c.GrantTTL < 30 || config.Secs(c.GrantTTL) > web.MaxTTL {
		return false, fmt.Errorf("grantTtl must be between 30 seconds and %s", web.MaxTTL)
	}
	// Polling faster than this buys a second of latency and spends the D1 read
	// budget on empty answers.
	if c.PollSeconds < 5 {
		return false, fmt.Errorf("poll must be at least 5 seconds")
	}
	if c.Cooldown < 0 || c.RequestTimeoutMS < 1 {
		return false, fmt.Errorf("cooldown and requestTimeout must be positive")
	}

	p.cfg = c
	return true, nil
}

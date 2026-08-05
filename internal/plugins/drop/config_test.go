package drop

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
	"github.com/ohayoubot/ohayou-bot/internal/web"
)

var creds = config.Cloudflare{AccountID: "acct", DatabaseID: "db", APIToken: "token"}

const sited = `{"url": "https://hemera.day/drop/", "imageBase": "https://img.hemera.day"}`

// configure runs the plugin's own configuration with the site's secret
// present, which is the state it is normally asked in.
func configure(t *testing.T, block string, cf config.Cloudflare) (*Plugin, bool, error) {
	t.Helper()
	return configureWeb(t, block, cf, config.Web{Secret: "s"})
}

func configureWeb(t *testing.T, block string, cf config.Cloudflare, w config.Web) (*Plugin, bool, error) {
	t.Helper()
	p := New()
	var raw json.RawMessage
	if block != "" {
		raw = json.RawMessage(block)
	}
	on, err := p.Configure(plugin.Config{Block: raw, Cloudflare: cf, Web: w})
	return p, on, err
}

func TestOffWithoutASecret(t *testing.T) {
	_, on, err := configureWeb(t, sited, creds, config.Web{})
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Error("drop came up with no signing secret")
	}
}

func TestOffWithoutAURL(t *testing.T) {
	_, on, err := configure(t, "", creds)
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Error("drop came up with nowhere to send anyone")
	}
}

// The signing secret belongs to the site, not to this plugin. A secret written
// into drop's own block is not one, and cannot arm it.
func TestSecretInThePluginBlockIsNotASecret(t *testing.T) {
	_, on, err := configureWeb(t,
		`{"url": "https://x/", "imageBase": "https://i", "secret": "in-the-file"}`,
		creds, config.Web{})
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Error("a secret in the plugin block armed drop")
	}
}

func TestDefaults(t *testing.T) {
	p, on, err := configure(t, sited, creds)
	if err != nil || !on {
		t.Fatalf("configure: on=%v err=%v", on, err)
	}
	c := p.cfg

	if c.GrantWait() != 300*time.Second {
		t.Errorf("GrantWait = %v, want 5m", c.GrantWait())
	}
	if c.PollWait() != 10*time.Second {
		t.Errorf("PollWait = %v, want 10s", c.PollWait())
	}
	if c.CooldownWait() != 60*time.Second {
		t.Errorf("CooldownWait = %v, want 1m", c.CooldownWait())
	}
	if c.RequestTimeout() != 10*time.Second {
		t.Errorf("RequestTimeout = %v, want 10s", c.RequestTimeout())
	}
}

func TestBorrowsTheSharedCloudflareBlock(t *testing.T) {
	p, _, err := configure(t, sited, creds)
	if err != nil {
		t.Fatal(err)
	}
	if p.cfg.AccountID != "acct" || p.cfg.DatabaseID != "db" || p.cfg.APIToken != "token" {
		t.Errorf("cfg = %+v, want the shared block borrowed", p.cfg)
	}
}

func TestMissingPiecesAreRefused(t *testing.T) {
	for _, tc := range []struct {
		block, want string
		cf          config.Cloudflare
	}{
		{`{"url": "https://x/"}`, "imageBase", creds},
		{sited, "accountId", config.Cloudflare{}},
		{sited, "databaseId", config.Cloudflare{AccountID: "acct"}},
		{sited, "apiToken", config.Cloudflare{AccountID: "acct", DatabaseID: "db"}},
	} {
		_, _, err := configure(t, tc.block, tc.cf)
		if err == nil {
			t.Errorf("configure accepted %s with %+v", tc.block, tc.cf)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("err = %v, want it to name %q", err, tc.want)
		}
	}
}

// The worker refuses a grant reaching further ahead than web.MaxTTL, so a
// config asking for one would mint links that are dead on arrival.
func TestGrantTTLIsBounded(t *testing.T) {
	for _, ttl := range []int{29, int(web.MaxTTL/time.Second) + 1} {
		block := `{"url": "https://x/", "imageBase": "https://i", "grantTtl": ` +
			strconv.Itoa(ttl) + `}`
		if _, _, err := configure(t, block, creds); err == nil {
			t.Errorf("configure accepted grantTtl %d", ttl)
		}
	}
}

func TestPollFloor(t *testing.T) {
	if _, _, err := configure(t, `{"url": "https://x/", "imageBase": "https://i", "poll": 1}`, creds); err == nil {
		t.Error("configure accepted a poll faster than the floor")
	}
}

func TestLinkPutsTheGrantInTheFragment(t *testing.T) {
	for _, url := range []string{"https://hemera.day/drop/", "https://hemera.day/drop/#"} {
		c := Config{URL: url}
		if got := c.Link("v1.a.b"); got != "https://hemera.day/drop/#v1.a.b" {
			t.Errorf("Link(%q) = %q", url, got)
		}
	}
}

func TestImageJoinsBaseAndKeyOnce(t *testing.T) {
	for _, base := range []string{"https://img.hemera.day", "https://img.hemera.day/"} {
		c := Config{ImageBase: base}
		if got := c.Image("abc.png"); got != "https://img.hemera.day/abc.png" {
			t.Errorf("Image(%q) = %q", base, got)
		}
	}
}

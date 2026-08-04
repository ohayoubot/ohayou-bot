package deerkins

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
)

var creds = config.Cloudflare{AccountID: "acct", DatabaseID: "db", APIToken: "token"}

// configure runs Configure over a block written the way an operator would.
func configure(t *testing.T, block string, cf config.Cloudflare) (*Plugin, bool, error) {
	t.Helper()
	p := New()
	var raw json.RawMessage
	if block != "" {
		raw = json.RawMessage(block)
	}
	on, err := p.Configure(plugin.Config{Block: raw, Cloudflare: cf})
	return p, on, err
}

func TestOffWithoutADatabase(t *testing.T) {
	_, on, err := configure(t, "", config.Cloudflare{})
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Error("deerkins came up without a database")
	}
}

func TestOnFromTheSharedCloudflareBlock(t *testing.T) {
	p, on, err := configure(t, "", creds)
	if err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("deerkins did not come up from the shared credentials")
	}
	if p.cfg.AccountID != "acct" || p.cfg.DatabaseID != "db" || p.cfg.APIToken != "token" {
		t.Errorf("cfg = %+v, want the shared block borrowed", p.cfg)
	}
}

func TestOwnBlockOverridesTheSharedOne(t *testing.T) {
	p, _, err := configure(t, `{"databaseId": "its-own"}`, creds)
	if err != nil {
		t.Fatal(err)
	}
	if p.cfg.DatabaseID != "its-own" {
		t.Errorf("DatabaseID = %q, want the plugin's own to win", p.cfg.DatabaseID)
	}
	if p.cfg.AccountID != "acct" {
		t.Errorf("AccountID = %q, want the shared one still borrowed", p.cfg.AccountID)
	}
}

func TestDefaults(t *testing.T) {
	p, on, err := configure(t, "", creds)
	if err != nil || !on {
		t.Fatalf("configure: on=%v err=%v", on, err)
	}
	c := p.cfg

	if c.Wait() != 300*time.Second {
		t.Errorf("Wait = %v, want 5m", c.Wait())
	}
	if c.MissWait() != 15*time.Second {
		t.Errorf("MissWait = %v, want 15s", c.MissWait())
	}
	if c.RequestTimeout() != 10*time.Second {
		t.Errorf("RequestTimeout = %v, want 10s", c.RequestTimeout())
	}
	if c.TimeoutPunish != 1.7 {
		t.Errorf("TimeoutPunish = %v, want 1.7", c.TimeoutPunish)
	}
	if c.MaxLines != 30 {
		t.Errorf("MaxLines = %d, want 30", c.MaxLines)
	}
	if c.Editor == "" {
		t.Error("Editor has no default")
	}
	if !c.MatchNick() || !c.MatchHost() {
		t.Error("privilegedMatch defaults to something other than both fields")
	}
}

// Turning it on explicitly without credentials is a mistake worth refusing at
// startup, rather than a plugin that quietly never answers.
func TestEnabledWithoutCredentialsIsAnError(t *testing.T) {
	_, _, err := configure(t, `{"enabled": true}`, config.Cloudflare{})
	if err == nil {
		t.Fatal("configure accepted a plugin with nowhere to read from")
	}
	if !strings.Contains(err.Error(), "accountId") {
		t.Errorf("err = %v, want it to name the missing field", err)
	}
}

func TestDisabledEvenWithCredentials(t *testing.T) {
	_, on, err := configure(t, `{"enabled": false}`, creds)
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Error("deerkins came up despite being turned off")
	}
}

func TestNonsenseNumbersAreRefused(t *testing.T) {
	for _, block := range []string{
		`{"timeoutPunish": 0.5}`,
		`{"maxLines": -1}`,
		`{"requestTimeout": -1}`,
		`{"timeout": -1}`,
		`{"missTimeout": -1}`,
	} {
		if _, _, err := configure(t, block, creds); err == nil {
			t.Errorf("configure accepted %s", block)
		}
	}
}

// A zero is how an absent field arrives, so it takes the default rather than
// being refused as out of range.
func TestZeroMeansDefault(t *testing.T) {
	p, on, err := configure(t, `{"maxLines": 0, "requestTimeout": 0}`, creds)
	if err != nil || !on {
		t.Fatalf("configure: on=%v err=%v", on, err)
	}
	if p.cfg.MaxLines != 30 || p.cfg.RequestTimeoutMS != 10000 {
		t.Errorf("cfg = %+v, want the defaults applied", p.cfg)
	}
}

func TestPrivilegedNicksAreLowercased(t *testing.T) {
	p, _, err := configure(t, `{"privileged": {"SomeNick": {"host": "a.host"}}}`, creds)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.cfg.Privileged["somenick"]; !ok {
		t.Errorf("privileged = %v, want the nick lowercased", p.cfg.Privileged)
	}
}

func TestBadJSONIsAnError(t *testing.T) {
	if _, _, err := configure(t, `{"maxLines": "thirty"}`, creds); err == nil {
		t.Error("configure accepted a block it could not parse")
	}
}

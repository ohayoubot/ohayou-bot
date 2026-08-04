package ohayou

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/plugin"
)

const dataDir = `{"dataDir": "../../../data"}`

func configure(t *testing.T, block string) (*Plugin, bool, error) {
	t.Helper()
	p := New()
	var raw json.RawMessage
	if block != "" {
		raw = json.RawMessage(block)
	}
	on, err := p.Configure(plugin.Config{Block: raw})
	return p, on, err
}

// The game is the bot's oldest functionality, so it stays on unless the
// operator says otherwise.
func TestOnByDefault(t *testing.T) {
	p, on, err := configure(t, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("the game did not come up from a config that says nothing")
	}
	if len(p.items) == 0 {
		t.Error("no items loaded")
	}
	if len(p.fortunes) == 0 {
		t.Error("no fortunes loaded")
	}
	if p.cfg.Timezone != "America/New_York" {
		t.Errorf("Timezone = %q, want the default", p.cfg.Timezone)
	}
}

// Turning it off must leave the bot needing no data directory at all.
func TestDisabledNeedsNoDataFiles(t *testing.T) {
	p, on, err := configure(t, `{"enabled": false, "dataDir": "/nonexistent"}`)
	if err != nil {
		t.Fatalf("a disabled game still went looking for its files: %v", err)
	}
	if on {
		t.Error("the game came up despite being turned off")
	}
	if len(p.items) != 0 {
		t.Error("a disabled game loaded its catalog anyway")
	}
}

// A game that cannot find its catalog says so at startup rather than answering
// every command with an error.
func TestMissingDataIsAnError(t *testing.T) {
	_, _, err := configure(t, `{"dataDir": "/nonexistent"}`)
	if err == nil {
		t.Fatal("configure accepted a game with no catalog")
	}
	if !strings.Contains(err.Error(), "items") {
		t.Errorf("err = %v, want it to name what was missing", err)
	}
}

func TestTimezoneIsConfigurable(t *testing.T) {
	p, _, err := configure(t, `{"dataDir": "../../../data", "timezone": "UTC"}`)
	if err != nil {
		t.Fatal(err)
	}
	if p.cfg.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want UTC", p.cfg.Timezone)
	}
}

func TestBadTimezoneIsRefusedAtRegister(t *testing.T) {
	p, _, err := configure(t, `{"dataDir": "../../../data", "timezone": "Mars/Olympus"}`)
	if err != nil {
		t.Fatal(err)
	}
	err = p.Register(plugin.Deps{Bot: testBot(t), Log: discardLog()})
	if err == nil {
		t.Fatal("register accepted a timezone that does not exist")
	}
	if !strings.Contains(err.Error(), "Mars/Olympus") {
		t.Errorf("err = %v, want it to name the timezone", err)
	}
}

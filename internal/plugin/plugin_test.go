package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/config"
)

type fake struct {
	name   string
	order  *[]string // shared, so Register order can be asserted
	regErr error
}

func (f *fake) Name() string { return f.name }

func (f *fake) Register(deps Deps) error {
	if f.regErr != nil {
		return f.regErr
	}
	*f.order = append(*f.order, f.name)
	return nil
}

func testDeps() Deps {
	return Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestRegisterRunsInTheOrderAdded(t *testing.T) {
	var order []string
	r := NewRegistry(testDeps())
	for _, name := range []string{"first", "second", "third"} {
		r.Add(&fake{name: name, order: &order})
	}

	if err := r.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := strings.Join(order, " "); got != "first second third" {
		t.Errorf("registered %q, want them in the order added", got)
	}
	if got := strings.Join(r.Names(), " "); got != "first second third" {
		t.Errorf("Names = %q", got)
	}
}

// A plugin that was configured but cannot register is fatal: silently doing
// nothing is the one outcome the operator did not ask for.
func TestRegisterStopsAtTheFirstFailure(t *testing.T) {
	var order []string
	r := NewRegistry(testDeps())
	r.Add(&fake{name: "fine", order: &order})
	r.Add(&fake{name: "broken", order: &order, regErr: errors.New("no credentials")})
	r.Add(&fake{name: "never", order: &order})

	err := r.Register()
	if err == nil {
		t.Fatal("Register succeeded with a broken plugin")
	}
	if !strings.Contains(err.Error(), "broken") || !strings.Contains(err.Error(), "no credentials") {
		t.Errorf("err = %v, want it to name the plugin and the cause", err)
	}
	if got := strings.Join(order, " "); got != "fine" {
		t.Errorf("registered %q, want nothing after the failure", got)
	}
}

func TestEachPluginGetsItsOwnLogger(t *testing.T) {
	var buf strings.Builder
	deps := Deps{Log: slog.New(slog.NewTextHandler(&buf, nil))}

	r := NewRegistry(deps)
	r.Add(&noisy{name: "talker"})
	if err := r.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if !strings.Contains(buf.String(), "plugin=talker") {
		t.Errorf("log did not name the plugin: %q", buf.String())
	}
}

type noisy struct{ name string }

func (n *noisy) Name() string { return n.name }

func (n *noisy) Register(deps Deps) error {
	deps.Log.Info("hello")
	return nil
}

type starter struct {
	name    string
	started *[]string
	err     error
}

func (s *starter) Name() string             { return s.name }
func (s *starter) Register(deps Deps) error { return nil }
func (s *starter) Start(ctx context.Context) error {
	if s.err != nil {
		return s.err
	}
	*s.started = append(*s.started, s.name)
	return nil
}

func TestStartSkipsPluginsWithoutBackgroundWork(t *testing.T) {
	var order, started []string
	r := NewRegistry(testDeps())
	r.Add(&fake{name: "quiet", order: &order})
	r.Add(&starter{name: "busy", started: &started})

	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := strings.Join(started, " "); got != "busy" {
		t.Errorf("started %q, want only the plugin that has work", got)
	}
}

func TestStartReportsAFailure(t *testing.T) {
	var started []string
	r := NewRegistry(testDeps())
	r.Add(&starter{name: "busy", started: &started, err: errors.New("nope")})

	err := r.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Errorf("err = %v, want it to name the plugin that failed", err)
	}
}

type configurable struct {
	name string
	on   bool
	err  error
	got  Config
}

func (c *configurable) Name() string             { return c.name }
func (c *configurable) Register(deps Deps) error { return nil }
func (c *configurable) Configure(pc Config) (bool, error) {
	c.got = pc
	return c.on, c.err
}

func TestConfigureDropsThePluginsNobodyAskedFor(t *testing.T) {
	r := NewRegistry(testDeps())
	r.Add(&configurable{name: "wanted", on: true})
	r.Add(&configurable{name: "unwanted"})

	if err := r.Configure(&config.Config{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if got := strings.Join(r.Names(), " "); got != "wanted" {
		t.Errorf("kept %q, want only the plugin that came up", got)
	}
}

// A plugin with no Configure is always kept: it has nothing to turn off.
func TestConfigureKeepsPluginsWithoutABlock(t *testing.T) {
	var order []string
	r := NewRegistry(testDeps())
	r.Add(&fake{name: "always", order: &order})

	if err := r.Configure(&config.Config{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if got := strings.Join(r.Names(), " "); got != "always" {
		t.Errorf("kept %q", got)
	}
}

func TestConfigureHandsOverTheBlockAndTheSharedSettings(t *testing.T) {
	p := &configurable{name: "reader", on: true}
	r := NewRegistry(testDeps())
	r.Add(p)

	cfg := &config.Config{
		Cloudflare: config.Cloudflare{AccountID: "acct"},
		Plugins:    map[string]json.RawMessage{"reader": json.RawMessage(`{"x": 1}`)},
	}
	if err := r.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if string(p.got.Block) != `{"x": 1}` {
		t.Errorf("Block = %q", p.got.Block)
	}
	if p.got.Cloudflare.AccountID != "acct" {
		t.Errorf("Cloudflare = %+v, want the shared block passed through", p.got.Cloudflare)
	}
}

func TestConfigureReportsABadBlock(t *testing.T) {
	r := NewRegistry(testDeps())
	r.Add(&configurable{name: "broken", err: errors.New("url is required")})

	err := r.Configure(&config.Config{})
	if err == nil {
		t.Fatal("Configure accepted a plugin that refused its own config")
	}
	if !strings.Contains(err.Error(), "broken") || !strings.Contains(err.Error(), "url is required") {
		t.Errorf("err = %v, want it to name the plugin and the cause", err)
	}
}

// A block naming nothing is a typo the operator wants told about, not silently
// ignored while they wonder why the plugin never answers.
func TestConfigureRejectsABlockForNobody(t *testing.T) {
	r := NewRegistry(testDeps())
	r.Add(&configurable{name: "real", on: true})

	cfg := &config.Config{Plugins: map[string]json.RawMessage{"typo": json.RawMessage(`{}`)}}
	err := r.Configure(cfg)
	if err == nil || !strings.Contains(err.Error(), "typo") {
		t.Errorf("err = %v, want it to name the block nothing owns", err)
	}
}

// A block for a plugin the operator then turned off is not a typo.
func TestABlockForADisabledPluginIsFine(t *testing.T) {
	r := NewRegistry(testDeps())
	r.Add(&configurable{name: "off"})

	cfg := &config.Config{Plugins: map[string]json.RawMessage{"off": json.RawMessage(`{"enabled": false}`)}}
	if err := r.Configure(cfg); err != nil {
		t.Errorf("Configure: %v", err)
	}
}

// Package plugin is the contract between the bot and the things it does.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/store"
	"github.com/ohayoubot/ohayou-bot/internal/task"
)

// Deps is what the bot lends a plugin.
type Deps struct {
	Bot   *bot.Bot
	Store store.Store
	Log   *slog.Logger
	// Runner is the bot-wide task clock. Plugins use the scoped Tasks below.
	Runner *task.Runner
	// KV is the plugin's own corner of the store, for the state it wants back
	// after a restart. Keys are already scoped to the plugin.
	KV *store.KV
	// Tasks schedules work for later, including after a restart. Already scoped
	// to the plugin.
	Tasks *task.Queue
}

// For scopes deps to one plugin: its own logger, and its own corner of the
// store. The registry does this for every plugin, and a test wiring one up by
// hand should too.
func (d Deps) For(name string) Deps {
	d.Log = d.Log.With("plugin", name)
	if d.Store != nil {
		d.KV = store.Namespace(d.Store, name)
	}
	if d.Runner != nil {
		d.Tasks = d.Runner.For(name)
	}
	return d
}

// Plugin is anything the bot can be taught to do. Register claims commands and
// watchers; it runs once, before the connection is up.
type Plugin interface {
	Name() string
	Register(Deps) error
}

// Config is what a plugin is given to configure itself
type Config struct {
	// Block is the plugin's entry under "plugins", nil when it has none.
	Block json.RawMessage
	// Cloudflare is the shared D1 database, for the plugins that read one.
	Cloudflare config.Cloudflare
}

// Configurable is a plugin that reads its own config block. Returning false
// means the operator did not ask for it and it is left out entirely; an error
// means they did ask and got it wrong, which is fatal.
type Configurable interface {
	Configure(Config) (bool, error)
}

// Starter is a plugin with background work. Start must not block: anything
// long-running belongs in a goroutine from Deps.Bot.Go, which shutdown drains.
type Starter interface {
	Start(ctx context.Context) error
}

// Stopper is a plugin with state worth writing down before the bot exits. Stop
// runs after the connection is closed and the tracked goroutines have drained,
// with the store still open.
type Stopper interface {
	Stop(ctx context.Context) error
}

// Registry holds the plugins in the order they were added, which is the order
// they register and start in.
type Registry struct {
	deps    Deps
	plugins []Plugin
}

func NewRegistry(deps Deps) *Registry {
	return &Registry{deps: deps}
}

// Add appends plugins in the order they should register.
func (r *Registry) Add(ps ...Plugin) { r.plugins = append(r.plugins, ps...) }

// Configure gives every plugin its block and drops the ones the operator did
// not ask for. A half-configured plugin is an error rather than one that
// quietly never answers: a nick typing a command and getting silence is worse
// than a refusal at startup.
func (r *Registry) Configure(cfg *config.Config) error {
	// Checked before anything is dropped, so a block naming a plugin the
	// operator then turned off is not reported as an unknown one.
	known := map[string]bool{}
	for _, p := range r.plugins {
		known[p.Name()] = true
	}
	for name := range cfg.Plugins {
		if !known[name] {
			return fmt.Errorf("config: no plugin named %q", name)
		}
	}

	kept := r.plugins[:0]
	for _, p := range r.plugins {
		c, ok := p.(Configurable)
		if !ok {
			kept = append(kept, p)
			continue
		}
		on, err := c.Configure(Config{
			Block:      cfg.Plugins[p.Name()],
			Cloudflare: cfg.Cloudflare,
		})
		if err != nil {
			return fmt.Errorf("plugin %s: %w", p.Name(), err)
		}
		if on {
			kept = append(kept, p)
		} else {
			r.deps.Log.Info("plugin off", "plugin", p.Name())
		}
	}
	r.plugins = kept
	return nil
}

// Names lists the registered plugins.
func (r *Registry) Names() []string {
	names := make([]string, len(r.plugins))
	for i, p := range r.plugins {
		names[i] = p.Name()
	}
	return names
}

// Register hands each plugin its dependencies. A plugin that cannot register is
// fatal rather than skipped: it was configured, so silently doing nothing is
// the one outcome the operator did not ask for.
func (r *Registry) Register() error {
	for _, p := range r.plugins {
		if err := p.Register(r.deps.For(p.Name())); err != nil {
			return fmt.Errorf("plugin %s: %w", p.Name(), err)
		}
	}
	r.deps.Log.Info("plugins registered", "plugins", strings.Join(r.Names(), " "))
	return nil
}

// Start runs the background work of every plugin that has any.
func (r *Registry) Start(ctx context.Context) error {
	for _, p := range r.plugins {
		s, ok := p.(Starter)
		if !ok {
			continue
		}
		if err := s.Start(ctx); err != nil {
			return fmt.Errorf("starting %s: %w", p.Name(), err)
		}
	}
	return nil
}

// Stop gives every plugin a chance to write down what it wants back. A failure
// is logged rather than returned: one plugin failing to save is no reason to
// deny the others the chance.
func (r *Registry) Stop(ctx context.Context) {
	for _, p := range r.plugins {
		s, ok := p.(Stopper)
		if !ok {
			continue
		}
		if err := s.Stop(ctx); err != nil {
			r.deps.Log.Error("stopping plugin", "plugin", p.Name(), "err", err)
		}
	}
}

// Package plugin is the contract between the bot and the things it does.
package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// Deps is what the bot lends a plugin.
type Deps struct {
	Bot   *bot.Bot
	Store store.Store
	Log   *slog.Logger
}

// Plugin is anything the bot can be taught to do. Register claims commands and
// watchers; it runs once, before the connection is up.
type Plugin interface {
	Name() string
	Register(Deps) error
}

// Starter is a plugin with background work. Start must not block: anything
// long-running belongs in a goroutine from Deps.Bot.Go, which shutdown drains.
type Starter interface {
	Start(ctx context.Context) error
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

// Add appends a plugin. A plugin the operator turned off is never added, so it
// costs nothing beyond the config that named it.
func (r *Registry) Add(p Plugin) { r.plugins = append(r.plugins, p) }

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
		deps := r.deps
		deps.Log = r.deps.Log.With("plugin", p.Name())
		if err := p.Register(deps); err != nil {
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

package plugin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
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

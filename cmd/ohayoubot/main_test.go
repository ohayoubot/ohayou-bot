package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
)

// conf-example.json is what an operator copies, so it has to survive the
// registry the binary actually builds.
func TestExampleConfigThroughTheRegistry(t *testing.T) {
	t.Setenv("OHAYOU_CF_API_TOKEN", "")
	t.Setenv("OHAYOU_DROP_SECRET", "")

	cfg, err := config.Load("../../conf-example.json")
	if err != nil {
		t.Fatalf("conf-example.json does not load: %v", err)
	}

	reg := plugin.NewRegistry(plugin.Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	reg.Add(plugins()...)
	if err := reg.Configure(cfg); err != nil {
		t.Fatalf("configure: %v", err)
	}

	// The example ships no credentials, so only what needs none comes up.
	if got := strings.Join(reg.Names(), " "); got != "catfact youtube" {
		t.Errorf("enabled = %q, want only the plugins needing no credentials", got)
	}
}

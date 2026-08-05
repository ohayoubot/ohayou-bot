package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
)

// conf-example.json is what an operator copies, so it has to survive the
// registry the binary actually builds.
func TestExampleConfigThroughTheRegistry(t *testing.T) {
	t.Setenv("OHAYOU_CF_API_TOKEN", "")
	t.Setenv("OHAYOU_WEB_SECRET", "")

	cfg, err := config.Load("../../conf-example.json")
	if err != nil {
		t.Fatalf("conf-example.json does not load: %v", err)
	}
	// The example points at a data directory beside the binary; the test runs
	// from this package.
	cfg.Plugins["ohayou"] = json.RawMessage(`{"dataDir": "../../data"}`)

	reg := plugin.NewRegistry(plugin.Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	reg.Add(plugins()...)
	if err := reg.Configure(cfg); err != nil {
		t.Fatalf("configure: %v", err)
	}

	// The example ships no credentials, so only what needs none comes up.
	if got := strings.Join(reg.Names(), " "); got != "catfact youtube ohayou" {
		t.Errorf("enabled = %q, want only the plugins needing no credentials", got)
	}
}

// -check is what install.sh runs before it restarts anything, so a config that
// would fail at startup fails here instead of in a restart loop.
func TestCheckAcceptsTheExampleConfig(t *testing.T) {
	t.Setenv("OHAYOU_CF_API_TOKEN", "")
	t.Setenv("OHAYOU_WEB_SECRET", "")
	// The example points dataDir at a directory beside the binary.
	t.Chdir("../..")

	if err := validate("conf-example.json", discardLog()); err != nil {
		t.Fatalf("the example config does not pass -check: %v", err)
	}
}

func TestCheckRefusesABadConfig(t *testing.T) {
	t.Setenv("OHAYOU_WEB_SECRET", "")

	data, err := filepath.Abs("../../data")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	for name, body := range map[string]string{
		"unreadable.json": `{"nick":`,
		"unknown.json":    `{"nick":"b","server":"s","plugins":{"nonesuch":{}}}`,
		"badzone.json":    `{"nick":"b","server":"s","plugins":{"ohayou":{"dataDir":"DATA","timezone":"Mars/Olympus"}}}`,
	} {
		path := filepath.Join(dir, name)
		body = strings.ReplaceAll(body, "DATA", data)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := validate(path, discardLog()); err == nil {
			t.Errorf("%s passed -check", name)
		}
	}

	if err := validate(filepath.Join(dir, "absent.json"), discardLog()); err == nil {
		t.Error("a missing config passed -check")
	}
}

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

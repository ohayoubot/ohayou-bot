package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func boolPtr(b bool) *bool { return &b }

func TestSASLUse(t *testing.T) {
	tests := []struct {
		name string
		sasl SASLConfig
		want bool
	}{
		{"unconfigured", SASLConfig{}, false},
		{"plain configured auto-on", SASLConfig{Login: "bot", Password: "pw"}, true},
		{"plain missing password", SASLConfig{Login: "bot"}, false},
		{"external auto-on", SASLConfig{Mechanism: "external"}, true},
		{"explicit off despite creds", SASLConfig{Enabled: boolPtr(false), Login: "bot", Password: "pw"}, false},
		{"explicit on without creds", SASLConfig{Enabled: boolPtr(true)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sasl.Use(); got != tt.want {
				t.Errorf("Use() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSASLMechDefault(t *testing.T) {
	if got := (SASLConfig{}).Mech(); got != "PLAIN" {
		t.Errorf("Mech() = %q, want PLAIN", got)
	}
	if got := (SASLConfig{Mechanism: "external"}).Mech(); got != "EXTERNAL" {
		t.Errorf("Mech() = %q, want EXTERNAL", got)
	}
}

func TestLoadNormalizesAdminNicks(t *testing.T) {
	cfg, err := Load(writeConfig(t, `{
		"nick": "ohayoubot", "server": "irc.example.net",
		"admins": {"AdminNick": "vhost.example"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if host, ok := cfg.Admins["adminnick"]; !ok || host != "vhost.example" {
		t.Errorf("admin key not lower-cased: %v", cfg.Admins)
	}
	if _, ok := cfg.Admins["AdminNick"]; ok {
		t.Error("original mixed-case admin key should not remain")
	}
}

func TestVHostUse(t *testing.T) {
	tests := []struct {
		name   string
		vhost  VHostConfig
		server string
		want   bool
	}{
		{"rizon auto-on", VHostConfig{}, "irc.rizon.net", true},
		{"non-rizon auto-off", VHostConfig{}, "irc.libera.chat", false},
		{"explicit on non-rizon", VHostConfig{Enabled: boolPtr(true)}, "irc.libera.chat", true},
		{"explicit off on rizon", VHostConfig{Enabled: boolPtr(false)}, "irc.rizon.net", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.vhost.Use(tt.server); got != tt.want {
				t.Errorf("Use(%q) = %v, want %v", tt.server, got, tt.want)
			}
		})
	}
}

func TestLoadVHostDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, `{"nick": "ohayoubot", "server": "irc.rizon.net"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.VHostEnabled() {
		t.Fatal("expected vhost gate enabled by default on Rizon")
	}
	if cfg.VHost.Service != "HostServ" {
		t.Errorf("Service = %q, want HostServ", cfg.VHost.Service)
	}
	if cfg.VHost.Command != "ON" {
		t.Errorf("Command = %q, want ON", cfg.VHost.Command)
	}
	if cfg.VHostTimeout() != 10*time.Second {
		t.Errorf("VHostTimeout = %v, want 10s", cfg.VHostTimeout())
	}

	off, err := Load(writeConfig(t, `{"nick": "ohayoubot", "server": "irc.libera.chat"}`))
	if err != nil {
		t.Fatal(err)
	}
	if off.VHostEnabled() {
		t.Fatal("expected vhost gate disabled by default off Rizon")
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "conf.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSASL(t *testing.T) {
	t.Run("plain enabled by default when configured", func(t *testing.T) {
		cfg, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"sasl": {"login": "ohayoubot", "password": "secret"}
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.SASL.Use() {
			t.Fatal("expected SASL to be used by default when configured")
		}
	})

	t.Run("tls defaults port to 6697", func(t *testing.T) {
		cfg, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net", "tls": true
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Port != 6697 {
			t.Errorf("Port = %d, want 6697", cfg.Port)
		}
	})

	t.Run("plain without credentials is rejected", func(t *testing.T) {
		_, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"sasl": {"enabled": true, "mechanism": "PLAIN"}
		}`))
		if err == nil {
			t.Fatal("expected error for PLAIN without credentials")
		}
	})

	t.Run("unsupported mechanism is rejected", func(t *testing.T) {
		_, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"sasl": {"enabled": true, "mechanism": "SCRAM-SHA-256", "login": "a", "password": "b"}
		}`))
		if err == nil {
			t.Fatal("expected error for unsupported mechanism")
		}
	})
}

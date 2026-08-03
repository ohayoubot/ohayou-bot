package config

import (
	"os"
	"path/filepath"
	"strconv"
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

func TestLoadDeerkins(t *testing.T) {
	const configured = `{
		"nick": "ohayoubot", "server": "irc.example.net",
		"deerkins": {"accountId": "acct", "databaseId": "db", "apiToken": "token"}
	}`

	t.Run("off unless configured", func(t *testing.T) {
		t.Setenv("DEERKINS_API_TOKEN", "")
		cfg, err := Load(writeConfig(t, `{"nick": "ohayoubot", "server": "irc.example.net"}`))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Deerkins.Use() {
			t.Error("deerkins came up without a database")
		}
	})

	t.Run("defaults", func(t *testing.T) {
		t.Setenv("DEERKINS_API_TOKEN", "")
		cfg, err := Load(writeConfig(t, configured))
		if err != nil {
			t.Fatal(err)
		}
		d := cfg.Deerkins
		if !d.Use() {
			t.Fatal("deerkins did not come up from a complete config")
		}
		if d.Wait() != 300*time.Second {
			t.Errorf("Wait = %v, want 5m", d.Wait())
		}
		if d.MissWait() != 15*time.Second {
			t.Errorf("MissWait = %v, want 15s", d.MissWait())
		}
		if d.RequestTimeout() != 10*time.Second {
			t.Errorf("RequestTimeout = %v, want 10s", d.RequestTimeout())
		}
		if d.TimeoutPunish != 1.7 || d.MaxLines != 30 || d.Editor == "" {
			t.Errorf("defaults not applied: %+v", d)
		}
		if !d.MatchNick() || !d.MatchHost() {
			t.Error("privileged nicks should have to match on both nick and host by default")
		}
	})

	t.Run("token comes from the environment", func(t *testing.T) {
		t.Setenv("DEERKINS_API_TOKEN", "from-env")
		cfg, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"deerkins": {"accountId": "acct", "databaseId": "db"}
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.Deerkins.Use() || cfg.Deerkins.APIToken != "from-env" {
			t.Errorf("APIToken = %q, want the environment's", cfg.Deerkins.APIToken)
		}
	})

	t.Run("half a config is an error", func(t *testing.T) {
		t.Setenv("DEERKINS_API_TOKEN", "")
		_, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"deerkins": {"enabled": true, "accountId": "acct"}
		}`))
		if err == nil {
			t.Fatal("expected an error for a database that can't be reached")
		}
	})

	t.Run("enabled false wins", func(t *testing.T) {
		t.Setenv("DEERKINS_API_TOKEN", "")
		cfg, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"deerkins": {"enabled": false, "accountId": "acct", "databaseId": "db", "apiToken": "t"}
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Deerkins.Use() {
			t.Error("deerkins ignored enabled:false")
		}
	})

	t.Run("privileged nicks are lower-cased", func(t *testing.T) {
		t.Setenv("DEERKINS_API_TOKEN", "")
		cfg, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"deerkins": {"accountId": "acct", "databaseId": "db", "apiToken": "t",
				"privileged": {"SvaJ": {"host": "wizard.of.the.night", "timeout": -10}}}
		}`))
		if err != nil {
			t.Fatal(err)
		}
		user, ok := cfg.Deerkins.Privileged["svaj"]
		if !ok {
			t.Fatalf("privileged key not lower-cased: %v", cfg.Deerkins.Privileged)
		}
		if user.Timeout == nil || *user.Timeout != -10 {
			t.Errorf("Timeout = %v, want -10", user.Timeout)
		}
	})
}

func TestLoadDrop(t *testing.T) {
	const configured = `{
		"nick": "ohayoubot", "server": "irc.example.net",
		"deerkins": {"accountId": "acct", "databaseId": "db", "apiToken": "token"},
		"drop": {"url": "https://hemera.day/drop/"}
	}`

	clearEnv := func(t *testing.T) {
		t.Helper()
		t.Setenv("DEERKINS_API_TOKEN", "")
		t.Setenv("OHAYOU_DROP_SECRET", "")
		t.Setenv("OHAYOU_DROP_TOKEN", "")
	}

	t.Run("off without a secret", func(t *testing.T) {
		clearEnv(t)
		cfg, err := Load(writeConfig(t, configured))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Drop.Use() {
			t.Error("drop came up with no signing secret")
		}
	})

	t.Run("off without a url", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("OHAYOU_DROP_SECRET", "s")
		cfg, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"deerkins": {"accountId": "acct", "databaseId": "db", "apiToken": "token"}
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Drop.Use() {
			t.Error("drop came up with nowhere to send anyone")
		}
	})

	t.Run("defaults", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("OHAYOU_DROP_SECRET", "s")
		cfg, err := Load(writeConfig(t, configured))
		if err != nil {
			t.Fatal(err)
		}
		d := cfg.Drop
		if !d.Use() {
			t.Fatal("drop did not come up from a complete config")
		}
		if d.GrantWait() != 300*time.Second {
			t.Errorf("GrantWait = %v, want 5m", d.GrantWait())
		}
		if d.PollWait() != 10*time.Second {
			t.Errorf("PollWait = %v, want 10s", d.PollWait())
		}
		if d.CooldownWait() != 60*time.Second {
			t.Errorf("CooldownWait = %v, want 1m", d.CooldownWait())
		}
		if d.RequestTimeout() != 10*time.Second {
			t.Errorf("RequestTimeout = %v, want 10s", d.RequestTimeout())
		}
	})

	t.Run("database borrowed from deerkins", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("OHAYOU_DROP_SECRET", "s")
		cfg, err := Load(writeConfig(t, configured))
		if err != nil {
			t.Fatal(err)
		}
		d := cfg.Drop
		if d.AccountID != "acct" || d.DatabaseID != "db" || d.APIToken != "token" {
			t.Errorf("did not inherit the deerkins database: %+v", d)
		}
	})

	t.Run("its own database wins", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("OHAYOU_DROP_SECRET", "s")
		cfg, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"deerkins": {"accountId": "acct", "databaseId": "db", "apiToken": "token"},
			"drop": {"url": "https://hemera.day/drop/", "databaseId": "other", "apiToken": "own"}
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Drop.DatabaseID != "other" || cfg.Drop.APIToken != "own" {
			t.Errorf("drop should keep its own database: %+v", cfg.Drop)
		}
		if cfg.Drop.AccountID != "acct" {
			t.Errorf("AccountID = %q, want the inherited one", cfg.Drop.AccountID)
		}
	})

	t.Run("the secret is never read from the config file", func(t *testing.T) {
		clearEnv(t)
		cfg, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"deerkins": {"accountId": "acct", "databaseId": "db", "apiToken": "token"},
			"drop": {"url": "https://hemera.day/drop/", "secret": "committed-by-mistake"}
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Drop.Secret != "" {
			t.Errorf("Secret = %q, want it ignored in json", cfg.Drop.Secret)
		}
	})

	t.Run("token comes from the environment", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("OHAYOU_DROP_SECRET", "s")
		t.Setenv("OHAYOU_DROP_TOKEN", "from-env")
		cfg, err := Load(writeConfig(t, configured))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Drop.APIToken != "from-env" {
			t.Errorf("APIToken = %q, want the environment's", cfg.Drop.APIToken)
		}
	})

	t.Run("enabled without a secret is an error", func(t *testing.T) {
		clearEnv(t)
		_, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"deerkins": {"accountId": "acct", "databaseId": "db", "apiToken": "token"},
			"drop": {"enabled": true, "url": "https://hemera.day/drop/"}
		}`))
		if err == nil {
			t.Fatal("expected an error for a drop plugin that cannot sign")
		}
	})

	t.Run("enabled false wins", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("OHAYOU_DROP_SECRET", "s")
		cfg, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"deerkins": {"accountId": "acct", "databaseId": "db", "apiToken": "token"},
			"drop": {"enabled": false, "url": "https://hemera.day/drop/"}
		}`))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Drop.Use() {
			t.Error("enabled false should keep drop off")
		}
	})

	t.Run("a grant that outlives what the worker accepts is an error", func(t *testing.T) {
		for _, ttl := range []int{20, 901} {
			clearEnv(t)
			t.Setenv("OHAYOU_DROP_SECRET", "s")
			_, err := Load(writeConfig(t, `{
				"nick": "ohayoubot", "server": "irc.example.net",
				"deerkins": {"accountId": "acct", "databaseId": "db", "apiToken": "token"},
				"drop": {"url": "https://hemera.day/drop/", "grantTtl": `+strconv.Itoa(ttl)+`}
			}`))
			if err == nil {
				t.Errorf("grantTtl %d was accepted", ttl)
			}
		}
	})

	t.Run("polling too fast is an error", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("OHAYOU_DROP_SECRET", "s")
		_, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"deerkins": {"accountId": "acct", "databaseId": "db", "apiToken": "token"},
			"drop": {"url": "https://hemera.day/drop/", "poll": 1}
		}`))
		if err == nil {
			t.Fatal("expected an error for a 1 second poll")
		}
	})

	t.Run("no database anywhere is an error, not silence", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("OHAYOU_DROP_SECRET", "s")
		_, err := Load(writeConfig(t, `{
			"nick": "ohayoubot", "server": "irc.example.net",
			"drop": {"url": "https://hemera.day/drop/"}
		}`))
		if err == nil {
			t.Fatal("expected an error for a drop plugin with no database to read")
		}
	})

	t.Run("the link puts the grant in the fragment", func(t *testing.T) {
		d := DropConfig{URL: "https://hemera.day/drop/"}
		if got := d.Link("v1.a.b"); got != "https://hemera.day/drop/#v1.a.b" {
			t.Errorf("Link = %q", got)
		}
		d.URL = "https://hemera.day/drop/#"
		if got := d.Link("v1.a.b"); got != "https://hemera.day/drop/#v1.a.b" {
			t.Errorf("Link with a trailing hash = %q", got)
		}
	})
}

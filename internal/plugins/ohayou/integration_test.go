package ohayou_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/bot"
	"github.com/ohayoubot/ohayou-bot/internal/config"
	"github.com/ohayoubot/ohayou-bot/internal/plugin"
	"github.com/ohayoubot/ohayou-bot/internal/plugins/ohayou"
	"github.com/ohayoubot/ohayou-bot/internal/store/sqlite"
)

func TestOhayouEndToEnd(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	reply := make(chan string, 16)
	go fakeServer(t, ln, reply)

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	if err := db.Init(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}

	cfg := &config.Config{
		Nick:          "ohayoubot",
		User:          "ohayoubot",
		Server:        "127.0.0.1",
		Port:          port,
		Channels:      []string{"#test"},
		CommandPrefix: "!",
		Admins:        map[string]string{},
		IgnoreList:    map[string]string{},
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := bot.New(cfg, log)
	g := ohayou.New()
	if _, err := g.Configure(plugin.Config{
		Block: json.RawMessage(`{"dataDir": "../../../data"}`),
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	deps := plugin.Deps{Bot: b, Store: db, Log: log}
	if err := g.Register(deps.For("ohayou")); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := g.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()

	select {
	case line := <-reply:
		if !strings.Contains(line, "PRIVMSG #test") ||
			!strings.Contains(strings.ToLower(line), "first ohayou") {
			t.Fatalf("unexpected reply: %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the bot's ohayou reply")
	}

	// user should now exist in the store
	if _, err := db.GetUser(context.Background(), "alice"); err != nil {
		t.Errorf("expected alice to be persisted: %v", err)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func fakeServer(t *testing.T, ln net.Listener, reply chan<- string) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	welcomed := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		switch {
		case strings.HasPrefix(line, "NICK") && !welcomed:
			welcomed = true
			fmt.Fprint(conn, ":test.server 001 ohayoubot :Welcome to the test network\r\n")
		case strings.HasPrefix(line, "JOIN"):
			// bot has joined. deliver a command.
			fmt.Fprint(conn, ":alice!user@host PRIVMSG #test :!ohayou\r\n")
		case strings.HasPrefix(line, "PRIVMSG #test"):
			select {
			case reply <- line:
			default:
			}
		}
	}
}

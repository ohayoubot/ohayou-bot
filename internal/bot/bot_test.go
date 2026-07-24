package bot

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/config"
)

func TestRejoinsChannelsOnReRegister(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	joins := make(chan string, 8)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		welcomes := 0
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case strings.HasPrefix(line, "NICK") && welcomes == 0:
				welcomes++
				io.WriteString(conn, ":srv 001 ohayoubot :Welcome\r\n")
			case strings.HasPrefix(line, "JOIN"):
				joins <- line
				// after the first join replay 001 to mock a reconnect
				if welcomes == 1 {
					welcomes++
					io.WriteString(conn, ":srv 001 ohayoubot :Welcome again\r\n")
				}
			}
		}
	}()

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
	b := New(cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()

	for i := 1; i <= 2; i++ {
		select {
		case j := <-joins:
			if !strings.Contains(j, "#test") {
				t.Fatalf("join #%d: unexpected line %q", i, j)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for join #%d (rejoin after re-register failed)", i)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

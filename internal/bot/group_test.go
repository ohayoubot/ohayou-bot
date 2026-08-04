package bot

import (
	"io"
	"log/slog"
	"testing"
	"time"

	irc "github.com/ohayoubot/go-ircevent"

	"github.com/ohayoubot/ohayou-bot/internal/config"
)

func testBot() *Bot {
	cfg := &config.Config{
		Nick: "ohayoubot", User: "ohayoubot", Server: "127.0.0.1",
		CommandPrefix: "!",
		Admins:        map[string]string{},
		IgnoreList:    map[string]string{},
	}
	return New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// waited reports whether fn returned before the deadline.
func waited(fn func(), d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

func TestWaitDrainsTrackedGoroutines(t *testing.T) {
	b := testBot()

	release := make(chan struct{})
	finished := make(chan struct{})
	b.Go(func() {
		<-release
		close(finished)
	})

	if waited(b.Wait, 100*time.Millisecond) {
		t.Fatal("Wait returned while a tracked goroutine was still running")
	}

	close(release)
	if !waited(b.Wait, 3*time.Second) {
		t.Fatal("Wait never returned after the goroutine finished")
	}
	<-finished
}

// A command handler writing to the store must finish before the database is
// closed, so dispatch is tracked like any other background work.
func TestWaitDrainsACommandInFlight(t *testing.T) {
	b := testBot()

	started := make(chan struct{})
	release := make(chan struct{})
	b.HandleFunc("blocker", false, func(m *Message) {
		close(started)
		<-release
	})

	b.onPrivmsg(&irc.Event{
		Nick:      "someone",
		Host:      "example.host",
		Arguments: []string{"#chan", "!blocker"},
	})
	<-started

	if waited(b.Wait, 100*time.Millisecond) {
		t.Fatal("Wait returned while a command handler was still running")
	}

	close(release)
	if !waited(b.Wait, 3*time.Second) {
		t.Fatal("Wait never returned after the handler finished")
	}
}

func TestWaitDrainsAWatcherInFlight(t *testing.T) {
	b := testBot()

	started := make(chan struct{})
	release := make(chan struct{})
	b.Watch(func(m *Message) {
		close(started)
		<-release
	})

	b.onPrivmsg(&irc.Event{
		Nick:      "someone",
		Host:      "example.host",
		Arguments: []string{"#chan", "just talking"},
	})
	<-started

	if waited(b.Wait, 100*time.Millisecond) {
		t.Fatal("Wait returned while a watcher was still running")
	}

	close(release)
	if !waited(b.Wait, 3*time.Second) {
		t.Fatal("Wait never returned after the watcher finished")
	}
}

func TestWaitReturnsWithNothingRunning(t *testing.T) {
	b := testBot()
	if !waited(b.Wait, 3*time.Second) {
		t.Fatal("Wait blocked with no tracked goroutines")
	}
}

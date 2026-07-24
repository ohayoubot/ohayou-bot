package bot

import (
	"sync"
	"time"

	irc "github.com/thoj/go-ircevent"
)

type sender struct {
	conn  *irc.Connection
	delay time.Duration
	mu    sync.Mutex
	last  time.Time
}

func newSender(conn *irc.Connection, delay time.Duration) *sender {
	return &sender{conn: conn, delay: delay}
}

// gate blocks until enough time has passed since the previous send, then runs
// fn while holding the lock so ordering and spacing are preserved. i.e., it is
// the flood/spam protection
func (s *sender) gate(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.delay > 0 {
		if wait := s.delay - time.Since(s.last); wait > 0 {
			time.Sleep(wait)
		}
	}
	fn()
	s.last = time.Now()
}

func (s *sender) Privmsg(target, msg string) { s.gate(func() { s.conn.Privmsg(target, msg) }) }
func (s *sender) Notice(target, msg string)  { s.gate(func() { s.conn.Notice(target, msg) }) }
func (s *sender) Action(target, msg string)  { s.gate(func() { s.conn.Action(target, msg) }) }

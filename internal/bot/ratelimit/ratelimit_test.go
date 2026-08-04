package ratelimit

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// clock is a hand-wound Now for tests.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTest(window time.Duration) (*Limiter, *clock) {
	c := &clock{t: time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)}
	l := New(window)
	l.Now = c.now
	return l, c
}

func TestFirstClaimIsFree(t *testing.T) {
	l, _ := newTest(time.Minute)
	if wait, ok := l.Claim("#chan"); !ok || wait != 0 {
		t.Errorf("got (%v, %v), want (0, true)", wait, ok)
	}
}

func TestSecondClaimWaitsOutTheWindow(t *testing.T) {
	l, c := newTest(time.Minute)
	l.Claim("#chan")

	c.advance(20 * time.Second)
	wait, ok := l.Claim("#chan")
	if ok {
		t.Fatal("claimed twice inside the window")
	}
	if want := 40 * time.Second; wait != want {
		t.Errorf("wait = %v, want %v", wait, want)
	}

	c.advance(40 * time.Second)
	if _, ok := l.Claim("#chan"); !ok {
		t.Error("refused a claim after the window ran out")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l, _ := newTest(time.Minute)
	l.Claim("#one")
	if _, ok := l.Claim("#two"); !ok {
		t.Error("one key's turn blocked another's")
	}
}

func TestKeysAreOpaque(t *testing.T) {
	l, _ := newTest(time.Minute)
	l.Claim("#chan")
	if _, ok := l.Claim("#CHAN"); !ok {
		t.Error("the limiter folded case; keys are the caller's business")
	}
}

func TestClaimForAppliesToTheCallThatAsks(t *testing.T) {
	shorter, c := newTest(time.Minute)
	shorter.Claim("#chan")
	c.advance(11 * time.Second)
	if _, ok := shorter.ClaimFor("#chan", 10*time.Second); !ok {
		t.Error("a shorter window for this call did not apply")
	}

	longer, c2 := newTest(time.Minute)
	longer.Claim("#chan")
	c2.advance(61 * time.Second)
	if _, ok := longer.ClaimFor("#chan", 2*time.Minute); ok {
		t.Error("a longer window for this call did not apply")
	}
}

func TestUntilDoesNotTakeATurn(t *testing.T) {
	l, c := newTest(time.Minute)

	if d := l.Until("#chan"); d != 0 {
		t.Errorf("an unclaimed key is %v away, want 0", d)
	}

	l.Claim("#chan")
	c.advance(15 * time.Second)
	if d, want := l.Until("#chan"), 45*time.Second; d != want {
		t.Errorf("Until = %v, want %v", d, want)
	}
	if d, want := l.Until("#chan"), 45*time.Second; d != want {
		t.Errorf("Until moved the deadline: %v, want %v", d, want)
	}

	c.advance(45 * time.Second)
	if d := l.Until("#chan"); d != 0 {
		t.Errorf("Until = %v after the window, want 0", d)
	}
}

func TestDelayShortensAWindow(t *testing.T) {
	l, c := newTest(time.Minute)
	l.Claim("#chan")
	l.Delay("#chan", 15*time.Second)

	if _, ok := l.Claim("#chan"); ok {
		t.Error("claimed before the delay elapsed")
	}
	c.advance(16 * time.Second)
	if _, ok := l.Claim("#chan"); !ok {
		t.Error("the delay outlasted the time it asked for")
	}
}

func TestDelayLengthensAWindow(t *testing.T) {
	l, c := newTest(time.Minute)
	l.Claim("#chan")
	l.Delay("#chan", 2*time.Minute)

	c.advance(61 * time.Second)
	if _, ok := l.Claim("#chan"); ok {
		t.Error("the delay did not hold past the normal window")
	}
	c.advance(60 * time.Second)
	if _, ok := l.Claim("#chan"); !ok {
		t.Error("the delay never ran out")
	}
}

func TestExpiredEntriesAreForgotten(t *testing.T) {
	l, c := newTest(time.Minute)
	for i := 0; i < maxTracked; i++ {
		l.Claim(fmt.Sprintf("nick%d", i))
	}
	c.advance(2 * time.Minute)
	l.Claim("straggler")

	l.mu.Lock()
	n := len(l.seen)
	l.mu.Unlock()
	if n != 1 {
		t.Errorf("%d entries kept, want the expired ones dropped", n)
	}
}

func TestLiveEntriesSurviveTheSweep(t *testing.T) {
	l, _ := newTest(time.Minute)
	for i := 0; i < maxTracked+10; i++ {
		l.Claim(fmt.Sprintf("nick%d", i))
	}

	l.mu.Lock()
	n := len(l.seen)
	l.mu.Unlock()
	if n != maxTracked+10 {
		t.Errorf("%d entries, want all %d kept while their windows are live", n, maxTracked+10)
	}
}

func TestConcurrentClaimsGrantOneTurn(t *testing.T) {
	l, _ := newTest(time.Minute)

	var wg sync.WaitGroup
	granted := make(chan struct{}, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := l.Claim("#chan"); ok {
				granted <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(granted)

	if n := len(granted); n != 1 {
		t.Errorf("%d goroutines got the turn, want 1", n)
	}
}

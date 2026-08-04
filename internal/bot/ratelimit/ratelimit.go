// Package ratelimit spaces out repeated actions per key.
package ratelimit

import (
	"sync"
	"time"
)

// maxTracked caps the map so a busy network cannot grow it without bound.
const maxTracked = 1000

// Limiter grants a key one turn per window. Keys are opaque: a caller that
// wants "#Chan" and "#chan" to share a turn lowercases them itself.
type Limiter struct {
	// Now is the clock, swappable for tests.
	Now func() time.Time

	window time.Duration

	mu   sync.Mutex
	seen map[string]time.Time
}

func New(window time.Duration) *Limiter {
	return &Limiter{Now: time.Now, window: window, seen: map[string]time.Time{}}
}

// Claim takes key's turn, reporting how long is left when it cannot.
func (l *Limiter) Claim(key string) (time.Duration, bool) {
	return l.ClaimFor(key, l.window)
}

// ClaimFor is Claim measured against window rather than the limiter's, for
// callers whose spacing is decided per request.
func (l *Limiter) ClaimFor(key string, window time.Duration) (time.Duration, bool) {
	now := l.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if last, ok := l.seen[key]; ok {
		if wait := window - now.Sub(last); wait > 0 {
			return wait, false
		}
	}
	if len(l.seen) >= maxTracked {
		l.forgetOld(now, window)
	}
	l.seen[key] = now
	return 0, true
}

// Until reports how long until key is free, without taking its turn.
func (l *Limiter) Until(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	last, ok := l.seen[key]
	if !ok {
		return 0
	}
	if wait := l.window - l.Now().Sub(last); wait > 0 {
		return wait
	}
	return 0
}

// Delay makes key free d from now, however recently it was claimed.
func (l *Limiter) Delay(key string, d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen[key] = l.Now().Add(d - l.window)
}

// forgetOld drops entries whose window has run out. Called with mu held.
func (l *Limiter) forgetOld(now time.Time, window time.Duration) {
	for key, last := range l.seen {
		if now.Sub(last) >= window {
			delete(l.seen, key)
		}
	}
}

package ohayou

import (
	"encoding/json"
	"sync"
	"time"
)

// guard is the protection the Ohayou Police are giving one user: what it takes
// off a thief's odds, and what it loses each hour.
type guard struct {
	Ohayou    int `json:"ohayou"`
	Cat       int `json:"cat"`
	DecOhayou int `json:"decOhayou"`
	DecCat    int `json:"decCat"`
	// Since is when the guard last decayed, so a run that missed several hours
	// can catch up on all of them at once.
	Since time.Time `json:"since"`
}

// policeRegistry stores the protection of users who were robbed and reported
// it. Mutex guarded because timers and command handlers both reach it.
type policeRegistry struct {
	mu sync.Mutex
	m  map[string]guard
}

func newPoliceRegistry() *policeRegistry {
	return &policeRegistry{m: map[string]guard{}}
}

// reserve claims a slot for user with nothing in it. It returns false if the
// user is already protected, or a report is already pending.
func (r *policeRegistry) reserve(user string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[user]; ok {
		return false
	}
	r.m[user] = guard{}
	return true
}

// set stores the protection now in force for user.
func (r *policeRegistry) set(user string, g guard) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[user] = g
}

// bonus returns what the guard takes off a thief's odds, and whether the user
// is protected at all.
func (r *policeRegistry) bonus(user string) (ohayou, cat int, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.m[user]
	return v.Ohayou, v.Cat, ok
}

// decay ages a guard by however many whole periods have passed since it last
// did, so a bot that was down for three hours does not hand those three hours
// back. It reports whether the user is still protected afterwards.
func (r *policeRegistry) decay(user string, now time.Time, every time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	v, ok := r.m[user]
	if !ok {
		return false
	}

	steps := 1
	if every > 0 && !v.Since.IsZero() {
		if elapsed := int(now.Sub(v.Since) / every); elapsed > steps {
			steps = elapsed
		}
	}
	v.Ohayou -= v.DecOhayou * steps
	v.Cat -= v.DecCat * steps
	v.Since = now

	if v.Ohayou <= 0 {
		delete(r.m, user)
		return false
	}
	r.m[user] = v
	return true
}

// remove clears any protection for user.
func (r *policeRegistry) remove(user string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, user)
}

// protectedUsers lists who is under guard.
func (r *policeRegistry) protectedUsers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.m))
	for user := range r.m {
		out = append(out, user)
	}
	return out
}

// dump serialises the guards for a caller that wants them back after a restart.
func (r *policeRegistry) dump() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw, err := json.Marshal(r.m)
	return string(raw), err
}

// restore reads back what dump wrote.
func (r *policeRegistry) restore(raw string) error {
	var saved map[string]guard
	if err := json.Unmarshal([]byte(raw), &saved); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for user, g := range saved {
		r.m[user] = g
	}
	return nil
}

// offers holds the robbery victims the police have PM'd and are waiting on. A
// !report from one of them closes their channel; the offer expires with the
// goroutine that made it.
type offers struct {
	mu sync.Mutex
	m  map[string]chan struct{}
}

func newOffers() *offers { return &offers{m: map[string]chan struct{}{}} }

// open starts waiting on user, returning the channel a report closes and a
// func to stop waiting. A second offer to the same user is refused.
func (o *offers) open(user string) (<-chan struct{}, func(), bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, waiting := o.m[user]; waiting {
		return nil, nil, false
	}
	ch := make(chan struct{})
	o.m[user] = ch
	return ch, func() { o.close(user) }, true
}

func (o *offers) close(user string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.m, user)
}

// take reports whether user had an offer open, closing it if so.
func (o *offers) take(user string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	ch, ok := o.m[user]
	if !ok {
		return false
	}
	delete(o.m, user)
	close(ch)
	return true
}

package ohayou

import "sync"

// policeRegistry stores police protection data for a user who has been
// robbed and asked for protection.
// index 0 is the ohayou-steal defense bonus, index 1 the cat-steal bonus.
// mutex guarded because of goroutines/timers
type policeRegistry struct {
	mu sync.Mutex
	m  map[string][2]int
}

func newPoliceRegistry() *policeRegistry {
	return &policeRegistry{m: map[string][2]int{}}
}

// reserve claims a slot for user with a zero bonus. It returns false if the
// user is already protected (or a report is already pending).
func (r *policeRegistry) reserve(user string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[user]; ok {
		return false
	}
	r.m[user] = [2]int{0, 0}
	return true
}

// set stores the active protection bonuses for user.
func (r *policeRegistry) set(user string, ohayou, cat int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[user] = [2]int{ohayou, cat}
}

// bonus returns the current bonuses for user, and whether they are protected.
func (r *policeRegistry) bonus(user string) (ohayou, cat int, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.m[user]
	return v[0], v[1], ok
}

// decay subtracts the given amounts. When the ohayou bonus reaches zero the
// user is removed. It reports whether the user is still protected afterward.
func (r *policeRegistry) decay(user string, ohayouDec, catDec int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.m[user]
	if !ok {
		return false
	}
	v[0] -= ohayouDec
	v[1] -= catDec
	if v[0] <= 0 {
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

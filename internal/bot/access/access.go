// Package access decides who may do what, by nick and host.
package access

import "strings"

// Set is a case-insensitive set of nicks, hosts or channel names.
type Set map[string]bool

// NewSet lowercases and trims values, dropping the empty ones.
func NewSet(values []string) Set {
	set := make(Set, len(values))
	for _, v := range values {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
			set[v] = true
		}
	}
	return set
}

func (s Set) Has(v string) bool {
	return s[strings.ToLower(strings.TrimSpace(v))]
}

// Rule is which of a nick and a host an entry must match on. Requiring both is
// the safer bar, since a nick alone is trivially borrowed.
type Rule struct {
	ByNick bool
	ByHost bool
}

// Match is what Find made of a sender.
type Match struct {
	// Key is the entry that answered, lowercased.
	Key string
	// WantHost is the host that entry requires, for a caller logging a refusal.
	WantHost string
	// Listed says an entry exists for this nick, whether or not it matched.
	Listed bool
	// OK says the entry matched in full.
	OK bool
}

// Find looks (nick, host) up in hosts, which maps a lowercased nick to the host
// it must come from. A rule matching on neither field never matches: an empty
// rule is a closed door, not an open one.
func (r Rule) Find(hosts map[string]string, nick, host string) Match {
	switch {
	case !r.ByNick && !r.ByHost:
		return Match{}
	case !r.ByNick:
		for key, want := range hosts {
			if want != "" && strings.EqualFold(want, host) {
				return Match{Key: key, WantHost: want, Listed: true, OK: true}
			}
		}
		return Match{}
	}

	key := strings.ToLower(nick)
	want, listed := hosts[key]
	if !listed {
		return Match{}
	}
	if r.ByHost && !strings.EqualFold(want, host) {
		return Match{Key: key, WantHost: want, Listed: true}
	}
	return Match{Key: key, WantHost: want, Listed: true, OK: true}
}

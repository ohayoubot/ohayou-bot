package access

import "testing"

func TestSetFoldsCaseAndSpace(t *testing.T) {
	s := NewSet([]string{"  #Quiet ", "SPAMMER", "", "   "})

	for _, in := range []string{"#quiet", "#QUIET", " #Quiet ", "spammer"} {
		if !s.Has(in) {
			t.Errorf("Has(%q) = false, want true", in)
		}
	}
	if s.Has("#loud") {
		t.Error("Has on something never listed = true")
	}
	if len(s) != 2 {
		t.Errorf("%d entries, want the blanks dropped", len(s))
	}
}

func TestSetOnNothing(t *testing.T) {
	if NewSet(nil).Has("anyone") {
		t.Error("an empty set matched")
	}
}

var (
	byBoth = Rule{ByNick: true, ByHost: true}
	byNick = Rule{ByNick: true}
	byHost = Rule{ByHost: true}
)

var hosts = map[string]string{"admin": "trusted.host", "other": "elsewhere.net"}

func TestBothMustMatch(t *testing.T) {
	who := byBoth.Find(hosts, "admin", "trusted.host")
	if !who.OK || who.Key != "admin" {
		t.Errorf("%+v, want a full match on admin", who)
	}
}

func TestNickIsFoldedButHostMustStillMatch(t *testing.T) {
	if who := byBoth.Find(hosts, "AdMiN", "trusted.host"); !who.OK {
		t.Errorf("%+v, want the nick matched case-insensitively", who)
	}
	if who := byBoth.Find(hosts, "admin", "somewhere.else"); who.OK {
		t.Errorf("%+v, want a refusal from the wrong host", who)
	}
}

// A refusal has to say who was listed, or an operator locked out by a changed
// vhost gets silence instead of a log line naming the host it wanted.
func TestAWrongHostIsStillListed(t *testing.T) {
	who := byBoth.Find(hosts, "admin", "somewhere.else")
	if !who.Listed {
		t.Error("Listed = false, want the entry reported so the refusal can be logged")
	}
	if who.WantHost != "trusted.host" {
		t.Errorf("WantHost = %q, want the host the entry requires", who.WantHost)
	}
}

// Hosts are case-insensitive, so a server spelling one differently must not
// lock out the person it belongs to.
func TestHostsFoldCase(t *testing.T) {
	if who := byBoth.Find(hosts, "admin", "TRUSTED.HOST"); !who.OK {
		t.Errorf("%+v, want the host matched case-insensitively", who)
	}
}

func TestUnlistedNickNeverMatches(t *testing.T) {
	who := byBoth.Find(hosts, "stranger", "trusted.host")
	if who.OK || who.Listed {
		t.Errorf("%+v, want nothing for a nick that is not listed", who)
	}
}

func TestByNickIgnoresTheHost(t *testing.T) {
	if who := byNick.Find(hosts, "admin", "anywhere.at.all"); !who.OK {
		t.Errorf("%+v, want a match on the nick alone", who)
	}
}

func TestByHostIgnoresTheNick(t *testing.T) {
	who := byHost.Find(hosts, "someone-else-entirely", "trusted.host")
	if !who.OK || who.Key != "admin" {
		t.Errorf("%+v, want the entry found by host alone", who)
	}
	if who := byHost.Find(hosts, "admin", "unknown.host"); who.OK {
		t.Errorf("%+v, want no match for an unlisted host", who)
	}
}

// An entry with no host cannot be matched by host, or every unlisted sender
// would satisfy it.
func TestByHostSkipsEntriesWithoutOne(t *testing.T) {
	if who := byHost.Find(map[string]string{"nohost": ""}, "anyone", ""); who.OK {
		t.Errorf("%+v, want an empty host to match nothing", who)
	}
}

// An empty rule is a closed door: a config asking to match on neither field
// must not privilege everybody.
func TestAnEmptyRuleMatchesNobody(t *testing.T) {
	if who := (Rule{}).Find(hosts, "admin", "trusted.host"); who.OK {
		t.Errorf("%+v, want a rule matching on nothing to match nobody", who)
	}
}

package web

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The worker signs and verifies the same bytes in javascript. If this changes,
// both sides change together: an old link would stop verifying, and a new one
// would be refused. The same vector is asserted in web/tools/hmac.test.mjs.
func TestGrantByteLayoutIsPinned(t *testing.T) {
	const (
		secret = "0123456789abcdef0123456789abcdef"
		want   = "v1.eyJhIjoic29tZW9uZSIsIm4iOiJzb21lb25lXyIsImMiOlsiI2NoYW4iLCIjb3RoZXIiXSwiZSI6MTc1NDI1MDMwMCwiaiI6Ik1ERXlNelExTmpjNE9XRmlZMlJsWmcifQ.JCgzlwyJ-xGgERsMw9DEWv91owB4GLvuEUgoGUB8Wpc"
	)

	got, err := NewMinter(secret).sign(payload{
		A: "someone",
		N: "someone_",
		C: []string{"#chan", "#other"},
		E: 1754250300,
		J: "MDEyMzQ1Njc4OWFiY2RlZg",
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if got != want {
		t.Errorf("sign =\n  %s\nwant\n  %s", got, want)
	}
}

func TestGrantPayloadIsReadableJSON(t *testing.T) {
	token, err := NewMinter("s").sign(payload{
		A: "acct", N: "nick", C: []string{"#chan"}, E: 42, J: "id",
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	if parts[0] != "v1" {
		t.Errorf("version = %q, want v1", parts[0])
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("payload is not unpadded base64url: %v", err)
	}
	var back payload
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("payload is not json: %v", err)
	}
	if back.A != "acct" || back.N != "nick" || back.E != 42 || back.J != "id" {
		t.Errorf("round trip lost something: %+v", back)
	}
}

// Go escapes <, > and & in json by default; JSON.stringify does not. A channel
// name carrying one would sign different bytes on each side.
func TestGrantDoesNotEscapeHTML(t *testing.T) {
	body, err := encodePayload(payload{A: "a", N: "n", C: []string{"#a&b"}, E: 1, J: "j"})
	if err != nil {
		t.Fatalf("encodePayload: %v", err)
	}
	if !strings.Contains(string(body), "#a&b") {
		t.Errorf("payload = %s, want a literal ampersand", body)
	}
}

func TestGrantSignatureCoversThePayload(t *testing.T) {
	base := payload{A: "acct", N: "nick", C: []string{"#chan"}, E: 42, J: "id"}

	first, err := NewMinter("s").sign(base)
	if err != nil {
		t.Fatal(err)
	}

	widened := base
	widened.C = []string{"#chan", "#secret"}
	second, err := NewMinter("s").sign(widened)
	if err != nil {
		t.Fatal(err)
	}

	if sig(first) == sig(second) {
		t.Error("adding a channel did not change the signature")
	}

	other, err := NewMinter("different secret").sign(base)
	if err != nil {
		t.Fatal(err)
	}
	if sig(first) == sig(other) {
		t.Error("a different secret produced the same signature")
	}
}

func TestGrantIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id, err := newID()
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate grant id %q", id)
		}
		seen[id] = true
	}
}

func TestMintSetsTheExpiryFromTheClock(t *testing.T) {
	m := NewMinter("s")
	m.Now = func() time.Time { return time.Unix(1754250000, 0) }

	token, id, err := m.Mint(Grant{
		Account: "acct", Nick: "nick", Channels: []string{"#chan"},
		TTL: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if id == "" {
		t.Error("Mint returned no id to log a refusal against")
	}

	got := decode(t, token)
	if got.E != 1754250300 {
		t.Errorf("expiry = %d, want 1754250300", got.E)
	}
	if got.J != id {
		t.Errorf("payload id %q is not the returned id %q", got.J, id)
	}
}

// A bot with no site configured hands out no links, rather than links nothing
// can verify.
func TestNewMinterIsNilWithoutASecret(t *testing.T) {
	if NewMinter("") != nil {
		t.Error("a minter was returned for an empty secret")
	}
}

// The worker refuses all of these anyway. Refusing here means the reason
// reaches a log instead of a user reaching a dead link.
func TestMintRefusesWhatTheWorkerWould(t *testing.T) {
	tooMany := make([]string, MaxChannels+1)
	for i := range tooMany {
		tooMany[i] = "#c"
	}

	for name, g := range map[string]Grant{
		"no account":  {Nick: "n", Channels: []string{"#c"}, TTL: time.Minute},
		"no nick":     {Account: "a", Channels: []string{"#c"}, TTL: time.Minute},
		"no channels": {Account: "a", Nick: "n", TTL: time.Minute},
		"too many":    {Account: "a", Nick: "n", Channels: tooMany, TTL: time.Minute},
		"no ttl":      {Account: "a", Nick: "n", Channels: []string{"#c"}},
		"ttl too far": {Account: "a", Nick: "n", Channels: []string{"#c"}, TTL: MaxTTL + time.Second},
	} {
		if _, _, err := NewMinter("s").Mint(g); err == nil {
			t.Errorf("%s: minted anyway", name)
		}
	}

	ok := Grant{Account: "a", Nick: "n", Channels: []string{"#c"}, TTL: time.Minute}
	if _, _, err := NewMinter("s").Mint(ok); err != nil {
		t.Errorf("a good grant was refused: %v", err)
	}
}

func decode(t *testing.T, token string) payload {
	t.Helper()
	parts := strings.Split(token, ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	return p
}

func sig(token string) string {
	parts := strings.Split(token, ".")
	return parts[len(parts)-1]
}

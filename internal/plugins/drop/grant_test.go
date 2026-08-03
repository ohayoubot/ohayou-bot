package drop

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The worker signs and verifies the same bytes in javascript. If this changes,
// both sides change together: an old link would stop verifying, and a new one
// would be refused. The same vector is asserted in tools/hmac.test.mjs.
func TestGrantByteLayoutIsPinned(t *testing.T) {
	const (
		secret = "0123456789abcdef0123456789abcdef"
		want   = "v1.eyJhIjoic29tZW9uZSIsIm4iOiJzb21lb25lXyIsImMiOlsiI2NoYW4iLCIjb3RoZXIiXSwiZSI6MTc1NDI1MDMwMCwiaiI6Ik1ERXlNelExTmpjNE9XRmlZMlJsWmcifQ.JCgzlwyJ-xGgERsMw9DEWv91owB4GLvuEUgoGUB8Wpc"
	)

	got, err := mint(secret, grant{
		A: "someone",
		N: "someone_",
		C: []string{"#chan", "#other"},
		E: 1754250300,
		J: "MDEyMzQ1Njc4OWFiY2RlZg",
	})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if got != want {
		t.Errorf("mint =\n  %s\nwant\n  %s", got, want)
	}
}

func TestGrantPayloadIsReadableJSON(t *testing.T) {
	token, err := mint("s", grant{
		A: "acct", N: "nick", C: []string{"#chan"}, E: 42, J: "id",
	})
	if err != nil {
		t.Fatalf("mint: %v", err)
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
	var back grant
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
	body, err := encodePayload(grant{A: "a", N: "n", C: []string{"#a&b"}, E: 1, J: "j"})
	if err != nil {
		t.Fatalf("encodePayload: %v", err)
	}
	if !strings.Contains(string(body), "#a&b") {
		t.Errorf("payload = %s, want a literal ampersand", body)
	}
}

func TestGrantSignatureCoversThePayload(t *testing.T) {
	base := grant{A: "acct", N: "nick", C: []string{"#chan"}, E: 42, J: "id"}

	first, err := mint("s", base)
	if err != nil {
		t.Fatal(err)
	}

	widened := base
	widened.C = []string{"#chan", "#secret"}
	second, err := mint("s", widened)
	if err != nil {
		t.Fatal(err)
	}

	if sig(first) == sig(second) {
		t.Error("adding a channel did not change the signature")
	}

	other, err := mint("different secret", base)
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
		id, err := newJTI()
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate grant id %q", id)
		}
		seen[id] = true
	}
}

func TestExpiryIsUnixSeconds(t *testing.T) {
	now := time.Unix(1754250000, 0)
	if got := expiry(now, 5*time.Minute); got != 1754250300 {
		t.Errorf("expiry = %d, want 1754250300", got)
	}
}

func sig(token string) string {
	parts := strings.Split(token, ".")
	return parts[len(parts)-1]
}

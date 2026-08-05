package web

import (
	"strings"
	"testing"
	"time"
)

// The example every pinned assertion is built from, here and in
// web/tools/hmac.test.mjs.
const (
	testSecret  = "0123456789abcdef0123456789abcdef"
	testExpiry  = 1754250300
	testToken   = "AgFoj7w8MDEyMzQ1NjcHc29tZW9uZQhzb21lb25lXwIFI2NoYW4GI290aGVy.zx4o3X9YTT1z-DbcIay-qw"
	testAccount = "someone"
	testNick    = "someone_"
)

var testID = []byte("01234567")

func testPayload() payload {
	return payload{
		Scopes:   ScopeDrop,
		Expiry:   testExpiry,
		ID:       testID,
		Account:  testAccount,
		Nick:     testNick,
		Channels: []string{"#chan", "#other"},
	}
}

// The worker signs and verifies the same bytes in javascript. If this changes,
// both sides change together: an old link would stop verifying, and a new one
// would be refused. The same vector is asserted in web/tools/hmac.test.mjs.
func TestGrantByteLayoutIsPinned(t *testing.T) {
	got, err := NewMinter(testSecret).sign(testPayload())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if got != testToken {
		t.Errorf("sign =\n  %s\nwant\n  %s", got, testToken)
	}
}

// The link is read off a terminal irc client. json in base64 spent 177
// characters before the url was even in front of it.
func TestTokenStaysShortEnoughToClick(t *testing.T) {
	const budget = 96
	if len(testToken) > budget {
		t.Errorf("a two-channel token is %d characters, over the %d budget",
			len(testToken), budget)
	}
}

func TestGrantRoundTrips(t *testing.T) {
	m := NewMinter(testSecret)
	m.Now = func() time.Time { return time.Unix(testExpiry-60, 0) }

	got, id, err := m.Verify(testToken)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Account != testAccount || got.Nick != testNick {
		t.Errorf("account/nick = %q/%q", got.Account, got.Nick)
	}
	if strings.Join(got.Channels, ",") != "#chan,#other" {
		t.Errorf("channels = %v", got.Channels)
	}
	if got.Scopes != ScopeDrop {
		t.Errorf("scopes = %d, want %d", got.Scopes, ScopeDrop)
	}
	if id != b64(testID) {
		t.Errorf("id = %q, want %q", id, b64(testID))
	}
}

// A grant minted for one plugin must not be spendable on another, which is the
// whole reason scopes are inside the signature.
func TestScopesAreSignedAndDistinct(t *testing.T) {
	m := NewMinter(testSecret)

	p := testPayload()
	one, err := m.sign(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Scopes = ScopeDrop | ScopeOhayou
	both, err := m.sign(p)
	if err != nil {
		t.Fatal(err)
	}
	if tagOf(one) == tagOf(both) {
		t.Error("widening the scopes did not change the signature")
	}

	m.Now = func() time.Time { return time.Unix(testExpiry-60, 0) }
	got, _, err := m.Verify(both)
	if err != nil {
		t.Fatal(err)
	}
	if got.Scopes&ScopeOhayou == 0 || got.Scopes&ScopeDrop == 0 {
		t.Errorf("scopes = %d, want both bits", got.Scopes)
	}
}

func TestVerifyRefusesATamperedToken(t *testing.T) {
	m := NewMinter(testSecret)
	m.Now = func() time.Time { return time.Unix(testExpiry-60, 0) }

	body, tag, _ := strings.Cut(testToken, ".")
	for name, token := range map[string]string{
		"another secret": mustSign(t, NewMinter("different secret"), testPayload()),
		"flipped body":   flip(body) + "." + tag,
		"flipped tag":    body + "." + flip(tag),
		"no tag":         body,
		"two dots":       body + "." + tag + "." + tag,
		"empty":          "",
	} {
		if _, _, err := m.Verify(token); err == nil {
			t.Errorf("%s: verified anyway", name)
		}
	}
}

func TestVerifyRefusesAnExpiredGrant(t *testing.T) {
	m := NewMinter(testSecret)

	m.Now = func() time.Time { return time.Unix(testExpiry, 0) }
	if _, _, err := m.Verify(testToken); err == nil {
		t.Error("a grant verified on the second it expired")
	}

	// The worker bounds how far ahead a grant may reach, so a bot with a broken
	// clock cannot mint a long-lived one.
	m.Now = func() time.Time { return time.Unix(testExpiry, 0).Add(-MaxTTL - time.Second) }
	if _, _, err := m.Verify(testToken); err == nil {
		t.Error("a grant reaching past MaxTTL verified")
	}
}

func TestGrantIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id, err := newID()
		if err != nil {
			t.Fatal(err)
		}
		if seen[b64(id)] {
			t.Fatalf("duplicate grant id %q", b64(id))
		}
		seen[b64(id)] = true
	}
}

func TestMintSetsTheExpiryFromTheClock(t *testing.T) {
	m := NewMinter(testSecret)
	m.Now = func() time.Time { return time.Unix(testExpiry-300, 0) }

	token, id, err := m.Mint(Grant{
		Account: testAccount, Nick: testNick, Channels: []string{"#chan"},
		Scopes: ScopeDrop, TTL: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if id == "" {
		t.Error("Mint returned no id to log a refusal against")
	}

	body, _, _ := splitToken(token)
	p, err := decodePayload(body)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Expiry != testExpiry {
		t.Errorf("expiry = %d, want %d", p.Expiry, testExpiry)
	}
	if b64(p.ID) != id {
		t.Errorf("payload id %q is not the returned id %q", b64(p.ID), id)
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
		"no account":  {Nick: "n", Channels: []string{"#c"}, Scopes: ScopeDrop, TTL: time.Minute},
		"no nick":     {Account: "a", Channels: []string{"#c"}, Scopes: ScopeDrop, TTL: time.Minute},
		"no scopes":   {Account: "a", Nick: "n", Channels: []string{"#c"}, TTL: time.Minute},
		"no channels": {Account: "a", Nick: "n", Scopes: ScopeDrop, TTL: time.Minute},
		"too many":    {Account: "a", Nick: "n", Channels: tooMany, Scopes: ScopeDrop, TTL: time.Minute},
		"no ttl":      {Account: "a", Nick: "n", Channels: []string{"#c"}, Scopes: ScopeDrop},
		"ttl too far": {Account: "a", Nick: "n", Channels: []string{"#c"}, Scopes: ScopeDrop, TTL: MaxTTL + time.Second},
		"long name":   {Account: strings.Repeat("a", maxName+1), Nick: "n", Channels: []string{"#c"}, Scopes: ScopeDrop, TTL: time.Minute},
	} {
		if _, _, err := NewMinter("s").Mint(g); err == nil {
			t.Errorf("%s: minted anyway", name)
		}
	}

	ok := Grant{Account: "a", Nick: "n", Channels: []string{"#c"}, Scopes: ScopeDrop, TTL: time.Minute}
	if _, _, err := NewMinter("s").Mint(ok); err != nil {
		t.Errorf("a good grant was refused: %v", err)
	}
}

// decodePayload runs on bytes whose tag checked out, but it still must not read
// past the end of a truncated one.
func TestDecodeRefusesAShortPayload(t *testing.T) {
	body, err := encodePayload(testPayload())
	if err != nil {
		t.Fatal(err)
	}
	for n := range len(body) {
		if _, err := decodePayload(body[:n]); err == nil {
			t.Errorf("%d of %d bytes decoded anyway", n, len(body))
		}
	}
	if _, err := decodePayload(append(body, 0)); err == nil {
		t.Error("a trailing byte decoded anyway")
	}
}

func TestDecodeRefusesAnotherVersion(t *testing.T) {
	body, err := encodePayload(testPayload())
	if err != nil {
		t.Fatal(err)
	}
	body[0] = grantVersion + 1
	if _, err := decodePayload(body); err == nil {
		t.Error("a grant from another version decoded")
	}
}

func mustSign(t *testing.T, m *Minter, p payload) string {
	t.Helper()
	token, err := m.sign(p)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// flip changes one base64url character to another, so the bytes differ but the
// encoding still parses.
func flip(s string) string {
	if s == "" {
		return "A"
	}
	last := s[len(s)-1]
	if last == 'A' {
		return s[:len(s)-1] + "B"
	}
	return s[:len(s)-1] + "A"
}

func tagOf(token string) string {
	_, tag, _ := strings.Cut(token, ".")
	return tag
}

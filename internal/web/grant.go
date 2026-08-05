package web

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// The grant is a signed link the user takes to the site, which trades it for a
// cookie:
//
//	v1.<payload>.<signature>
//
// The worker verifies this in web/lib/hmac.js. Neither side may change without
// the other; grant_test.go pins the exact bytes both must produce.
const grantVersion = "v1"

// MaxChannels matches the ceiling the worker puts on a grant's channel list.
const MaxChannels = 32

// MaxTTL matches the furthest ahead the worker will accept an expiry.
const MaxTTL = 900 * time.Second

// payload is what gets signed. Field order is the json output order, which the
// pinned vector depends on. Verification hashes the payload as it arrives, so a
// reordering would still interoperate, but the vector would need regenerating.
type payload struct {
	A string   `json:"a"` // services account, the identity everything keys on
	N string   `json:"n"` // nick at mint time, display only
	C []string `json:"c"` // channels this grant may post to
	E int64    `json:"e"` // expiry, unix seconds
	J string   `json:"j"` // unique id, redeemable once
}

// validate refuses what the worker would refuse anyway, so a bad grant fails
// here, where the reason is still in hand, rather than as a dead link.
func (g Grant) validate() error {
	switch {
	case g.Account == "":
		return fmt.Errorf("web: a grant needs an account")
	case g.Nick == "":
		return fmt.Errorf("web: a grant needs a nick")
	case len(g.Channels) == 0:
		return fmt.Errorf("web: a grant needs somewhere to post")
	case len(g.Channels) > MaxChannels:
		return fmt.Errorf("web: a grant carries at most %d channels", MaxChannels)
	case g.TTL <= 0 || g.TTL > MaxTTL:
		return fmt.Errorf("web: a grant lasts up to %s", MaxTTL)
	}
	return nil
}

// sign builds the token. The secret is keyed as its own utf-8 bytes, not
// decoded from the hex it is written as; the worker does the same.
func (m *Minter) sign(p payload) (string, error) {
	body, err := encodePayload(p)
	if err != nil {
		return "", err
	}

	signed := grantVersion + "." + b64(body)
	mac := hmac.New(sha256.New, []byte(m.secret))
	mac.Write([]byte(signed))
	return signed + "." + b64(mac.Sum(nil)), nil
}

// encodePayload is json.Marshal with html escaping off. Marshal would write
// "&" for an "&" in a channel name where JSON.stringify writes it plainly,
// and the two sides must agree on every byte.
func encodePayload(p payload) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(p); err != nil {
		return nil, fmt.Errorf("web: encoding grant: %w", err)
	}
	// Encode appends a newline that Marshal does not.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func newID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("web: generating a grant id: %w", err)
	}
	return b64(raw), nil
}

func b64(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

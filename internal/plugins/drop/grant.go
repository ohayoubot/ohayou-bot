package drop

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

// The grant is a signed link the user takes to the upload site, which trades it
// for a cookie:
//
//	v1.<payload>.<signature>
//
// The worker verifies this in lib/hmac.js. Neither side may change without the
// other; grant_test.go pins the exact bytes both must produce.
const grantVersion = "v1"

// maxChannels matches the ceiling the worker puts on a grant's channel list.
const maxChannels = 32

// grant is the payload. Field order is the json output order, which the pinned
// vector depends on. Verification hashes the payload as it arrives, so a
// reordering would still interoperate, but the vector would need regenerating.
type grant struct {
	A string   `json:"a"` // services account, the identity everything keys on
	N string   `json:"n"` // nick at mint time, display only
	C []string `json:"c"` // channels this upload may be posted to
	E int64    `json:"e"` // expiry, unix seconds
	J string   `json:"j"` // unique id, redeemable once
}

// mint signs a grant. The secret is keyed as its own utf-8 bytes, not decoded
// from the hex it is written as; the worker does the same.
func mint(secret string, payload grant) (string, error) {
	body, err := encodePayload(payload)
	if err != nil {
		return "", err
	}

	signed := grantVersion + "." + b64(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	return signed + "." + b64(mac.Sum(nil)), nil
}

// encodePayload is json.Marshal with html escaping off. Marshal would write
// "&" for an "&" in a channel name where JSON.stringify writes it plainly,
// and the two sides must agree on every byte.
func encodePayload(payload grant) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return nil, fmt.Errorf("drop: encoding grant: %w", err)
	}
	// Encode appends a newline that Marshal does not.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func newJTI() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("drop: generating a grant id: %w", err)
	}
	return b64(raw), nil
}

func expiry(now time.Time, ttl time.Duration) int64 {
	return now.Add(ttl).Unix()
}

func b64(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"time"
)

// The grant is a signed link the user takes to the site, which trades it for a
// cookie:
//
//	<payload>.<tag>
//
// both base64url without padding. Verified by web/lib/hmac.js; neither side may
// change without the other, and grant_test.go pins the same bytes hmac.test.mjs
// does.
//
// Packed rather than json because the link is read off a terminal irc client,
// where a url that wraps is one nobody can click:
//
//	0       version
//	1       scopes, a bitmask
//	2..5    expiry, unix seconds, big endian
//	6..13   id, 8 random bytes
//	14..    account, nick, then a count and that many channels, each a length
//	        byte followed by its utf-8 bytes
const grantVersion = 2

// MaxChannels matches the ceiling the worker puts on a grant's channel list.
const MaxChannels = 32

// MaxTTL matches the furthest ahead the worker will accept an expiry.
const MaxTTL = 900 * time.Second

// maxName bounds an account, nick or channel. A length is one byte, and the
// worker holds every name to 64 anyway.
const maxName = 64

// idBytes is the id the worker records so a link is redeemable once. 64 bits is
// past collision for something that lives fifteen minutes.
const idBytes = 8

// tagBytes truncates the HMAC. 128 bits is not forgeable, and the full 256 cost
// 43 characters of a url somebody has to click.
const tagBytes = 16

// Scope is what a grant lets its holder do on the site. The bit positions are
// wire format: web/lib/hmac.js lists the same ones, and grant_test.go pins them.
type Scope uint8

const (
	// ScopeDrop uploads images and posts them to the grant's channels.
	ScopeDrop Scope = 1 << 0
	// ScopeOhayou reads the holder's own territory in full.
	ScopeOhayou Scope = 1 << 1
)

// payload is what gets signed.
type payload struct {
	Scopes   Scope
	Expiry   int64
	ID       []byte
	Account  string
	Nick     string
	Channels []string
}

// validate refuses what the worker would, so the reason reaches a log rather
// than the user reaching a dead link.
func (g Grant) validate() error {
	switch {
	case g.Account == "":
		return fmt.Errorf("web: a grant needs an account")
	case g.Nick == "":
		return fmt.Errorf("web: a grant needs a nick")
	case g.Scopes == 0:
		return fmt.Errorf("web: a grant that permits nothing is not worth minting")
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
	return b64(body) + "." + b64(m.tag(body)), nil
}

// Verify checks a token against the secret and the clock. The worker is what
// verifies in production; this exists so the same tests exercise both
// directions of the format.
func (m *Minter) Verify(token string) (Grant, string, error) {
	body, tag, ok := splitToken(token)
	if !ok {
		return Grant{}, "", fmt.Errorf("web: malformed grant")
	}
	if subtle.ConstantTimeCompare(tag, m.tag(body)) != 1 {
		return Grant{}, "", fmt.Errorf("web: bad signature")
	}

	p, err := decodePayload(body)
	if err != nil {
		return Grant{}, "", err
	}
	now := m.Now()
	if p.Expiry <= now.Unix() {
		return Grant{}, "", fmt.Errorf("web: expired grant")
	}
	if p.Expiry > now.Add(MaxTTL).Unix() {
		return Grant{}, "", fmt.Errorf("web: grant expires too far out")
	}

	return Grant{
		Account:  p.Account,
		Nick:     p.Nick,
		Channels: p.Channels,
		Scopes:   p.Scopes,
		TTL:      time.Duration(p.Expiry-now.Unix()) * time.Second,
	}, b64(p.ID), nil
}

func (m *Minter) tag(body []byte) []byte {
	mac := hmac.New(sha256.New, []byte(m.secret))
	mac.Write(body)
	return mac.Sum(nil)[:tagBytes]
}

func splitToken(token string) (body, tag []byte, ok bool) {
	// Bounds the work before any parsing.
	if len(token) > 1024 {
		return nil, nil, false
	}
	dot := -1
	for i := range len(token) {
		if token[i] == '.' {
			if dot >= 0 {
				return nil, nil, false
			}
			dot = i
		}
	}
	if dot < 0 {
		return nil, nil, false
	}
	body, err := base64.RawURLEncoding.DecodeString(token[:dot])
	if err != nil {
		return nil, nil, false
	}
	tag, err = base64.RawURLEncoding.DecodeString(token[dot+1:])
	if err != nil || len(tag) != tagBytes {
		return nil, nil, false
	}
	return body, tag, true
}

func encodePayload(p payload) ([]byte, error) {
	body := make([]byte, 0, 64)
	body = append(body, grantVersion, byte(p.Scopes))
	if p.Expiry < 0 || p.Expiry > int64(^uint32(0)) {
		return nil, fmt.Errorf("web: expiry %d is not a uint32 unix time", p.Expiry)
	}
	body = binary.BigEndian.AppendUint32(body, uint32(p.Expiry))
	if len(p.ID) != idBytes {
		return nil, fmt.Errorf("web: a grant id is %d bytes", idBytes)
	}
	body = append(body, p.ID...)

	var err error
	if body, err = appendName(body, p.Account); err != nil {
		return nil, err
	}
	if body, err = appendName(body, p.Nick); err != nil {
		return nil, err
	}
	if len(p.Channels) > MaxChannels {
		return nil, fmt.Errorf("web: a grant carries at most %d channels", MaxChannels)
	}
	body = append(body, byte(len(p.Channels)))
	for _, name := range p.Channels {
		if body, err = appendName(body, name); err != nil {
			return nil, err
		}
	}
	return body, nil
}

func appendName(body []byte, name string) ([]byte, error) {
	if name == "" || len(name) > maxName {
		return nil, fmt.Errorf("web: %q is not between 1 and %d bytes", name, maxName)
	}
	return append(append(body, byte(len(name))), name...), nil
}

// decodePayload runs on bytes whose tag checked out, so a failure is this side
// disagreeing with itself. It still refuses to read past the end.
func decodePayload(body []byte) (payload, error) {
	r := reader{body: body}
	version := r.byte()
	scopes := r.byte()
	expiry := r.uint32()
	id := r.take(idBytes)
	account := r.name()
	nick := r.name()

	count := int(r.byte())
	if r.err == nil && count > MaxChannels {
		r.err = fmt.Errorf("web: %d channels, more than the %d allowed", count, MaxChannels)
	}
	channels := make([]string, 0, min(count, MaxChannels))
	for range count {
		if r.err != nil {
			break
		}
		channels = append(channels, r.name())
	}

	if r.err != nil {
		return payload{}, r.err
	}
	if version != grantVersion {
		return payload{}, fmt.Errorf("web: grant version %d, want %d", version, grantVersion)
	}
	if r.at != len(body) {
		return payload{}, fmt.Errorf("web: %d bytes left over", len(body)-r.at)
	}
	return payload{
		Scopes:   Scope(scopes),
		Expiry:   int64(expiry),
		ID:       id,
		Account:  account,
		Nick:     nick,
		Channels: channels,
	}, nil
}

// reader walks the payload, latching the first error so each step can be
// written without a check around it.
type reader struct {
	body []byte
	at   int
	err  error
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.at+n > len(r.body) {
		r.err = fmt.Errorf("web: grant ends early")
		return nil
	}
	out := r.body[r.at : r.at+n]
	r.at += n
	return out
}

func (r *reader) byte() byte {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

func (r *reader) uint32() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

func (r *reader) name() string {
	n := int(r.byte())
	if n == 0 && r.err == nil {
		r.err = fmt.Errorf("web: empty name in grant")
		return ""
	}
	return string(r.take(n))
}

func newID() ([]byte, error) {
	raw := make([]byte, idBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("web: generating a grant id: %w", err)
	}
	return raw, nil
}

func b64(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

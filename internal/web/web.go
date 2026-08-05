// Package web is the bot's side of the website: the signed links a plugin hands
// a user, and the projections a plugin publishes. The identity a grant carries
// is the bot's rather than any plugin's, which is why it lives here.
package web

import "time"

// Grant is what a plugin asks to hand a user.
type Grant struct {
	// Account is the services account, the identity everything else keys on.
	Account string
	// Nick is who they were at mint time, for display.
	Nick string
	// Channels bound where the session may post. The site cannot widen them.
	Channels []string
	// Scopes are what the holder may do. A grant minted for one plugin cannot be
	// spent on another.
	Scopes Scope
	// TTL is how long the link stays good. Anything past MaxTTL is refused by
	// the worker, so it is refused here first.
	TTL time.Duration
}

// Channels trims a list to what a grant can carry. Truncating rather than
// refusing: somebody in more than MaxChannels with the bot still gets a link.
func Channels(names []string) []string {
	if len(names) > MaxChannels {
		return names[:MaxChannels]
	}
	return names
}

// Minter signs grants with the secret the worker verifies them with.
type Minter struct {
	secret string
	// Now is the clock, so a test can pin an expiry.
	Now func() time.Time
}

// NewMinter returns nil when there is no secret to sign with.
func NewMinter(secret string) *Minter {
	if secret == "" {
		return nil
	}
	return &Minter{secret: secret, Now: time.Now}
}

// Mint signs g and returns the token and the grant's id. The worker records the
// id to make a grant redeemable once, so it is worth logging.
func (m *Minter) Mint(g Grant) (token, id string, err error) {
	if err := g.validate(); err != nil {
		return "", "", err
	}
	raw, err := newID()
	if err != nil {
		return "", "", err
	}
	token, err = m.sign(payload{
		Scopes:   g.Scopes,
		Expiry:   m.Now().Add(g.TTL).Unix(),
		ID:       raw,
		Account:  g.Account,
		Nick:     g.Nick,
		Channels: g.Channels,
	})
	if err != nil {
		return "", "", err
	}
	return token, b64(raw), nil
}

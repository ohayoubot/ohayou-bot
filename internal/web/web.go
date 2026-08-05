// Package web is the bot's side of the website: the signed links a plugin hands
// a user so the site knows who they are. It lives here rather than in the one
// plugin that mints them today, because the identity a grant carries is the
// bot's, not any plugin's.
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
	// TTL is how long the link stays good. Anything past MaxTTL is refused by
	// the worker, so it is refused here first.
	TTL time.Duration
}

// Minter signs grants with the secret the worker verifies them with.
type Minter struct {
	secret string
	// Now is the clock, so a test can pin an expiry.
	Now func() time.Time
}

// NewMinter returns a minter, or nil when there is no secret to sign with: a
// bot with no site configured hands out no links.
func NewMinter(secret string) *Minter {
	if secret == "" {
		return nil
	}
	return &Minter{secret: secret, Now: time.Now}
}

// Mint signs g and returns the link token and the grant's id. The id is what
// the worker records to make a grant redeemable once, so it is worth logging.
func (m *Minter) Mint(g Grant) (token, id string, err error) {
	if err := g.validate(); err != nil {
		return "", "", err
	}
	id, err = newID()
	if err != nil {
		return "", "", err
	}
	token, err = m.sign(payload{
		A: g.Account,
		N: g.Nick,
		C: g.Channels,
		E: m.Now().Add(g.TTL).Unix(),
		J: id,
	})
	if err != nil {
		return "", "", err
	}
	return token, id, nil
}

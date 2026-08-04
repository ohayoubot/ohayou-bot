package bot

import (
	"context"
	"strings"
)

// Verify asks the network whether nick is logged in to services. An error is
// not a "no": it means the lookup failed, and a caller gating on identification
// must refuse rather than tell an identified user they are not.
func (b *Bot) Verify(ctx context.Context, nick string) (bool, error) {
	who, err := b.WhoisInfo(ctx, nick)
	if err != nil {
		return false, err
	}
	return who.Identified(), nil
}

// Identify verifies nick and, when it checks out, remembers it until they
// change nick or disconnect.
func (b *Bot) Identify(ctx context.Context, nick string) (bool, error) {
	ok, err := b.Verify(ctx, nick)
	if err != nil || !ok {
		return false, err
	}

	b.identMu.Lock()
	b.identified[strings.ToLower(nick)] = true
	b.identMu.Unlock()
	return true, nil
}

// Identified reports whether nick proved itself earlier in this session. It
// asks the server nothing, so a nick that never identified is not identified.
func (b *Bot) Identified(nick string) bool {
	b.identMu.RLock()
	defer b.identMu.RUnlock()
	return b.identified[strings.ToLower(nick)]
}

// registerIdentity drops a nick's proof when they change nick or disconnect,
// which is as long as it is worth anything.
func (b *Bot) registerIdentity() {
	b.OnNick(func(e NickEvent) { b.forget(e.From, "nick change") })
	b.OnQuit(func(e QuitEvent) { b.forget(e.Nick, "quit") })
}

func (b *Bot) forget(nick, why string) {
	key := strings.ToLower(nick)

	b.identMu.Lock()
	_, had := b.identified[key]
	delete(b.identified, key)
	b.identMu.Unlock()

	if had {
		b.log.Debug("identity dropped", "nick", key, "reason", why)
	}
}

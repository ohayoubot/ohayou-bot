package ohayou

import (
	"context"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// Store is what the game needs of the store. It is declared here rather than in
// the store package so that adding a table for the game is not a change every
// other plugin has to know about. The sqlite store satisfies it; Register says
// so if whatever it was handed does not.
type Store interface {
	// SeedItems syncs the catalog from items.json into the items table.
	SeedItems(ctx context.Context, items []store.Item) (int, error)

	GetUser(ctx context.Context, nick string) (*store.User, error)
	CreateUser(ctx context.Context, nick string, ohayous int) error

	SaveOhayou(ctx context.Context, nick string, newTotal, addedCum int, last, dayStart time.Time) error
	SetAccount(ctx context.Context, nick, account string) error
	SetVisibility(ctx context.Context, nick string, v store.Visibility) error
	SetRegister(ctx context.Context, nick string, registered bool) error
	ResetLast(ctx context.Context, nick string) error
	SetStatus(ctx context.Context, nick, action string, active bool) error
	ResetAllStatus(ctx context.Context) error
	SetLastUsed(ctx context.Context, nick, item string, ts time.Time) error
	SetFortune(ctx context.Context, nick, fortune string) error

	// Items
	GetItem(ctx context.Context, name string) (*store.Item, error)
	ItemsByCategory(ctx context.Context, category string) ([]store.Item, error)
	Categories(ctx context.Context) ([]string, error)

	// Economy mutations.
	AddItem(ctx context.Context, nick string, item store.Item, amt int) error
	ConsumeItem(ctx context.Context, nick, item string) error
	AddCat(ctx context.Context, nick string, amt int) error
	RemoveCat(ctx context.Context, nick string, amt int) error
	AddOil(ctx context.Context, nick string, amt int) error
	AddMetals(ctx context.Context, nick string, yield map[string]int) error
	Equip(ctx context.Context, nick string, item store.Item) error
	Unequip(ctx context.Context, nick, equipCategory string) error
	InstallVault(ctx context.Context, nick string) error
	IncVaultLevel(ctx context.Context, nick string) error
	// VaultTransfer moves ohayous between a user's balance and their vault. Ensure negativity
	// invarients or returns an error
	VaultTransfer(ctx context.Context, nick string, ohayousDelta, vaultDelta int, last, dayStart time.Time) error
	Top(ctx context.Context, n int) ([]store.UserOhayous, error)

	// Players lists everyone who has ever ohayou'd, oldest hands first. They all
	// appear on the map; what their plot says about them is decided per user.
	Players(ctx context.Context) ([]string, error)

	// Build consumes metals (from user_metals), items (from user_items) and
	// ohayous, then grants outAmt of the output item. All in one transaction.
	Build(ctx context.Context, nick string, metalCost, itemCost map[string]int, ohayouCost int, output string, outAmt int) error

	// Steal outcomes.
	SaveSuccessSteal(ctx context.Context, thief, victim string, cat, ohy int) error
	SaveFailSteal(ctx context.Context, nick string, fine int, probation time.Time) error
}

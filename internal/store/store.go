// Package store is an interface over the store. Currently this is sqlite but could
// in theory be another one. Should be agnostic to store-specific things for this reason.
package store

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by lookups when no matching row exists.
var ErrNotFound = errors.New("store: not found")

// ErrInsufficient is returned by a conditional mutation whose guard failed,
// i.e., a negative balance
var ErrInsufficient = errors.New("store: insufficient or gated")

// Store is the full set of storage ops the game needs.
type Store interface {
	// Lifecycle.
	Init(ctx context.Context) error
	SeedItems(ctx context.Context, items []Item) (int, error)
	Close() error

	// Users.
	GetUser(ctx context.Context, nick string) (*User, error)
	CreateUser(ctx context.Context, nick string, ohayous int) error

	SaveOhayou(ctx context.Context, nick string, newTotal, addedCum int, last, dayStart time.Time) error
	SetRegister(ctx context.Context, nick string, registered bool) error
	ResetLast(ctx context.Context, nick string) error
	SetStatus(ctx context.Context, nick, action string, active bool) error
	ResetAllStatus(ctx context.Context) error
	SetLastUsed(ctx context.Context, nick, item string, ts time.Time) error
	SetFortune(ctx context.Context, nick, fortune string) error

	// Items
	GetItem(ctx context.Context, name string) (*Item, error)
	ItemsByCategory(ctx context.Context, category string) ([]Item, error)
	Categories(ctx context.Context) ([]string, error)

	// Economy mutations.
	AddItem(ctx context.Context, nick string, item Item, amt int) error
	ConsumeItem(ctx context.Context, nick, item string) error
	AddCat(ctx context.Context, nick string, amt int) error
	RemoveCat(ctx context.Context, nick string, amt int) error
	AddOil(ctx context.Context, nick string, amt int) error
	AddMetals(ctx context.Context, nick string, yield map[string]int) error
	Equip(ctx context.Context, nick string, item Item) error
	Unequip(ctx context.Context, nick, equipCategory string) error
	InstallVault(ctx context.Context, nick string) error
	IncVaultLevel(ctx context.Context, nick string) error
	// VaultTransfer moves ohayous between a user's balance and their vault. Ensure negativity
	// invarients or returns an error
	VaultTransfer(ctx context.Context, nick string, ohayousDelta, vaultDelta int, last, dayStart time.Time) error
	Top(ctx context.Context, n int) ([]UserOhayous, error)

	// Build consumes metals (from user_metals), items (from user_items) and
	// ohayous, then grants outAmt of the output item. All in one transaction.
	Build(ctx context.Context, nick string, metalCost, itemCost map[string]int, ohayouCost int, output string, outAmt int) error

	// Steal outcomes.
	SaveSuccessSteal(ctx context.Context, thief, victim string, cat, ohy int) error
	SaveFailSteal(ctx context.Context, nick string, fine int, probation time.Time) error
}

// Package store is an interface over the store. Currently this is sqlite but could
// in theory be another one. Should be agnostic to store-specific things for this reason.
package store

import (
	"context"
	"errors"
)

// ErrNotFound is returned by lookups when no matching row exists.
var ErrNotFound = errors.New("store: not found")

// ErrInsufficient is returned by a conditional mutation whose guard failed,
// i.e., a negative balance
var ErrInsufficient = errors.New("store: insufficient or gated")

// Store is what the bot itself needs
type Store interface {
	// Lifecycle.
	Init(ctx context.Context) error
	Close() error

	// Migrate applies a plugin's own schema. The SQL is expected to be written
	// so running it on every start is a no-op.
	Migrate(ctx context.Context, name, schema string) error

	// AddColumn adds a column to a table that has already shipped, and is a
	// no-op when it is already there. Migrate only creates what is missing, and
	// a numbered migration file would fail on a fresh database that got the
	// column from the schema.
	AddColumn(ctx context.Context, table, column, definition string) error

	// Bot state that belongs to no user. GetKV returns ErrNotFound when the key
	// has never been set.
	GetKV(ctx context.Context, key string) (string, error)
	SetKV(ctx context.Context, key, value string) error
	DeleteKV(ctx context.Context, key string) error

	// Work a plugin wants done later, kept across restarts.
	TaskStore
}

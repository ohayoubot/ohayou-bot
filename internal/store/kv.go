package store

import "context"

// KVStore is the part of a Store a namespace is carved out of.
type KVStore interface {
	GetKV(ctx context.Context, key string) (string, error)
	SetKV(ctx context.Context, key, value string) error
	DeleteKV(ctx context.Context, key string) error
}

// KV is a plugin's own corner of the key-value table. Every key is written
// under the plugin's name, so two plugins cannot tread on each other's state
// and neither has to remember to prefix anything.
type KV struct {
	store  KVStore
	prefix string
}

// Namespace scopes s to a plugin.
func Namespace(s KVStore, name string) *KV {
	return &KV{store: s, prefix: name + "."}
}

// Get returns ErrNotFound when the key has never been set.
func (k *KV) Get(ctx context.Context, key string) (string, error) {
	return k.store.GetKV(ctx, k.prefix+key)
}

func (k *KV) Set(ctx context.Context, key, value string) error {
	return k.store.SetKV(ctx, k.prefix+key, value)
}

func (k *KV) Delete(ctx context.Context, key string) error {
	return k.store.DeleteKV(ctx, k.prefix+key)
}

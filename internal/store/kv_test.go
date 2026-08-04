package store

import (
	"context"
	"testing"
)

// fakeKV is a store that records the keys it was handed.
type fakeKV map[string]string

func (f fakeKV) GetKV(ctx context.Context, key string) (string, error) {
	v, ok := f[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}
func (f fakeKV) SetKV(ctx context.Context, key, value string) error {
	f[key] = value
	return nil
}
func (f fakeKV) DeleteKV(ctx context.Context, key string) error {
	delete(f, key)
	return nil
}

func TestNamespacePrefixesKeys(t *testing.T) {
	backing := fakeKV{}
	kv := Namespace(backing, "drop")

	if err := kv.Set(context.Background(), "cursor", "12"); err != nil {
		t.Fatal(err)
	}
	if got, ok := backing["drop.cursor"]; !ok || got != "12" {
		t.Errorf("backing = %v, want the key written under the plugin's name", backing)
	}
}

func TestNamespacesCannotSeeEachOther(t *testing.T) {
	backing := fakeKV{}
	one := Namespace(backing, "one")
	two := Namespace(backing, "two")

	ctx := context.Background()
	if err := one.Set(ctx, "state", "mine"); err != nil {
		t.Fatal(err)
	}
	if _, err := two.Get(ctx, "state"); err != ErrNotFound {
		t.Errorf("err = %v, want one plugin's key invisible to another", err)
	}

	if err := two.Set(ctx, "state", "theirs"); err != nil {
		t.Fatal(err)
	}
	if got, _ := one.Get(ctx, "state"); got != "mine" {
		t.Errorf("one's value = %q, want it untouched by the other", got)
	}
}

func TestNamespaceRoundTripAndDelete(t *testing.T) {
	kv := Namespace(fakeKV{}, "p")
	ctx := context.Background()

	if _, err := kv.Get(ctx, "missing"); err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound for a key never set", err)
	}
	if err := kv.Set(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	if got, err := kv.Get(ctx, "k"); err != nil || got != "v" {
		t.Errorf("Get = %q, %v", got, err)
	}
	if err := kv.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Get(ctx, "k"); err != ErrNotFound {
		t.Errorf("err = %v, want the key gone", err)
	}
}

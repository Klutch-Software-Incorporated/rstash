package blob

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
)

// memStore is a minimal in-memory Store used for wrapper tests.
type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStore() *memStore {
	return &memStore{data: make(map[string][]byte)}
}

func keyFor(userID int64, path string) string {
	return string(rune(userID)) + path
}

func (m *memStore) Get(_ context.Context, userID int64, path string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.data[keyFor(userID, path)]
	if !ok {
		return nil, io.EOF
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (m *memStore) Put(_ context.Context, userID int64, path string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[keyFor(userID, path)] = append([]byte{}, data...)
	return nil
}

func (m *memStore) raw(userID int64, path string) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[keyFor(userID, path)]
}

func (m *memStore) Delete(_ context.Context, userID int64, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, keyFor(userID, path))
	return nil
}

func (m *memStore) DeleteTree(_ context.Context, _ int64, _ string) error { return nil }
func (m *memStore) Close() error                                          { return nil }

type fixedKey []byte

func (f fixedKey) CurrentKey() ([]byte, bool) {
	if len(f) != 32 {
		return nil, false
	}
	return f, true
}

type disabledKey struct{}

func (disabledKey) CurrentKey() ([]byte, bool) { return nil, false }

func TestEncryptedStore_RoundTrip(t *testing.T) {
	inner := newMemStore()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	store := Wrap(inner, fixedKey(key))
	ctx := context.Background()

	plaintext := []byte("hello world")
	if err := store.Put(ctx, 1, "/notes/foo", plaintext); err != nil {
		t.Fatal(err)
	}

	// Raw bytes in the underlying store must be encrypted (magic prefix + ciphertext).
	raw := inner.raw(1, "/notes/foo")
	if !bytes.HasPrefix(raw, encMagic) {
		t.Fatal("expected encMagic prefix on stored bytes")
	}
	if bytes.Contains(raw, plaintext) {
		t.Fatal("plaintext leaked into stored bytes")
	}

	rc, err := store.Get(ctx, 1, "/notes/foo")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestEncryptedStore_PassthroughWhenDisabled(t *testing.T) {
	inner := newMemStore()
	store := Wrap(inner, disabledKey{})
	ctx := context.Background()

	plaintext := []byte("hello world")
	if err := store.Put(ctx, 1, "/notes/foo", plaintext); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(inner.raw(1, "/notes/foo"), plaintext) {
		t.Fatal("expected passthrough write when encryption disabled")
	}

	rc, err := store.Get(ctx, 1, "/notes/foo")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("mismatch: got %q, want %q", got, plaintext)
	}
}

func TestEncryptedStore_LegacyPlaintextBlob(t *testing.T) {
	// Blobs written before encryption was enabled lack the magic prefix and
	// must be returned as-is.
	inner := newMemStore()
	ctx := context.Background()
	_ = inner.Put(ctx, 1, "/legacy", []byte("plain"))

	key := make([]byte, 32)
	store := Wrap(inner, fixedKey(key))
	rc, err := store.Get(ctx, 1, "/legacy")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != "plain" {
		t.Fatalf("legacy read failed: %q", got)
	}
}

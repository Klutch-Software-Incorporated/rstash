package blob

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// encMagic is a 4-byte prefix identifying encrypted blobs. Blobs written
// before encryption was enabled lack this prefix and are read as-is.
var encMagic = []byte{0x72, 0x73, 0x65, 0x31} // "rse1"

// encryptedStore wraps an underlying Store with AES-256-GCM encryption.
// Writes are encrypted when the KeyProvider returns a key; reads auto-detect
// via the encMagic prefix so mixed (encrypted + legacy plaintext) stores work.
type encryptedStore struct {
	inner Store
	keys  KeyProvider
}

// Wrap returns a Store that encrypts new writes using keys from the provider.
// When the provider returns enabled=false, the wrapper is a transparent
// passthrough (new blobs written unencrypted). Reads always honor the
// encMagic prefix so prior-encrypted blobs continue to decrypt even if
// encryption is later turned off during a key-rotation or downgrade.
func Wrap(inner Store, keys KeyProvider) Store {
	if keys == nil {
		return inner
	}
	return &encryptedStore{inner: inner, keys: keys}
}

func (s *encryptedStore) Get(ctx context.Context, userID int64, path string) (io.ReadCloser, error) {
	rc, err := s.inner.Get(ctx, userID, path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(data, encMagic) {
		// Legacy plaintext blob; return as-is.
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	key, enabled := s.keys.CurrentKey()
	if !enabled {
		return nil, ErrKeyRequired
	}
	plain, err := decrypt(key, data[len(encMagic):])
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(plain)), nil
}

func (s *encryptedStore) Put(ctx context.Context, userID int64, path string, data []byte) error {
	key, enabled := s.keys.CurrentKey()
	if !enabled {
		return s.inner.Put(ctx, userID, path, data)
	}
	cipherBytes, err := encrypt(key, data)
	if err != nil {
		return err
	}
	wrapped := append(append([]byte{}, encMagic...), cipherBytes...)
	return s.inner.Put(ctx, userID, path, wrapped)
}

func (s *encryptedStore) Delete(ctx context.Context, userID int64, path string) error {
	return s.inner.Delete(ctx, userID, path)
}

func (s *encryptedStore) DeleteTree(ctx context.Context, userID int64, folderPath string) error {
	return s.inner.DeleteTree(ctx, userID, folderPath)
}

func (s *encryptedStore) Close() error { return s.inner.Close() }

// Inner exposes the wrapped store. Used by the encrypt-existing migration.
func (s *encryptedStore) Inner() Store { return s.inner }

func encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	return append(nonce, sealed...), nil
}

func decrypt(key, body []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(body) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, sealed := body[:nonceSize], body[nonceSize:]
	return gcm.Open(nil, nonce, sealed, nil)
}

package blob

import (
	"encoding/base64"
	"errors"
	"os"
)

// KeyProvider returns the AES-256 encryption key used to wrap blobs.
// Implementations return (nil, false) to disable encryption; the wrapper
// store then behaves as a transparent passthrough.
//
// The interface is intentionally small so future KMS-backed providers
// (AWS KMS, Azure Key Vault, HashiCorp Vault) can be added without
// touching the call sites.
type KeyProvider interface {
	// CurrentKey returns the active encryption key. When enabled is false,
	// the wrapper skips encryption (new writes pass through; existing
	// encrypted blobs cannot be read).
	CurrentKey() (key []byte, enabled bool)
}

// EnvKeyProvider reads the key from RSTASH_BLOB_KEY (base64 of 32 bytes).
// Any invalid or missing value disables encryption.
type EnvKeyProvider struct{}

// CurrentKey implements KeyProvider.
func (EnvKeyProvider) CurrentKey() ([]byte, bool) {
	raw := os.Getenv("RSTASH_BLOB_KEY")
	if raw == "" {
		return nil, false
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, false
	}
	return key, true
}

// ErrKeyRequired is returned when a read encounters an encrypted blob but no
// key is available to decrypt it (misconfiguration or key loss).
var ErrKeyRequired = errors.New("encrypted blob read without a key")

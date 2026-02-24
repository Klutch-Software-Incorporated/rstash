package db

import (
	"crypto/rand"
	"encoding/hex"
)

// RandomHex returns a cryptographically random hex string of 2*n characters.
func RandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Package token mints and verifies secret edit tokens (capability URLs).
// Tokens are returned to the client once; only their sha256 hash is stored.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// Generate returns a 256-bit URL-safe random token (~43 chars, no padding).
func Generate() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Hash returns the sha256 digest of a token, stored at rest.
func Hash(tok string) []byte {
	sum := sha256.Sum256([]byte(tok))
	return sum[:]
}

// Equal reports whether presented matches the stored hash, in constant time.
func Equal(storedHash []byte, presented string) bool {
	return subtle.ConstantTimeCompare(storedHash, Hash(presented)) == 1
}

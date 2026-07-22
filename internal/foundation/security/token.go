package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// RandomToken returns a base64url-encoded random token of size bytes, defaulting to 32.
func RandomToken(size int) (string, error) {
	buf := make([]byte, 32)
	if size > 0 {
		buf = make([]byte, size)
	}
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken returns the base64url-encoded SHA-256 digest of raw for storage and lookup.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

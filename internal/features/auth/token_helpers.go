package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

func generateSessionToken() (raw, tokenHash string, err error) {
	raw, err = newRandomToken(32)
	if err != nil {
		return "", "", err
	}
	return raw, hashToken(raw), nil
}

func newRandomToken(size int) (string, error) {
	buf := make([]byte, 32)
	if size > 0 {
		buf = make([]byte, size)
	}
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

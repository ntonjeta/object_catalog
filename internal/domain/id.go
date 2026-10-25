package domain

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID returns a cryptographically random 128-bit identifier.
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

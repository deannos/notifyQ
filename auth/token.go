package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateAppToken produces a cryptographically random 32-byte hex token.
func GenerateAppToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

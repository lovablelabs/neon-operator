package utils

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const passwordAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GeneratePassword returns a cryptographically random alphanumeric password.
func GeneratePassword(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("password length must be positive")
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate password bytes: %w", err)
	}

	out := make([]byte, length)
	for i := range bytes {
		out[i] = passwordAlphabet[int(bytes[i])%len(passwordAlphabet)]
	}

	return string(out), nil
}

// PostgresMD5Hash returns the Postgres-compatible md5 hash without the `md5` prefix.
// Postgres computes this as md5(password + username).
func PostgresMD5Hash(username, password string) string {
	hash := md5.Sum([]byte(password + username))
	return hex.EncodeToString(hash[:])
}

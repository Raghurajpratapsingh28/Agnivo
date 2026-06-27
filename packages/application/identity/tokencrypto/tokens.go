// Package tokencrypto provides secure random token generation and hashing for
// refresh tokens, API keys, email verification, and invitations.
package tokencrypto

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/hashx"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
)

// Generate returns a URL-safe random token and its SHA-256 hex hash. Only the
// hash is stored; the raw token is shown to the user once.
func Generate(n int) (raw, hash string) {
	raw = idx.Token(n)
	hash = Hash(raw)
	return raw, hash
}

// Hash returns the SHA-256 hex digest of a token.
func Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// APIKeyPrefix generates a human-readable API key prefix (e.g. agn_abc123).
func APIKeyPrefix() string {
	return "agn_" + idx.Hex(4)
}

// FormatAPIKey combines prefix and secret for the one-time display value.
func FormatAPIKey(prefix, secret string) string {
	return prefix + "_" + secret
}

// ConstantTimeEqual compares two token hashes in constant time.
func ConstantTimeEqual(a, b string) bool {
	return hashx.ConstantTimeEqualString(a, b)
}

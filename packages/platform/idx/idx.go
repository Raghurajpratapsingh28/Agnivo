// Package idx centralizes identifier and random-token generation. It wraps
// google/uuid for UUIDs and crypto/rand for cryptographically secure tokens so
// the rest of the platform never reaches for math/rand by accident.
package idx

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// b32 is a lowercase, padding-free Base32 alphabet used for human-friendly,
// case-insensitive identifiers (e.g. prefixed resource IDs).
var b32 = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// NewUUID returns a random RFC 4122 v4 UUID string.
func NewUUID() string { return uuid.NewString() }

// MustParseUUID parses s as a UUID, panicking on failure. Use only for constant
// inputs in tests and initialization.
func MustParseUUID(s string) uuid.UUID { return uuid.MustParse(s) }

// ParseUUID parses s as a UUID, returning an error for malformed input.
func ParseUUID(s string) (uuid.UUID, error) { return uuid.Parse(s) }

// IsUUID reports whether s is a syntactically valid UUID.
func IsUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// Prefixed returns a resource identifier of the form "<prefix>_<random>", where
// random is bytes of crypto-random data Base32-encoded (Stripe-style IDs). It
// panics only if the system CSPRNG fails, which is an unrecoverable condition.
func Prefixed(prefix string, bytesLen int) string {
	if bytesLen <= 0 {
		bytesLen = 16
	}
	buf := make([]byte, bytesLen)
	mustRead(buf)
	return prefix + "_" + b32.EncodeToString(buf)
}

// Token returns a URL-safe, Base64 (raw) random token of n bytes of entropy.
func Token(n int) string {
	if n <= 0 {
		n = 32
	}
	buf := make([]byte, n)
	mustRead(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

// Hex returns a lowercase hex-encoded random token of n bytes of entropy.
func Hex(n int) string {
	if n <= 0 {
		n = 16
	}
	buf := make([]byte, n)
	mustRead(buf)
	return hex.EncodeToString(buf)
}

// Bytes returns n cryptographically secure random bytes.
func Bytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("idx: read random: %w", err)
	}
	return buf, nil
}

// Normalize lowercases and trims an identifier for stable comparison.
func Normalize(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func mustRead(buf []byte) {
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Errorf("idx: crypto/rand failed: %w", err))
	}
}

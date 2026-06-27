// Package hashx provides hashing helpers: fast non-cryptographic hashing for
// sharding and cache keys, SHA-256 content digests, and constant-time
// comparison for secret material.
package hashx

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"hash/fnv"
)

// SHA256Hex returns the hex-encoded SHA-256 digest of data. Use for content
// addressing and integrity checks, not for password storage.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// SHA256HexString is the string convenience form of SHA256Hex.
func SHA256HexString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// FNV32 returns a fast 32-bit FNV-1a hash, suitable for sharding keys across a
// fixed number of buckets. It is not cryptographically secure.
func FNV32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// FNV64 returns a fast 64-bit FNV-1a hash for cache keys and bloom-style
// fingerprints. It is not cryptographically secure.
func FNV64(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// Bucket maps key to one of n buckets using FNV-1a. It panics when n <= 0.
func Bucket(key string, n int) int {
	if n <= 0 {
		panic("hashx: bucket count must be positive")
	}
	return int(FNV32(key) % uint32(n))
}

// ConstantTimeEqual compares two byte slices in constant time, preventing
// timing attacks when comparing tokens, signatures, or MACs.
func ConstantTimeEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// ConstantTimeEqualString is the string form of ConstantTimeEqual.
func ConstantTimeEqualString(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

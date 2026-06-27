package hashx_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/hashx"
	"github.com/stretchr/testify/assert"
)

func TestSHA256Stable(t *testing.T) {
	// Known SHA-256 of empty string.
	assert.Equal(t,
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		hashx.SHA256HexString(""))
	assert.Equal(t, hashx.SHA256Hex([]byte("abc")), hashx.SHA256HexString("abc"))
}

func TestFNVAndBucket(t *testing.T) {
	assert.Equal(t, hashx.FNV32("key"), hashx.FNV32("key"))
	assert.Equal(t, hashx.FNV64("key"), hashx.FNV64("key"))
	b := hashx.Bucket("key", 16)
	assert.GreaterOrEqual(t, b, 0)
	assert.Less(t, b, 16)
}

func TestBucketPanicsOnZero(t *testing.T) {
	assert.Panics(t, func() { hashx.Bucket("x", 0) })
}

func TestConstantTimeEqual(t *testing.T) {
	assert.True(t, hashx.ConstantTimeEqualString("secret", "secret"))
	assert.False(t, hashx.ConstantTimeEqualString("secret", "secre7"))
	assert.False(t, hashx.ConstantTimeEqual([]byte("a"), []byte("ab")))
}

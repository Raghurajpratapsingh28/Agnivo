package password_test

import (
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/password"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashAndVerify(t *testing.T) {
	h, err := password.NewHasher(password.DefaultParams)
	require.NoError(t, err)

	hash, err := h.Hash("correct-horse-battery-staple")
	require.NoError(t, err)
	assert.Contains(t, hash, "$argon2id$")

	ok, err := h.Verify("correct-horse-battery-staple", hash)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = h.Verify("wrong-password", hash)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestVerifyEmptyHashUsesDummy(t *testing.T) {
	h, err := password.NewHasher(password.DefaultParams)
	require.NoError(t, err)

	start := time.Now()
	ok, err := h.Verify("any-password", "")
	require.NoError(t, err)
	assert.False(t, ok)
	// Dummy verification should take measurable time (not instant skip).
	assert.GreaterOrEqual(t, time.Since(start), time.Millisecond)
}

func TestDifferentHashesForSamePassword(t *testing.T) {
	h, err := password.NewHasher(password.DefaultParams)
	require.NoError(t, err)

	a, err := h.Hash("same-password")
	require.NoError(t, err)
	b, err := h.Hash("same-password")
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
}

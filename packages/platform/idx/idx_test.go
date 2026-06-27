package idx_test

import (
	"strings"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUUIDAndIsUUID(t *testing.T) {
	id := idx.NewUUID()
	assert.True(t, idx.IsUUID(id))
	assert.False(t, idx.IsUUID("not-a-uuid"))
}

func TestPrefixedUnique(t *testing.T) {
	a := idx.Prefixed("dep", 16)
	b := idx.Prefixed("dep", 16)
	assert.True(t, strings.HasPrefix(a, "dep_"))
	assert.NotEqual(t, a, b)
}

func TestTokenAndHex(t *testing.T) {
	tok := idx.Token(32)
	assert.NotEmpty(t, tok)
	assert.NotContains(t, tok, "=") // raw url encoding, no padding
	h := idx.Hex(16)
	assert.Len(t, h, 32)
}

func TestBytes(t *testing.T) {
	b, err := idx.Bytes(8)
	require.NoError(t, err)
	assert.Len(t, b, 8)
}

func TestNormalize(t *testing.T) {
	assert.Equal(t, "abc", idx.Normalize("  ABC  "))
}

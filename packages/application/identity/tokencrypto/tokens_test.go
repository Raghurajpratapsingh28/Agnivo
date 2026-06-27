package tokencrypto_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/tokencrypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndHash(t *testing.T) {
	raw, hash := tokencrypto.Generate(32)
	assert.NotEmpty(t, raw)
	assert.NotEmpty(t, hash)
	assert.Equal(t, hash, tokencrypto.Hash(raw))
}

func TestConstantTimeEqual(t *testing.T) {
	_, h := tokencrypto.Generate(16)
	assert.True(t, tokencrypto.ConstantTimeEqual(h, h))
	_, other := tokencrypto.Generate(16)
	assert.False(t, tokencrypto.ConstantTimeEqual(h, other))
}

func TestAPIKeyFormat(t *testing.T) {
	prefix := tokencrypto.APIKeyPrefix()
	assert.Contains(t, prefix, "agn_")
	raw, _ := tokencrypto.Generate(32)
	formatted := tokencrypto.FormatAPIKey(prefix, raw)
	require.Contains(t, formatted, prefix)
}

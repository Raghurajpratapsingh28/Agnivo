package crypto_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/crypto"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultEncryptDecrypt(t *testing.T) {
	v, err := crypto.NewVault(&config.Config{})
	require.NoError(t, err)
	aad := crypto.AAD("org-1", "proj-1")
	enc, err := v.Encrypt([]byte("secret-value"), aad)
	require.NoError(t, err)
	plain, err := v.Decrypt(enc, aad)
	require.NoError(t, err)
	assert.Equal(t, "secret-value", string(plain))
}

func TestVaultWrongAADFails(t *testing.T) {
	v, err := crypto.NewVault(&config.Config{})
	require.NoError(t, err)
	enc, err := v.Encrypt([]byte("x"), crypto.AAD("a", "b"))
	require.NoError(t, err)
	_, err = v.Decrypt(enc, crypto.AAD("c", "d"))
	require.Error(t, err)
}

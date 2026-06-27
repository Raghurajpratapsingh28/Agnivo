package cryptox_test

import (
	"bytes"
	"testing"

	"github.com/agnivo/agnivo/packages/platform/cryptox"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func key() []byte {
	k := make([]byte, cryptox.KeySize)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := cryptox.NewCipher(key())
	require.NoError(t, err)

	plaintext := []byte("super secret value")
	ad := []byte("context")

	ct, err := c.Encrypt(plaintext, ad)
	require.NoError(t, err)
	assert.False(t, bytes.Contains(ct, plaintext))

	got, err := c.Decrypt(ct, ad)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)
}

func TestDecryptWrongAssociatedData(t *testing.T) {
	c, _ := cryptox.NewCipher(key())
	ct, _ := c.Encrypt([]byte("x"), []byte("ad1"))
	_, err := c.Decrypt(ct, []byte("ad2"))
	require.Error(t, err)
	assert.Equal(t, errors.CodeInvalidArgument, errors.CodeOf(err))
}

func TestNonceIsRandom(t *testing.T) {
	c, _ := cryptox.NewCipher(key())
	a, _ := c.Encrypt([]byte("same"), nil)
	b, _ := c.Encrypt([]byte("same"), nil)
	assert.NotEqual(t, a, b)
}

func TestInvalidKeySize(t *testing.T) {
	_, err := cryptox.NewCipher([]byte("short"))
	assert.Equal(t, errors.CodeInvalidArgument, errors.CodeOf(err))
}

func TestDecryptTooShort(t *testing.T) {
	c, _ := cryptox.NewCipher(key())
	_, err := c.Decrypt([]byte{1, 2}, nil)
	assert.Equal(t, errors.CodeInvalidArgument, errors.CodeOf(err))
}

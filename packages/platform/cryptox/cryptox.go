// Package cryptox provides authenticated symmetric encryption helpers built on
// AES-256-GCM. It is intended for protecting secrets at rest (e.g. environment
// variables, provider credentials) with a key supplied by configuration or a
// KMS. A random nonce is generated per message and prepended to the ciphertext.
package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"

	"github.com/agnivo/agnivo/packages/platform/errors"
)

// KeySize is the required key length in bytes for AES-256.
const KeySize = 32

// Cipher performs authenticated encryption with a fixed 256-bit key.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher constructs a Cipher from a 32-byte key. The key must be kept secret
// and rotated according to policy; callers typically derive it from a KMS data
// key or a configured secret.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, errors.Newf(errors.CodeInvalidArgument, "cryptox: key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "cryptox: new cipher")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "cryptox: new gcm")
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt seals plaintext, returning nonce||ciphertext||tag. The optional
// associated data is authenticated but not encrypted; pass the same value to
// Decrypt.
func (c *Cipher) Encrypt(plaintext, associatedData []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "cryptox: generate nonce")
	}
	// Seal appends the ciphertext to nonce, so the nonce prefixes the output.
	return c.aead.Seal(nonce, nonce, plaintext, associatedData), nil
}

// Decrypt opens data produced by Encrypt. It returns CodeInvalidArgument when
// the ciphertext is malformed or fails authentication.
func (c *Cipher) Decrypt(data, associatedData []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New(errors.CodeInvalidArgument, "cryptox: ciphertext too short")
	}
	nonce, ciphertext := data[:ns], data[ns:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, associatedData)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInvalidArgument, "cryptox: authentication failed")
	}
	return plaintext, nil
}

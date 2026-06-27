package crypto

import (
	"encoding/base64"
	"encoding/hex"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/cryptox"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// Vault encrypts and decrypts sensitive values at rest.
type Vault struct {
	cipher *cryptox.Cipher
}

// NewVault constructs a Vault from configuration. In development, an empty key
// generates an ephemeral 32-byte key (data lost on restart).
func NewVault(cfg *config.Config) (*Vault, error) {
	key, err := loadKey(cfg.ControlPlane.EncryptionKey)
	if err != nil {
		return nil, err
	}
	c, err := cryptox.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &Vault{cipher: c}, nil
}

// Encrypt seals plaintext with optional associated data (e.g. org+project ID).
func (v *Vault) Encrypt(plaintext, aad []byte) ([]byte, error) {
	return v.cipher.Encrypt(plaintext, aad)
}

// Decrypt opens ciphertext with the same associated data used at encryption.
func (v *Vault) Decrypt(ciphertext, aad []byte) ([]byte, error) {
	return v.cipher.Decrypt(ciphertext, aad)
}

// AAD builds associated data for org-scoped encryption.
func AAD(orgID, projectID string) []byte {
	return []byte(orgID + ":" + projectID)
}

func loadKey(raw string) ([]byte, error) {
	if raw == "" {
		// Ephemeral dev key — not for production.
		b := make([]byte, cryptox.KeySize)
		for i := range b {
			b[i] = byte(i + 1)
		}
		return b, nil
	}
	if dec, err := base64.StdEncoding.DecodeString(raw); err == nil && len(dec) == cryptox.KeySize {
		return dec, nil
	}
	if dec, err := hex.DecodeString(raw); err == nil && len(dec) == cryptox.KeySize {
		return dec, nil
	}
	return nil, errors.InvalidArgument("controlplane encryption key must be 32 bytes (base64 or hex)")
}

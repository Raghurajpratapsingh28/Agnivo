// Package password provides Argon2id password hashing per OWASP recommendations.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/agnivo/agnivo/packages/platform/errors"
	"golang.org/x/crypto/argon2"
)

// Params tune Argon2id. Defaults follow OWASP guidance for interactive login.
type Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultParams is the production Argon2id configuration (64 MiB, t=3, p=4).
var DefaultParams = Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

// Hasher hashes and verifies passwords using Argon2id.
type Hasher struct {
	params Params
	// dummyHash is verified on login when the user does not exist, preventing
	// timing-based user enumeration via password check duration.
	dummyHash string
}

// NewHasher constructs a Hasher with the given params.
func NewHasher(params Params) (*Hasher, error) {
	if params.SaltLength == 0 {
		params = DefaultParams
	}
	h := &Hasher{params: params}
	// Pre-compute a dummy hash so failed lookups still run argon2.
	dummy, err := h.Hash("dummy-timing-obfuscation-password")
	if err != nil {
		return nil, err
	}
	h.dummyHash = dummy
	return h, nil
}

// Hash returns an encoded Argon2id hash string: $argon2id$v=19$m=...,t=...,p=...$salt$hash
func (h *Hasher) Hash(password string) (string, error) {
	salt := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", errors.Wrap(err, errors.CodeInternal, "password: generate salt")
	}
	key := argon2.IDKey([]byte(password), salt, h.params.Iterations, h.params.Memory, h.params.Parallelism, h.params.KeyLength)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Key := base64.RawStdEncoding.EncodeToString(key)
	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, h.params.Memory, h.params.Iterations, h.params.Parallelism, b64Salt, b64Key)
	return encoded, nil
}

// Verify compares password against encoded. When hash is empty (user not found),
// verification runs against the dummy hash to equalize timing.
func (h *Hasher) Verify(password, encoded string) (bool, error) {
	if encoded == "" {
		encoded = h.dummyHash
	}
	params, salt, key, err := decode(encoded)
	if err != nil {
		// Malformed stored hash: still run argon2 against dummy.
		_, _ = h.Verify(password, h.dummyHash)
		return false, nil
	}
	other := argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)
	return subtle.ConstantTimeCompare(key, other) == 1, nil
}

func decode(encoded string) (Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Params{}, nil, nil, fmt.Errorf("invalid hash format")
	}
	var version int
	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Params{}, nil, nil, err
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return Params{}, nil, nil, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Params{}, nil, nil, err
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Params{}, nil, nil, err
	}
	return Params{Memory: memory, Iterations: iterations, Parallelism: parallelism, KeyLength: uint32(len(key))}, salt, key, nil
}

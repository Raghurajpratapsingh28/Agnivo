// Package jwt provides RS256 JWT access token issuance and validation with clock
// skew tolerance and session binding.
package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/agnivo/agnivo/packages/platform/errors"
	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Config configures the token issuer.
type Config struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
	Issuer     string
	Audience   string
	AccessTTL  time.Duration
	ClockSkew  time.Duration
}

// Claims are the JWT access token claims.
type Claims struct {
	jwtlib.RegisteredClaims
	UserID    string `json:"uid"`
	SessionID string `json:"sid"`
	OrgID     string `json:"oid,omitempty"`
}

// Manager issues and validates access tokens.
type Manager struct {
	cfg Config
}

// NewManager constructs a Manager from config.
func NewManager(cfg Config) *Manager {
	if cfg.ClockSkew <= 0 {
		cfg.ClockSkew = 30 * time.Second
	}
	if cfg.AccessTTL <= 0 {
		cfg.AccessTTL = 15 * time.Minute
	}
	return &Manager{cfg: cfg}
}

// IssueAccessToken mints a signed RS256 access token.
func (m *Manager) IssueAccessToken(userID, sessionID, orgID string) (string, time.Time, error) {
	now := time.Now().UTC()
	exp := now.Add(m.cfg.AccessTTL)
	claims := Claims{
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer:    m.cfg.Issuer,
			Audience:  jwtlib.ClaimStrings{m.cfg.Audience},
			Subject:   userID,
			IssuedAt:  jwtlib.NewNumericDate(now),
			NotBefore: jwtlib.NewNumericDate(now.Add(-m.cfg.ClockSkew)),
			ExpiresAt: jwtlib.NewNumericDate(exp),
			ID:        newJTI(),
		},
		UserID:    userID,
		SessionID: sessionID,
		OrgID:     orgID,
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims)
	signed, err := token.SignedString(m.cfg.PrivateKey)
	if err != nil {
		return "", time.Time{}, errors.Wrap(err, errors.CodeInternal, "jwt: sign token")
	}
	return signed, exp, nil
}

// ValidateAccessToken parses and validates an access token, returning claims.
func (m *Manager) ValidateAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwtlib.ParseWithClaims(tokenStr, &Claims{}, func(t *jwtlib.Token) (any, error) {
		if t.Method != jwtlib.SigningMethodRS256 {
			return nil, errors.Unauthenticated("unexpected signing method")
		}
		return m.cfg.PublicKey, nil
	}, jwtlib.WithLeeway(m.cfg.ClockSkew))
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeUnauthenticated, "invalid access token")
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.Unauthenticated("invalid access token")
	}
	if m.cfg.Issuer != "" && claims.Issuer != m.cfg.Issuer {
		return nil, errors.Unauthenticated("invalid token issuer")
	}
	return claims, nil
}

// LoadKeysFromPEM parses RSA private and public keys from PEM strings.
func LoadKeysFromPEM(privatePEM, publicPEM string) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	priv, err := parsePrivateKey(privatePEM)
	if err != nil {
		return nil, nil, err
	}
	pub, err := parsePublicKey(publicPEM)
	if err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}

// GenerateKeyPair creates a 2048-bit RSA key pair for development.
func GenerateKeyPair() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	return priv, &priv.PublicKey, nil
}

func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("invalid private key PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA private key")
	}
	return rsaKey, nil
}

func parsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("invalid public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	return rsaPub, nil
}

func newJTI() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

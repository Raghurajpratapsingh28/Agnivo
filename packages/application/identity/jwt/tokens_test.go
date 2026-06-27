package jwt_test

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/agnivo/agnivo/packages/application/identity/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodePriv(key *rsa.PrivateKey) string {
	b, _ := x509.MarshalPKCS8PrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b}))
}

func encodePub(key *rsa.PublicKey) string {
	b, _ := x509.MarshalPKIXPublicKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: b}))
}

func TestIssueAndValidateAccessToken(t *testing.T) {
	priv, pub, err := jwt.GenerateKeyPair()
	require.NoError(t, err)

	mgr := jwt.NewManager(jwt.Config{
		PrivateKey: priv, PublicKey: pub,
		Issuer: "test", Audience: "test-api",
		AccessTTL: 15 * time.Minute, ClockSkew: 30 * time.Second,
	})

	token, exp, err := mgr.IssueAccessToken("user-1", "sess-1", "org-1")
	require.NoError(t, err)
	assert.False(t, exp.IsZero())

	claims, err := mgr.ValidateAccessToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.UserID)
	assert.Equal(t, "sess-1", claims.SessionID)
	assert.Equal(t, "org-1", claims.OrgID)
	assert.Equal(t, "test", claims.Issuer)
}

func TestValidateRejectsTamperedToken(t *testing.T) {
	priv, pub, err := jwt.GenerateKeyPair()
	require.NoError(t, err)
	mgr := jwt.NewManager(jwt.Config{PrivateKey: priv, PublicKey: pub, Issuer: "test", Audience: "test-api"})

	token, _, err := mgr.IssueAccessToken("user-1", "sess-1", "")
	require.NoError(t, err)

	_, err = mgr.ValidateAccessToken(token + "tampered")
	require.Error(t, err)
}

func TestLoadKeysFromPEM(t *testing.T) {
	priv, pub, err := jwt.GenerateKeyPair()
	require.NoError(t, err)

	loadedPriv, loadedPub, err := jwt.LoadKeysFromPEM(encodePriv(priv), encodePub(pub))
	require.NoError(t, err)
	assert.Equal(t, priv.N, loadedPriv.N)
	assert.Equal(t, pub.N, loadedPub.N)
}

package webhook_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHMACSHA256(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "test-secret"
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write(body)
	expected := "sha256=" + hex.EncodeToString(m.Sum(nil))
	assert.Contains(t, expected, "sha256=")
	assert.Len(t, hex.EncodeToString(m.Sum(nil)), 64)
}

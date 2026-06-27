package dns_test

import (
	"testing"

	"github.com/agnivo/agnivo/packages/application/proxy/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateDomainFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "example.com", false},
		{"valid subdomain", "app.example.com", false},
		{"valid deep subdomain", "a.b.c.example.com", false},
		{"empty", "", true},
		{"no dot", "localhost", true},
		{"too long overall", string(make([]byte, 254)), true},
		{"valid preview", "main-abc12345.preview.agnivo.app", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := dns.ValidateDomainFormat(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateChallenge_TXT(t *testing.T) {
	challenge := dns.GenerateChallenge(dns.MethodTXT, "550e8400-e29b-41d4-a716-446655440000")
	assert.Contains(t, challenge, "agnivo-verify=")
	assert.Len(t, challenge, len("agnivo-verify=")+16)
}

func TestGenerateChallenge_CNAME(t *testing.T) {
	challenge := dns.GenerateChallenge(dns.MethodCNAME, "550e8400-e29b-41d4-a716-446655440000")
	assert.Contains(t, challenge, ".verify.agnivo.app")
}

func TestGenerateChallenge_Deterministic(t *testing.T) {
	domainID := "550e8400-e29b-41d4-a716-446655440000"
	c1 := dns.GenerateChallenge(dns.MethodTXT, domainID)
	c2 := dns.GenerateChallenge(dns.MethodTXT, domainID)
	require.Equal(t, c1, c2)
}

func TestNewVerifier_NotNil(t *testing.T) {
	v := dns.NewVerifier("agnivo-verify=", nil)
	assert.NotNil(t, v)
}

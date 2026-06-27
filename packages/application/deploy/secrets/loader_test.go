package secrets_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/secrets"
	"github.com/stretchr/testify/assert"
)

func TestMaskEnv(t *testing.T) {
	masked := secrets.MaskEnv(model.RuntimeConfig{
		Env:     map[string]string{"PORT": "8080"},
		Secrets: map[string]string{"API_KEY": "super-secret"},
	})
	assert.Equal(t, "8080", masked["PORT"])
	assert.Equal(t, "[REDACTED]", masked["API_KEY"])
}

func TestLoadWithoutVault(t *testing.T) {
	loader := secrets.NewLoader(nil, nil, nil)
	cfg, err := loader.LoadRuntimeConfig(t.Context(), "o1", "p1", "production")
	assert.NoError(t, err)
	assert.NotNil(t, cfg.Env)
	assert.Empty(t, cfg.Labels)
}

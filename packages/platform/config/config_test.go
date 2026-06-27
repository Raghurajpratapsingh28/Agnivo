package config_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultsAndEnvOverride(t *testing.T) {
	t.Setenv("AGNIVO_APP_ENVIRONMENT", "production")
	t.Setenv("AGNIVO_HTTP_ENABLED", "true")
	t.Setenv("AGNIVO_HTTP_PORT", "9123")
	t.Setenv("AGNIVO_LOG_LEVEL", "warn")

	cfg, err := config.Load(config.Options{AppName: "api"})
	require.NoError(t, err)

	require.Equal(t, "api", cfg.App.Name)
	require.Equal(t, config.EnvProduction, cfg.App.Environment)
	require.True(t, cfg.HTTP.Enabled)
	require.Equal(t, 9123, cfg.HTTP.Port)
	require.Equal(t, "warn", cfg.Log.Level)
	// Defaults still applied where not overridden.
	require.Equal(t, 9090, cfg.Observability.AdminPort)
	require.Positive(t, cfg.App.ShutdownTimeout)
}

func TestLoad_InvalidLogLevelFails(t *testing.T) {
	t.Setenv("AGNIVO_LOG_LEVEL", "verbose")
	_, err := config.Load(config.Options{AppName: "api"})
	require.Error(t, err)
}

func TestLoad_RequiresAppName(t *testing.T) {
	_, err := config.Load(config.Options{})
	require.Error(t, err)
}

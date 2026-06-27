// Package testkit provides reusable test scaffolding shared by every executable:
// fake configuration, a no-op logger, fixtures, and assertion helpers.
package testkit

import (
	"time"

	"github.com/agnivo/agnivo/packages/platform/config"
)

// NewConfig returns a fully-populated, valid configuration for tests with all
// external dependencies disabled by default. Override fields as needed.
func NewConfig(appName string) *config.Config {
	return &config.Config{
		App: config.App{
			Name:            appName,
			Environment:     config.EnvDevelopment,
			ShutdownTimeout: 5 * time.Second,
		},
		HTTP: config.HTTP{
			Enabled:           true,
			Host:              "127.0.0.1",
			Port:              0,
			ReadTimeout:       5 * time.Second,
			ReadHeaderTimeout: 2 * time.Second,
			WriteTimeout:      5 * time.Second,
			IdleTimeout:       10 * time.Second,
			RequestTimeout:    5 * time.Second,
			CORS: config.CORS{
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST"},
				AllowedHeaders: []string{"Content-Type"},
			},
		},
		Log:      config.Log{Level: "debug", Format: "console"},
		Database: config.Database{Enabled: false},
		Redis:    config.Redis{Enabled: false},
		Observability: config.Observability{
			AdminHost: "127.0.0.1",
			AdminPort: 0,
			Tracing:   config.Tracing{Enabled: false, Sampler: 1.0},
		},
	}
}

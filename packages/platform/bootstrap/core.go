// Package bootstrap is the composition root shared by every executable. It
// builds core infrastructure via Google Wire and runs the standard startup and
// shutdown lifecycle so all binaries boot identically.
package bootstrap

import (
	"context"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/cache/redis"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/health"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/logger"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/observability/metrics"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/observability/tracing"
	"github.com/google/wire"
	"go.uber.org/zap"
)

// Core holds the infrastructure dependencies shared by all executables. DB and
// Redis are nil when disabled in configuration.
type Core struct {
	Logger  *zap.Logger
	Metrics *metrics.Registry
	Tracer  *tracing.Provider
	DB      *postgres.DB
	Redis   *redis.Client
	Health  *health.Registry
}

// ProviderSet is the Wire provider set for core infrastructure.
var ProviderSet = wire.NewSet(
	provideLogger,
	provideMetrics,
	provideTracing,
	providePostgres,
	provideRedis,
	provideHealth,
	wire.Struct(new(Core), "*"),
)

func provideLogger(cfg *config.Config) (*zap.Logger, error) {
	return logger.New(cfg)
}

func provideMetrics(cfg *config.Config) *metrics.Registry {
	return metrics.New(cfg.App.Name)
}

func provideTracing(ctx context.Context, cfg *config.Config) (*tracing.Provider, error) {
	return tracing.New(ctx, cfg)
}

// providePostgres returns nil when the database is disabled, so workers that do
// not need a database boot without one. When enabled it wires the logger and a
// Prometheus metrics collector (pool gauges, query/transaction instruments) so
// every executable observes its database uniformly.
func providePostgres(ctx context.Context, cfg *config.Config, log *zap.Logger, reg *metrics.Registry) (*postgres.DB, error) {
	if !cfg.Database.Enabled {
		return nil, nil
	}
	dbMetrics := postgres.NewMetrics(cfg.App.Name)
	reg.MustRegister(dbMetrics.Collectors()...)
	return postgres.New(ctx, cfg, postgres.WithLogger(log), postgres.WithMetrics(dbMetrics))
}

// provideRedis returns nil when Redis is disabled. When enabled it wires the
// logger and a Prometheus metrics collector.
func provideRedis(ctx context.Context, cfg *config.Config, log *zap.Logger, reg *metrics.Registry) (*redis.Client, error) {
	if !cfg.Redis.Enabled {
		return nil, nil
	}
	redisMetrics := redis.NewMetrics(cfg.App.Name)
	reg.MustRegister(redisMetrics.Collectors()...)
	return redis.New(ctx, cfg, redis.WithLogger(log), redis.WithMetrics(redisMetrics))
}

func provideHealth() *health.Registry { return health.New() }

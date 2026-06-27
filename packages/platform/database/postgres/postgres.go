// Package postgres provides a production-grade PostgreSQL abstraction over
// pgxpool: connection pooling, readiness verification, a context-propagating
// transaction manager with nested (savepoint) support, automatic retries of
// transient failures, query timeouts, prepared-statement caching, a dependency-
// free migration runner, pool health monitoring, Prometheus metrics, and
// graceful shutdown.
//
// Business logic never imports pgx directly. Repositories accept a Querier
// (satisfied by both the pool and an in-flight transaction), so the same code
// runs inside or outside a transaction with no branching.
package postgres

import (
	"context"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// DB is the shared database handle. It embeds *pgxpool.Pool for direct pool
// access while layering transaction management, retries, and metrics on top.
type DB struct {
	*pgxpool.Pool

	cfg     config.Database
	log     *zap.Logger
	metrics *Metrics
}

// Option customizes DB construction.
type Option func(*options)

type options struct {
	log     *zap.Logger
	metrics *Metrics
}

// WithLogger attaches a logger used by the health monitor and retry logging.
func WithLogger(log *zap.Logger) Option {
	return func(o *options) { o.log = log }
}

// WithMetrics attaches a Metrics collector; pool gauges are exported from it.
func WithMetrics(m *Metrics) Option {
	return func(o *options) { o.metrics = m }
}

// New creates and verifies a PostgreSQL connection pool. The pool is pinged
// before returning so a misconfigured database fails fast at boot. pgx's
// statement-cache exec mode is enabled by default, giving implicit prepared
// statements for every distinct query without per-call ceremony.
func New(ctx context.Context, cfg *config.Config, opts ...Option) (*DB, error) {
	o := &options{}
	for _, fn := range opts {
		fn(o)
	}
	if o.log == nil {
		o.log = zap.NewNop()
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInvalidArgument, "postgres: parse url")
	}

	if cfg.Database.MaxConns > 0 {
		poolCfg.MaxConns = cfg.Database.MaxConns
	}
	if cfg.Database.MinConns > 0 {
		poolCfg.MinConns = cfg.Database.MinConns
	}
	if cfg.Database.MaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.Database.MaxConnLifetime
	}
	if cfg.Database.MaxConnIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.Database.MaxConnIdleTime
	}
	if cfg.Database.ConnectTimeout > 0 {
		poolCfg.ConnConfig.ConnectTimeout = cfg.Database.ConnectTimeout
	}
	// Cache prepared statements per connection. This is the recommended default
	// for application workloads: repeated queries skip parse/plan on the server.
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeUnavailable, "postgres: connect")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.Wrap(err, errors.CodeUnavailable, "postgres: ping")
	}

	db := &DB{Pool: pool, cfg: cfg.Database, log: o.log, metrics: o.metrics}
	if o.metrics != nil {
		o.metrics.bindPool(pool)
	}
	return db, nil
}

// Check verifies connectivity for readiness probes, honoring any context
// deadline supplied by the caller.
func (db *DB) Check(ctx context.Context) error {
	if db == nil || db.Pool == nil {
		return errors.New(errors.CodeUnavailable, "postgres: pool not initialized")
	}
	if err := db.Pool.Ping(ctx); err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "postgres: ping")
	}
	return nil
}

// Close releases all pool connections. It is idempotent and safe on a nil DB.
func (db *DB) Close() {
	if db != nil && db.Pool != nil {
		db.Pool.Close()
	}
}

// QueryTimeout returns the configured default per-query timeout (zero when
// disabled).
func (db *DB) QueryTimeout() time.Duration { return db.cfg.QueryTimeout }

// Context derives a child context bounded by the configured query timeout. When
// the timeout is zero, the parent context is returned with a no-op cancel so
// callers can always defer cancel() uniformly.
func (db *DB) Context(ctx context.Context) (context.Context, context.CancelFunc) {
	if db.cfg.QueryTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, db.cfg.QueryTimeout)
}

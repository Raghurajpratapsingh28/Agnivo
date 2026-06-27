// Package redis provides a production-grade Redis abstraction over go-redis:
// connection pooling, health checks, Prometheus metrics, pub/sub and stream
// helpers, distributed locks, rate-limiting primitives, TTL helpers, pipelines,
// and graceful shutdown. Higher layers depend on the small interfaces declared
// here rather than on go-redis directly where practical.
package redis

import (
	"context"

	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/agnivo/agnivo/packages/platform/errors"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Client wraps a go-redis client with platform conveniences. The embedded
// *goredis.Client remains available for advanced commands not wrapped here.
type Client struct {
	*goredis.Client

	log     *zap.Logger
	metrics *Metrics
}

// Option customizes Client construction.
type Option func(*options)

type options struct {
	log     *zap.Logger
	metrics *Metrics
}

// WithLogger attaches a logger used by locks and background helpers.
func WithLogger(log *zap.Logger) Option { return func(o *options) { o.log = log } }

// WithMetrics attaches a Metrics collector exporting pool gauges and command
// instrumentation.
func WithMetrics(m *Metrics) Option { return func(o *options) { o.metrics = m } }

// New creates and verifies a Redis client. The client is pinged before
// returning so a misconfigured Redis fails fast at boot.
func New(ctx context.Context, cfg *config.Config, opts ...Option) (*Client, error) {
	o := &options{}
	for _, fn := range opts {
		fn(o)
	}
	if o.log == nil {
		o.log = zap.NewNop()
	}

	parsed, err := goredis.ParseURL(cfg.Redis.URL)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInvalidArgument, "redis: parse url")
	}
	if cfg.Redis.PoolSize > 0 {
		parsed.PoolSize = cfg.Redis.PoolSize
	}
	if cfg.Redis.MinIdleConns > 0 {
		parsed.MinIdleConns = cfg.Redis.MinIdleConns
	}
	if cfg.Redis.DialTimeout > 0 {
		parsed.DialTimeout = cfg.Redis.DialTimeout
	}
	if cfg.Redis.ReadTimeout > 0 {
		parsed.ReadTimeout = cfg.Redis.ReadTimeout
	}
	if cfg.Redis.WriteTimeout > 0 {
		parsed.WriteTimeout = cfg.Redis.WriteTimeout
	}

	client := goredis.NewClient(parsed)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, errors.Wrap(err, errors.CodeUnavailable, "redis: ping")
	}

	c := &Client{Client: client, log: o.log, metrics: o.metrics}
	if o.metrics != nil {
		o.metrics.bindClient(client)
	}
	return c, nil
}

// Check verifies connectivity for readiness probes.
func (c *Client) Check(ctx context.Context) error {
	if c == nil || c.Client == nil {
		return errors.New(errors.CodeUnavailable, "redis: client not initialized")
	}
	if err := c.Client.Ping(ctx).Err(); err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "redis: ping")
	}
	return nil
}

// Close releases the client connections. It is idempotent and nil-safe.
func (c *Client) Close() error {
	if c == nil || c.Client == nil {
		return nil
	}
	return c.Client.Close()
}

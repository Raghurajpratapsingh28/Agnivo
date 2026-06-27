package redis

import (
	"context"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	goredis "github.com/redis/go-redis/v9"
)

// SetTTL stores value at key with an expiration. A ttl of 0 stores it without
// expiry.
func (c *Client) SetTTL(ctx context.Context, key string, value any, ttl time.Duration) error {
	if err := c.Client.Set(ctx, key, value, ttl).Err(); err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "redis: set")
	}
	return nil
}

// GetString fetches a string value. It returns (", false, nil) when the key is
// absent, distinguishing "missing" from "error".
func (c *Client) GetString(ctx context.Context, key string) (string, bool, error) {
	v, err := c.Client.Get(ctx, key).Result()
	if err == goredis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.Wrap(err, errors.CodeUnavailable, "redis: get")
	}
	return v, true, nil
}

// Incr atomically increments the integer at key and returns the new value,
// creating it at 0 first when absent.
func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	v, err := c.Client.Incr(ctx, key).Result()
	if err != nil {
		return 0, errors.Wrap(err, errors.CodeUnavailable, "redis: incr")
	}
	return v, nil
}

// IncrBy atomically adds delta to the integer at key and returns the new value.
func (c *Client) IncrBy(ctx context.Context, key string, delta int64) (int64, error) {
	v, err := c.Client.IncrBy(ctx, key, delta).Result()
	if err != nil {
		return 0, errors.Wrap(err, errors.CodeUnavailable, "redis: incrby")
	}
	return v, nil
}

// Expire sets a TTL on an existing key, returning whether the key existed.
func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	ok, err := c.Client.Expire(ctx, key, ttl).Result()
	if err != nil {
		return false, errors.Wrap(err, errors.CodeUnavailable, "redis: expire")
	}
	return ok, nil
}

// TTL returns the remaining time-to-live for key. A negative duration means no
// expiry (-1) or missing key (-2), matching Redis semantics.
func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	d, err := c.Client.TTL(ctx, key).Result()
	if err != nil {
		return 0, errors.Wrap(err, errors.CodeUnavailable, "redis: ttl")
	}
	return d, nil
}

// Delete removes keys and returns the number actually deleted.
func (c *Client) Delete(ctx context.Context, keys ...string) (int64, error) {
	n, err := c.Client.Del(ctx, keys...).Result()
	if err != nil {
		return 0, errors.Wrap(err, errors.CodeUnavailable, "redis: del")
	}
	return n, nil
}

// Pipeline executes fn against a pipeliner and flushes all queued commands in a
// single round trip, returning the per-command results. Use it to batch many
// independent operations and cut latency.
func (c *Client) Pipeline(ctx context.Context, fn func(p goredis.Pipeliner) error) ([]goredis.Cmder, error) {
	pipe := c.Client.Pipeline()
	if err := fn(pipe); err != nil {
		return nil, err
	}
	cmds, err := pipe.Exec(ctx)
	if err != nil && err != goredis.Nil {
		return cmds, errors.Wrap(err, errors.CodeUnavailable, "redis: pipeline exec")
	}
	return cmds, nil
}

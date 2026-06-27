package redis

import (
	"context"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	goredis "github.com/redis/go-redis/v9"
)

// tokenBucketScript implements an atomic token-bucket rate limiter entirely in
// Redis. State (tokens, last-refill timestamp) lives in a hash keyed by the
// limiter key; the script refills based on elapsed time, then conditionally
// consumes. Running it server-side guarantees atomicity across many callers and
// avoids races that a GET/SET round trip would introduce.
//
// KEYS[1] = bucket key
// ARGV[1] = capacity (max tokens)
// ARGV[2] = refill rate (tokens per second)
// ARGV[3] = now (unix milliseconds)
// ARGV[4] = requested tokens
// Returns {allowed (1/0), remaining tokens, retry-after milliseconds}
var tokenBucketScript = goredis.NewScript(`
local capacity = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])

local state = redis.call("HMGET", KEYS[1], "tokens", "ts")
local tokens = tonumber(state[1])
local ts = tonumber(state[2])
if tokens == nil then
	tokens = capacity
	ts = now
end

local elapsed = math.max(0, now - ts) / 1000.0
tokens = math.min(capacity, tokens + elapsed * rate)

local allowed = 0
local retry_after = 0
if tokens >= requested then
	allowed = 1
	tokens = tokens - requested
else
	if rate > 0 then
		retry_after = math.ceil(((requested - tokens) / rate) * 1000)
	end
end

redis.call("HSET", KEYS[1], "tokens", tokens, "ts", now)
-- Expire idle buckets so unused keys do not accumulate.
local ttl = math.ceil(capacity / math.max(rate, 0.001)) + 1
redis.call("EXPIRE", KEYS[1], ttl)

return {allowed, math.floor(tokens), retry_after}`)

// RateLimitResult describes the outcome of an Allow check.
type RateLimitResult struct {
	// Allowed reports whether the request may proceed.
	Allowed bool
	// Remaining is the approximate number of tokens left after this call.
	Remaining int64
	// RetryAfter is how long to wait before the request would succeed. Zero
	// when Allowed is true.
	RetryAfter time.Duration
}

// TokenBucket is a reusable, Redis-backed token-bucket limiter. A single
// instance is safe for concurrent use and can serve many distinct keys.
type TokenBucket struct {
	client   *goredis.Client
	capacity int
	rate     float64 // tokens per second
}

// NewTokenBucket constructs a limiter that allows bursts up to capacity and
// refills at ratePerSec tokens per second.
func (c *Client) NewTokenBucket(capacity int, ratePerSec float64) *TokenBucket {
	return &TokenBucket{client: c.Client, capacity: capacity, rate: ratePerSec}
}

// Allow consumes one token for key. See AllowN for batch consumption.
func (b *TokenBucket) Allow(ctx context.Context, key string) (RateLimitResult, error) {
	return b.AllowN(ctx, key, 1)
}

// AllowN attempts to consume n tokens for key atomically.
func (b *TokenBucket) AllowN(ctx context.Context, key string, n int) (RateLimitResult, error) {
	now := time.Now().UnixMilli()
	res, err := tokenBucketScript.Run(ctx, b.client, []string{key},
		b.capacity, b.rate, now, n).Int64Slice()
	if err != nil {
		return RateLimitResult{}, errors.Wrap(err, errors.CodeUnavailable, "redis: rate limit")
	}
	if len(res) != 3 {
		return RateLimitResult{}, errors.New(errors.CodeInternal, "redis: unexpected rate limit reply")
	}
	return RateLimitResult{
		Allowed:    res[0] == 1,
		Remaining:  res[1],
		RetryAfter: time.Duration(res[2]) * time.Millisecond,
	}, nil
}

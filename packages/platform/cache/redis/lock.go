package redis

import (
	"context"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	goredis "github.com/redis/go-redis/v9"
)

// releaseScript deletes the lock key only if it still holds our token, so a
// lock that already expired and was re-acquired by another owner is never
// released by us. This is the canonical safe single-instance unlock.
var releaseScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
else
	return 0
end`)

// extendScript renews the TTL only while we still own the lock.
var extendScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("PEXPIRE", KEYS[1], ARGV[2])
else
	return 0
end`)

// ErrLockNotAcquired is returned by Acquire when the lock is already held.
var ErrLockNotAcquired = errors.New(errors.CodeConflict, "redis: lock not acquired")

// Lock is a held distributed lock. It carries a unique fencing token so only
// the owner can release or extend it.
type Lock struct {
	client *goredis.Client
	key    string
	token  string
	ttl    time.Duration
}

// Acquire attempts to acquire the lock named key for ttl using SET NX PX. It
// returns ErrLockNotAcquired (a retryable conflict) when another owner holds
// it. Always defer Release on success.
func (c *Client) Acquire(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
	token := idx.Token(16)
	ok, err := c.Client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeUnavailable, "redis: acquire lock")
	}
	if !ok {
		return nil, ErrLockNotAcquired.WithRetryable(true)
	}
	return &Lock{client: c.Client, key: key, token: token, ttl: ttl}, nil
}

// AcquireWait repeatedly tries to acquire the lock until success, ctx
// cancellation, or the overall wait budget elapses, polling every retry
// interval. It is suitable for short, contended critical sections.
func (c *Client) AcquireWait(ctx context.Context, key string, ttl, wait, retry time.Duration) (*Lock, error) {
	deadline := time.Now().Add(wait)
	for {
		lock, err := c.Acquire(ctx, key, ttl)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, ErrLockNotAcquired) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrLockNotAcquired
		}
		select {
		case <-ctx.Done():
			return nil, errors.Wrap(ctx.Err(), errors.CodeCanceled, "redis: acquire wait canceled")
		case <-time.After(retry):
		}
	}
}

// Release frees the lock if we still own it. Releasing an expired or
// re-acquired lock is a no-op, never affecting another owner.
func (l *Lock) Release(ctx context.Context) error {
	res, err := releaseScript.Run(ctx, l.client, []string{l.key}, l.token).Int()
	if err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "redis: release lock")
	}
	if res == 0 {
		return errors.New(errors.CodeConflict, "redis: lock no longer owned")
	}
	return nil
}

// Extend renews the lock TTL while we still own it, for long-running work that
// must keep the lock alive past its initial TTL.
func (l *Lock) Extend(ctx context.Context, ttl time.Duration) error {
	res, err := extendScript.Run(ctx, l.client, []string{l.key}, l.token, ttl.Milliseconds()).Int()
	if err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "redis: extend lock")
	}
	if res == 0 {
		return errors.New(errors.CodeConflict, "redis: lock no longer owned")
	}
	l.ttl = ttl
	return nil
}

// Token returns the lock's fencing token, useful for logging and debugging.
func (l *Lock) Token() string { return l.token }

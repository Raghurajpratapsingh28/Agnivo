// Package cachex provides a generic, thread-safe in-memory cache with per-entry
// TTL and lazy expiration, plus a single-flight loader to collapse concurrent
// cache misses for the same key into one computation. It is intended for small,
// hot, process-local data (config snapshots, parsed templates), not as a
// distributed cache.
package cachex

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Cache is a generic key/value cache with optional per-entry expiration.
type Cache[K comparable, V any] struct {
	mu      sync.RWMutex
	entries map[K]entry[V]
	now     func() time.Time
	sf      singleflight.Group
}

type entry[V any] struct {
	value   V
	expires time.Time // zero means no expiry
}

// New creates an empty cache.
func New[K comparable, V any]() *Cache[K, V] {
	return &Cache[K, V]{entries: make(map[K]entry[V]), now: time.Now}
}

// Set stores value under key with the given ttl. A ttl <= 0 stores the entry
// without expiration.
func (c *Cache[K, V]) Set(key K, value V, ttl time.Duration) {
	var exp time.Time
	if ttl > 0 {
		exp = c.now().Add(ttl)
	}
	c.mu.Lock()
	c.entries[key] = entry[V]{value: value, expires: exp}
	c.mu.Unlock()
}

// Get returns the value for key and whether it was present and unexpired.
// Expired entries are treated as absent and removed lazily.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		var zero V
		return zero, false
	}
	if !e.expires.IsZero() && c.now().After(e.expires) {
		c.Delete(key)
		var zero V
		return zero, false
	}
	return e.value, true
}

// Delete removes key from the cache.
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Len returns the number of entries, including any not yet lazily expired.
func (c *Cache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Purge removes all expired entries, reclaiming memory. Call periodically when
// the working set churns; reads alone only expire entries that are accessed.
func (c *Cache[K, V]) Purge() {
	now := c.now()
	c.mu.Lock()
	for k, e := range c.entries {
		if !e.expires.IsZero() && now.After(e.expires) {
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()
}

// GetOrLoad returns the cached value for key, or computes it via load exactly
// once across concurrent callers (single-flight), caching the result with ttl.
// Errors from load are not cached.
func (c *Cache[K, V]) GetOrLoad(key K, ttl time.Duration, load func() (V, error)) (V, error) {
	if v, ok := c.Get(key); ok {
		return v, nil
	}
	// singleflight keys are strings; derive a stable key string from K.
	v, err, _ := c.sf.Do(keyString(key), func() (any, error) {
		// Re-check after acquiring the flight in case another caller filled it.
		if cached, ok := c.Get(key); ok {
			return cached, nil
		}
		loaded, err := load()
		if err != nil {
			return nil, err
		}
		c.Set(key, loaded, ttl)
		return loaded, nil
	})
	if err != nil {
		var zero V
		return zero, err
	}
	return v.(V), nil
}

// keyString derives a stable single-flight key from an arbitrary comparable
// key. %v is deterministic for the comparable types used as cache keys.
func keyString[K comparable](key K) string { return fmt.Sprintf("%v", key) }

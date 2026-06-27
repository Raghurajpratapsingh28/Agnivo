package cachex_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/cachex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetGetDelete(t *testing.T) {
	c := cachex.New[string, int]()
	c.Set("a", 1, 0)
	v, ok := c.Get("a")
	assert.True(t, ok)
	assert.Equal(t, 1, v)

	c.Delete("a")
	_, ok = c.Get("a")
	assert.False(t, ok)
}

func TestExpiry(t *testing.T) {
	c := cachex.New[string, string]()
	c.Set("k", "v", 20*time.Millisecond)
	_, ok := c.Get("k")
	assert.True(t, ok)
	time.Sleep(40 * time.Millisecond)
	_, ok = c.Get("k")
	assert.False(t, ok)
}

func TestGetOrLoadSingleFlight(t *testing.T) {
	c := cachex.New[string, int]()
	var loads int64
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.GetOrLoad("key", time.Minute, func() (int, error) {
				atomic.AddInt64(&loads, 1)
				time.Sleep(10 * time.Millisecond)
				return 42, nil
			})
			require.NoError(t, err)
			assert.Equal(t, 42, v)
		}()
	}
	wg.Wait()
	// Single-flight collapses concurrent misses into one load.
	assert.Equal(t, int64(1), atomic.LoadInt64(&loads))
}

func TestPurge(t *testing.T) {
	c := cachex.New[int, int]()
	c.Set(1, 1, time.Nanosecond)
	c.Set(2, 2, 0)
	time.Sleep(time.Millisecond)
	c.Purge()
	assert.Equal(t, 1, c.Len())
}

package cache_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/cache"
	"github.com/stretchr/testify/assert"
)

func TestCacheStatsHitRatio(t *testing.T) {
	s := cache.Stats{}
	s.RecordLine("CACHED [stage 1/5]")
	s.RecordLine("exporting layer sha256:abc")
	s.RecordLine("cache hit for step 2")
	assert.Greater(t, s.HitRatio(), 0.0)
	assert.Equal(t, 3, s.TotalLayers())
}

func TestCacheManager(t *testing.T) {
	m := cache.NewManager(true, "registry/cache:ref", true)
	assert.True(t, m.Enabled())
	assert.Equal(t, "registry/cache:ref", m.RegistryRef())
	assert.True(t, m.InlineCache())
}

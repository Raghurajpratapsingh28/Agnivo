package cache

import (
	"strings"
	"sync/atomic"
)

// Stats tracks layer cache effectiveness for a build.
type Stats struct {
	hitLayers   atomic.Int32
	totalLayers atomic.Int32
}

// RecordLine parses build output for cache hit/miss hints.
func (s *Stats) RecordLine(line string) {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "cached") || strings.Contains(lower, "cache hit") {
		s.hitLayers.Add(1)
		s.totalLayers.Add(1)
	} else if strings.Contains(lower, "exporting layer") || strings.Contains(lower, "sha256:") {
		s.totalLayers.Add(1)
	}
}

// Merge combines stats from another Stats value.
func (s *Stats) Merge(other *Stats) {
	if other == nil {
		return
	}
	s.hitLayers.Add(other.hitLayers.Load())
	s.totalLayers.Add(other.totalLayers.Load())
}

// HitRatio returns cache hit ratio in [0,1].
func (s *Stats) HitRatio() float64 {
	total := s.totalLayers.Load()
	if total == 0 {
		return 0
	}
	return float64(s.hitLayers.Load()) / float64(total)
}

// HitLayers returns the number of cache hits.
func (s *Stats) HitLayers() int { return int(s.hitLayers.Load()) }

// TotalLayers returns the total layer count observed.
func (s *Stats) TotalLayers() int { return int(s.totalLayers.Load()) }

// Manager provides cache configuration helpers.
type Manager struct {
	enabled     bool
	registryRef string
	inlineCache bool
}

// NewManager constructs a cache manager.
func NewManager(enabled bool, registryRef string, inlineCache bool) *Manager {
	return &Manager{enabled: enabled, registryRef: registryRef, inlineCache: inlineCache}
}

// Enabled reports whether caching is active.
func (m *Manager) Enabled() bool { return m.enabled }

// RegistryRef returns the remote cache reference.
func (m *Manager) RegistryRef() string { return m.registryRef }

// InlineCache reports whether inline cache export is enabled.
func (m *Manager) InlineCache() bool { return m.inlineCache }

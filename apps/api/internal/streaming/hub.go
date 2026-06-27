// Package streaming manages live SSE and WebSocket fan-out for build logs,
// deployment progress, and notifications. This is the foundation; per-feature
// channels are added in later prompts.
package streaming

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// Hub manages live connections and broadcasts.
type Hub struct {
	log   *zap.Logger
	mu    sync.RWMutex
	conns map[string]struct{}
}

// NewHub creates a streaming hub.
func NewHub(log *zap.Logger) *Hub {
	return &Hub{log: log, conns: make(map[string]struct{})}
}

// Run consumes the event bus and fans out to subscribers until ctx is cancelled.
func (h *Hub) Run(ctx context.Context) error {
	h.log.Info("streaming hub started")
	<-ctx.Done()
	h.log.Info("streaming hub stopped")
	return ctx.Err()
}

// Shutdown drains active connections.
func (h *Hub) Shutdown(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns = make(map[string]struct{})
	return nil
}

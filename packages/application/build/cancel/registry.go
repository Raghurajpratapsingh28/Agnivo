package cancel

import (
	"context"
	"sync"
)

// Registry tracks in-flight builds for cooperative cancellation.
type Registry struct {
	mu      sync.RWMutex
	active  map[string]context.CancelFunc
}

// NewRegistry constructs a cancellation registry.
func NewRegistry() *Registry {
	return &Registry{active: make(map[string]context.CancelFunc)}
}

// Register associates a deployment with its cancel function.
func (r *Registry) Register(deploymentID string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if prev, ok := r.active[deploymentID]; ok {
		prev()
	}
	r.active[deploymentID] = cancel
}

// Unregister removes a deployment from the registry.
func (r *Registry) Unregister(deploymentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, deploymentID)
}

// Cancel cancels an in-flight build by deployment ID.
func (r *Registry) Cancel(deploymentID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if fn, ok := r.active[deploymentID]; ok {
		fn()
		return true
	}
	return false
}

// ActiveCount returns the number of in-flight builds.
func (r *Registry) ActiveCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.active)
}

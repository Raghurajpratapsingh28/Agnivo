// Package health provides liveness and readiness checking shared by all
// executables. Liveness reflects "the process is up"; readiness aggregates
// dependency checks (database, redis, ...).
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// CheckFunc verifies a single dependency.
type CheckFunc func(ctx context.Context) error

// Registry aggregates readiness checks.
type Registry struct {
	mu     sync.RWMutex
	checks map[string]CheckFunc
}

// New creates an empty registry.
func New() *Registry { return &Registry{checks: make(map[string]CheckFunc)} }

// Register adds a named readiness check.
func (r *Registry) Register(name string, fn CheckFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checks[name] = fn
}

// result is the readiness response body.
type result struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// Live is the liveness handler: 200 as long as the process serves requests.
func (r *Registry) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, result{Status: "ok"})
}

// Ready is the readiness handler: 200 only when all dependencies pass.
func (r *Registry) Ready(w http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()

	r.mu.RLock()
	checks := make(map[string]CheckFunc, len(r.checks))
	for k, v := range r.checks {
		checks[k] = v
	}
	r.mu.RUnlock()

	res := result{Status: "ok", Checks: make(map[string]string, len(checks))}
	code := http.StatusOK
	for name, fn := range checks {
		if err := fn(ctx); err != nil {
			res.Checks[name] = "unhealthy: " + err.Error()
			res.Status = "degraded"
			code = http.StatusServiceUnavailable
			continue
		}
		res.Checks[name] = "ok"
	}
	writeJSON(w, code, res)
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

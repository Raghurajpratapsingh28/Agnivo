// Package resilience provides production reliability primitives: circuit breakers
// and bulkhead-style concurrency guards for external service calls.
package resilience

import (
	"sync"
	"time"

	"github.com/agnivo/agnivo/packages/platform/errors"
)

// State is the circuit breaker lifecycle state.
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

// CircuitBreaker prevents cascading failures by short-circuiting calls when a
// dependency is unhealthy. It is safe for concurrent use.
type CircuitBreaker struct {
	mu sync.Mutex

	name          string
	maxFailures   int
	openDuration  time.Duration
	halfOpenMax   int

	state         State
	failures      int
	halfOpenTries int
	openUntil     time.Time
}

// CircuitBreakerConfig tunes a CircuitBreaker.
type CircuitBreakerConfig struct {
	Name         string
	MaxFailures  int           // consecutive failures before opening (default 5)
	OpenDuration time.Duration // how long the circuit stays open (default 30s)
	HalfOpenMax  int           // probe attempts in half-open (default 1)
}

// NewCircuitBreaker constructs a CircuitBreaker.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = 5
	}
	if cfg.OpenDuration <= 0 {
		cfg.OpenDuration = 30 * time.Second
	}
	if cfg.HalfOpenMax <= 0 {
		cfg.HalfOpenMax = 1
	}
	return &CircuitBreaker{
		name:         cfg.Name,
		maxFailures:  cfg.MaxFailures,
		openDuration: cfg.OpenDuration,
		halfOpenMax:  cfg.HalfOpenMax,
		state:        StateClosed,
	}
}

// Call executes fn when the circuit allows it.
func (cb *CircuitBreaker) Call(fn func() error) error {
	if err := cb.beforeCall(); err != nil {
		return err
	}
	err := fn()
	cb.afterCall(err)
	return err
}

func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateOpen:
		if time.Now().Before(cb.openUntil) {
			return errors.New(errors.CodeUnavailable, "circuit breaker open: "+cb.name)
		}
		cb.state = StateHalfOpen
		cb.halfOpenTries = 0
		return nil
	case StateHalfOpen:
		if cb.halfOpenTries >= cb.halfOpenMax {
			return errors.New(errors.CodeUnavailable, "circuit breaker half-open limit: "+cb.name)
		}
		cb.halfOpenTries++
		return nil
	default:
		return nil
	}
}

func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err == nil {
		cb.failures = 0
		cb.state = StateClosed
		return
	}

	cb.failures++
	if cb.state == StateHalfOpen || cb.failures >= cb.maxFailures {
		cb.state = StateOpen
		cb.openUntil = time.Now().Add(cb.openDuration)
		cb.failures = 0
	}
}

// State returns the current circuit state (for metrics/tests).
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

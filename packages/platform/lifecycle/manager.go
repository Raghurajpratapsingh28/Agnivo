// Package lifecycle coordinates ordered startup hooks, long-running runners, and
// reverse-ordered graceful shutdown driven by OS signals or context cancellation.
package lifecycle

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// Runner is a long-running component (HTTP server, worker loop) that blocks
// until its context is cancelled.
type Runner struct {
	Name string
	Run  func(ctx context.Context) error
}

// Hook is a startup/shutdown pair. Start runs during boot in registration
// order; Stop runs during shutdown in reverse order. Either may be nil.
type Hook struct {
	Name  string
	Start func(ctx context.Context) error
	Stop  func(ctx context.Context) error
}

// Manager owns process lifecycle.
type Manager struct {
	log             *zap.Logger
	shutdownTimeout time.Duration
	hooks           []Hook
	runners         []Runner
}

// New creates a lifecycle manager.
func New(log *zap.Logger, shutdownTimeout time.Duration) *Manager {
	return &Manager{log: log, shutdownTimeout: shutdownTimeout}
}

// AddHook registers a start/stop hook.
func (m *Manager) AddHook(h Hook) { m.hooks = append(m.hooks, h) }

// AddRunner registers a long-running component.
func (m *Manager) AddRunner(r Runner) { m.runners = append(m.runners, r) }

// Run executes start hooks, launches runners, blocks until a signal, a runner
// error, or context cancellation, then runs stop hooks in reverse with a bounded
// timeout. It is the single entry point for every executable's main loop.
func (m *Manager) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start hooks in order; abort (and unwind) on first failure.
	started := 0
	for _, h := range m.hooks {
		if h.Start == nil {
			started++
			continue
		}
		m.log.Info("lifecycle: starting", zap.String("hook", h.Name))
		if err := h.Start(ctx); err != nil {
			m.log.Error("lifecycle: start failed", zap.String("hook", h.Name), zap.Error(err))
			m.stop(m.hooks[:started])
			return err
		}
		started++
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	errCh := make(chan error, len(m.runners))
	for _, r := range m.runners {
		runner := r
		go func() {
			m.log.Info("lifecycle: runner started", zap.String("runner", runner.Name))
			errCh <- runner.Run(ctx)
		}()
	}

	var runErr error
	select {
	case sig := <-sigCh:
		m.log.Info("lifecycle: signal received", zap.String("signal", sig.String()))
	case <-ctx.Done():
		m.log.Info("lifecycle: context cancelled")
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			runErr = err
			m.log.Error("lifecycle: runner exited with error", zap.Error(err))
		}
	}

	// Signal all runners to stop, then run shutdown hooks in reverse.
	cancel()
	m.stop(m.hooks[:started])
	return runErr
}

func (m *Manager) stop(hooks []Hook) {
	ctx, cancel := context.WithTimeout(context.Background(), m.shutdownTimeout)
	defer cancel()
	for i := len(hooks) - 1; i >= 0; i-- {
		h := hooks[i]
		if h.Stop == nil {
			continue
		}
		m.log.Info("lifecycle: stopping", zap.String("hook", h.Name))
		if err := h.Stop(ctx); err != nil {
			m.log.Error("lifecycle: stop failed", zap.String("hook", h.Name), zap.Error(err))
		}
	}
}

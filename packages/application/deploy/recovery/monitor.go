package recovery

import (
	"context"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/deploystore"
	deploymodel "github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/model"
)

// Monitor detects stale in-progress deployments for recovery.
type Monitor struct {
	executions *deploystore.Repository
	staleAfter time.Duration
}

// NewMonitor constructs a recovery monitor.
func NewMonitor(executions *deploystore.Repository, staleAfter time.Duration) *Monitor {
	if staleAfter <= 0 {
		staleAfter = 30 * time.Minute
	}
	return &Monitor{executions: executions, staleAfter: staleAfter}
}

// StaleCount returns running executions older than staleAfter (approximation via stats).
func (m *Monitor) StaleCount(ctx context.Context) (int64, error) {
	stats, err := m.executions.Stats(ctx)
	if err != nil {
		return 0, err
	}
	return stats[deploymodel.ExecRunning], nil
}

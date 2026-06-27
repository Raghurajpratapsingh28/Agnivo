package postgres

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// Monitor runs a background loop that periodically pings the database and logs
// transitions between healthy and unhealthy states. It is a long-running task
// suitable for registration with the lifecycle manager (App.AddRunner). It
// returns when ctx is canceled. A zero HealthInterval disables monitoring.
func (db *DB) Monitor(ctx context.Context) error {
	interval := db.cfg.HealthInterval
	if interval <= 0 {
		<-ctx.Done()
		return ctx.Err()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	healthy := true
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, interval)
			err := db.Pool.Ping(pingCtx)
			cancel()

			switch {
			case err != nil && healthy:
				healthy = false
				db.log.Error("database became unhealthy", zap.Error(err))
			case err == nil && !healthy:
				healthy = true
				db.log.Info("database recovered")
			}
		}
	}
}

// Stats returns a point-in-time snapshot of pool statistics, useful for
// debugging and admin endpoints without scraping Prometheus.
func (db *DB) Stats() PoolStats {
	s := db.Pool.Stat()
	return PoolStats{
		AcquiredConns:    s.AcquiredConns(),
		IdleConns:        s.IdleConns(),
		TotalConns:       s.TotalConns(),
		MaxConns:         s.MaxConns(),
		NewConnsCount:    s.NewConnsCount(),
		AcquireCount:     s.AcquireCount(),
		EmptyAcquireWait: s.EmptyAcquireCount(),
	}
}

// PoolStats is a transport-friendly snapshot of pgxpool.Stat.
type PoolStats struct {
	AcquiredConns    int32 `json:"acquired_conns"`
	IdleConns        int32 `json:"idle_conns"`
	TotalConns       int32 `json:"total_conns"`
	MaxConns         int32 `json:"max_conns"`
	NewConnsCount    int64 `json:"new_conns_count"`
	AcquireCount     int64 `json:"acquire_count"`
	EmptyAcquireWait int64 `json:"empty_acquire_count"`
}

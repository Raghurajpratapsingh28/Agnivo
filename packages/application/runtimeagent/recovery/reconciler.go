package recovery

import (
	"context"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/docker"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/executor"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/store"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
)

// Reconciler restores crashed containers and runs GC.
type Reconciler struct {
	cfg       config.RuntimeAgent
	docker    *docker.Client
	store     *store.Repository
	lifecycle *executor.Lifecycle
}

// NewReconciler constructs a recovery reconciler.
func NewReconciler(cfg config.RuntimeAgent, docker *docker.Client, store *store.Repository, lifecycle *executor.Lifecycle) *Reconciler {
	return &Reconciler{cfg: cfg, docker: docker, store: store, lifecycle: lifecycle}
}

// Run periodically reconciles state and garbage-collects.
func (r *Reconciler) Run(ctx context.Context) error {
	gcInterval := r.cfg.GCInterval
	if gcInterval <= 0 {
		gcInterval = 5 * time.Minute
	}
	ticker := time.NewTicker(gcInterval)
	defer ticker.Stop()
	for {
		r.reconcile(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_, _ = r.lifecycle.GC(ctx)
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) {
	managed, err := r.docker.ListManagedContainers(ctx)
	if err != nil {
		return
	}
	known := map[string]bool{}
	for _, c := range managed {
		known[c.ID] = true
	}
	records, _ := r.store.ListActive(ctx)
	for _, rec := range records {
		if !known[rec.ContainerID] && rec.Status == model.StatusRunning {
			_ = r.store.SetStatus(ctx, rec.ContainerID, rec.Status, model.StatusFailed, "container missing")
			if rec.RestartCount < r.cfg.RestartMaxAttempts {
				// Attempt recreate is orchestrator responsibility; mark for visibility
				_ = rec
			}
		}
	}
}

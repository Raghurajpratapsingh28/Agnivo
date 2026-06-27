package health

import (
	"context"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/docker"
	rtevents "github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/events"
	rtmetrics "github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/metrics"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/store"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
)

// Monitor collects container health and resource metrics.
type Monitor struct {
	cfg     config.RuntimeAgent
	docker  *docker.Client
	store   *store.Repository
	events  *rtevents.Publisher
	metrics *rtmetrics.Metrics
}

// NewMonitor constructs a health monitor.
func NewMonitor(cfg config.RuntimeAgent, docker *docker.Client, store *store.Repository, events *rtevents.Publisher, metrics *rtmetrics.Metrics) *Monitor {
	return &Monitor{cfg: cfg, docker: docker, store: store, events: events, metrics: metrics}
}

// Run periodically checks all active containers.
func (m *Monitor) Run(ctx context.Context) error {
	interval := m.cfg.HealthInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		m.checkAll(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (m *Monitor) checkAll(ctx context.Context) {
	containers, err := m.store.ListActive(ctx)
	if err != nil {
		return
	}
	var totalCPU float64
	var totalMem int64
	for _, c := range containers {
		if c.Status != model.StatusRunning {
			continue
		}
		cpu, mem, err := m.docker.ContainerStats(ctx, c.ContainerID)
		if err != nil {
			_ = m.store.RecordHealth(ctx, c.ContainerID, "liveness", false, 0, 0, "stats unavailable")
			continue
		}
		totalCPU += cpu
		totalMem += mem
		healthy := cpu < 95 && mem < m.cfg.MemoryMB*1024*1024
		_ = m.store.RecordHealth(ctx, c.ContainerID, "readiness", healthy, cpu, mem, "ok")
		_ = m.events.Publish(ctx, rtevents.HealthUpdated, rtevents.Meta{
			ContainerID: c.ContainerID, DeploymentID: c.DeploymentID,
		}, model.HealthReport{
			ContainerID: c.ContainerID, Healthy: healthy, CPUPercent: cpu, MemoryMB: mem,
			Restarts: c.RestartCount, OOMKilled: c.OOMKilled,
		})
	}
	if m.metrics != nil {
		m.metrics.SetCPU(totalCPU)
		m.metrics.SetMemory(float64(totalMem))
		m.metrics.SetActiveContainers(float64(len(containers)))
	}
	_ = m.events.Publish(ctx, rtevents.MetricsCollected, rtevents.Meta{}, map[string]any{
		"cpu_percent": totalCPU, "memory_mb": totalMem, "containers": len(containers),
	})
}

// Report returns health for a single container.
func (m *Monitor) Report(ctx context.Context, containerID string) (model.HealthReport, error) {
	rec, err := m.store.GetByContainerID(ctx, containerID)
	if err != nil {
		return model.HealthReport{}, err
	}
	cpu, mem, _ := m.docker.ContainerStats(ctx, containerID)
	return model.HealthReport{
		ContainerID: containerID, Healthy: rec.Status == model.StatusRunning,
		CPUPercent: cpu, MemoryMB: mem, Restarts: rec.RestartCount, OOMKilled: rec.OOMKilled,
	}, nil
}

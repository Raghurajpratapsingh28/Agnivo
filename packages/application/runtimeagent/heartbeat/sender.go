package heartbeat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	schedmodel "github.com/agnivo/agnivo/packages/application/scheduler/model"
	rtevents "github.com/agnivo/agnivo/packages/application/runtimeagent/events"
	"github.com/agnivo/agnivo/packages/application/runtimeagent/docker"
	"github.com/agnivo/agnivo/packages/application/runtimeagent/store"
	"github.com/agnivo/agnivo/packages/platform/config"
)

// Sender registers and heartbeats with the scheduler.
type Sender struct {
	cfg        config.RuntimeAgent
	nodeID     string
	httpClient *http.Client
	docker     *docker.Client
	store      *store.Repository
	events     *rtevents.Publisher
}

// NewSender constructs a heartbeat sender.
func NewSender(cfg config.RuntimeAgent, docker *docker.Client, store *store.Repository, events *rtevents.Publisher) *Sender {
	nodeID := os.Getenv("HOSTNAME")
	if nodeID == "" {
		nodeID = fmt.Sprintf("node-%d", time.Now().UnixNano())
	}
	return &Sender{
		cfg: cfg, nodeID: nodeID,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		docker: docker, store: store, events: events,
	}
}

// NodeID returns this agent's node identifier.
func (s *Sender) NodeID() string { return s.nodeID }

// Send builds and posts a heartbeat to the scheduler.
func (s *Sender) Send(ctx context.Context) error {
	if s.cfg.SchedulerURL == "" {
		return nil
	}
	dockerVer, kernelVer, _ := s.docker.Version(ctx)
	count, _ := s.store.CountActive(ctx)
	host := s.cfg.AdvertiseHost
	if host == "" {
		host = "127.0.0.1"
	}
	agentPort := s.cfg.AdvertisePort
	if agentPort <= 0 {
		agentPort = s.cfg.InternalPort
	}
	agentURL := fmt.Sprintf("http://%s:%d", host, agentPort)

	hb := schedmodel.HeartbeatPayload{
		NodeID: s.nodeID, Hostname: s.nodeID, AdvertiseHost: host, AgentURL: agentURL,
		Region: s.cfg.Region, AvailabilityZone: s.cfg.AvailabilityZone,
		InstanceType: s.cfg.InstanceType, Architecture: runtime.GOARCH,
		OS: runtime.GOOS, KernelVersion: kernelVer, DockerVersion: dockerVer,
		CPUCores: s.cfg.CPUCores, MemoryMB: s.cfg.MemoryMB, DiskGB: s.cfg.DiskGB,
		GPUCount: s.cfg.GPUCount, MaxContainers: s.cfg.MaxContainers, ContainerCount: count,
		HealthStatus: schedmodel.HealthHealthy,
		Labels: map[string]string{"agnivo.runtime_version": s.cfg.Version},
		LoadSummary: map[string]any{"containers": count},
		ResourceSummary: map[string]any{
			"cpu_cores": s.cfg.CPUCores, "memory_mb": s.cfg.MemoryMB, "disk_gb": s.cfg.DiskGB,
		},
	}
	body, _ := json.Marshal(hb)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.SchedulerURL+"/internal/v1/heartbeat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_ = s.events.Publish(ctx, rtevents.HeartbeatSent, rtevents.Meta{ServerID: s.nodeID}, map[string]int{"containers": count})
	return nil
}

// Run loops heartbeats until context cancellation.
func (s *Sender) Run(ctx context.Context) error {
	interval := s.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := s.Send(ctx); err != nil {
			// Heartbeat failures are retried on next tick
			_ = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

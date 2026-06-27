package docker

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// Client wraps the Docker SDK for runtime operations.
type Client struct {
	cli     *client.Client
	network string
	host    string
}

// NewClient constructs a Docker SDK client.
func NewClient(cfg config.RuntimeAgent) (*Client, error) {
	opts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	if cfg.DockerHost != "" {
		opts = append(opts, client.WithHost(cfg.DockerHost))
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeFailedPrecond, "runtime: docker client")
	}
	netName := cfg.DockerNetwork
	if netName == "" {
		netName = "agnivo"
	}
	host := cfg.AdvertiseHost
	if host == "" {
		host = "127.0.0.1"
	}
	return &Client{cli: cli, network: netName, host: host}, nil
}

// Close closes the Docker client.
func (c *Client) Close() error {
	if c.cli != nil {
		return c.cli.Close()
	}
	return nil
}

// Ping verifies Docker connectivity.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// Version returns Docker server version info.
func (c *Client) Version(ctx context.Context) (string, string, error) {
	v, err := c.cli.ServerVersion(ctx)
	if err != nil {
		return "", "", err
	}
	return v.Version, v.KernelVersion, nil
}

// EnsureNetwork creates the configured network if missing.
func (c *Client) EnsureNetwork(ctx context.Context) error {
	_, err := c.cli.NetworkInspect(ctx, c.network, dockertypes.NetworkInspectOptions{})
	if err == nil {
		return nil
	}
	_, err = c.cli.NetworkCreate(ctx, c.network, network.CreateOptions{Driver: "bridge"})
	return errors.Wrap(err, errors.CodeFailedPrecond, "runtime: create network")
}

// PullImage pulls an image with optional cache reuse.
func (c *Client) PullImage(ctx context.Context, ref string) error {
	reader, err := c.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return errors.Wrap(err, errors.CodeFailedPrecond, "runtime: pull image")
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

// CreateContainer creates a container without starting it.
func (c *Client) CreateContainer(ctx context.Context, req model.CreateRequest) (model.ContainerInfo, error) {
	if req.Image == "" {
		return model.ContainerInfo{}, errors.New(errors.CodeInvalidArgument, "runtime: empty image")
	}
	name := containerName(req.DeploymentID)
	port := req.Port
	if port <= 0 {
		port = 8080
	}
	exposed := nat.PortSet{nat.Port(fmt.Sprintf("%d/tcp", port)): struct{}{}}
	bindings := nat.PortMap{}
	if req.HostPort > 0 {
		bindings[nat.Port(fmt.Sprintf("%d/tcp", port))] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: strconv.Itoa(req.HostPort)}}
	}

	env := make([]string, 0, len(req.Env)+len(req.Secrets))
	for k, v := range req.Env {
		env = append(env, k+"="+v)
	}
	for k, v := range req.Secrets {
		env = append(env, k+"="+v)
	}
	labels := map[string]string{"agnivo.deployment_id": req.DeploymentID}
	for k, v := range req.Labels {
		labels[k] = v
	}

	netName := c.network
	if req.Network != "" {
		netName = req.Network
	}
	_ = netName

	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Image: req.Image, Env: env, Labels: labels, ExposedPorts: exposed,
	}, &container.HostConfig{
		PortBindings: bindings, RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		NetworkMode: container.NetworkMode(netName),
	}, &network.NetworkingConfig{}, nil, name)
	if err != nil {
		return model.ContainerInfo{}, errors.Wrap(err, errors.CodeFailedPrecond, "runtime: create container")
	}
	hostPort := req.HostPort
	return model.ContainerInfo{
		ID: resp.ID, Name: name, Host: c.host, Port: hostPort, Image: req.Image, Status: model.StatusCreated,
	}, nil
}

// StartContainer starts a created container.
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	return errors.Wrap(c.cli.ContainerStart(ctx, containerID, container.StartOptions{}),
		errors.CodeFailedPrecond, "runtime: start container")
}

// StopContainer stops a running container.
func (c *Client) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	secs := int(timeout.Seconds())
	if secs < 1 {
		secs = 10
	}
	return errors.Wrap(c.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &secs}),
		errors.CodeFailedPrecond, "runtime: stop container")
}

// RemoveContainer removes a container.
func (c *Client) RemoveContainer(ctx context.Context, containerID string) error {
	return errors.Wrap(c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}),
		errors.CodeFailedPrecond, "runtime: remove container")
}

// RestartContainer restarts a container.
func (c *Client) RestartContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	secs := int(timeout.Seconds())
	if secs < 1 {
		secs = 10
	}
	return errors.Wrap(c.cli.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: &secs}),
		errors.CodeFailedPrecond, "runtime: restart container")
}

// InspectContainer returns container state.
func (c *Client) InspectContainer(ctx context.Context, containerID string) (model.ContainerInfo, error) {
	inspect, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return model.ContainerInfo{}, errors.Wrap(err, errors.CodeNotFound, "runtime: inspect container")
	}
	hostPort := 0
	for _, bindings := range inspect.NetworkSettings.Ports {
		for _, b := range bindings {
			if p, err := strconv.Atoi(b.HostPort); err == nil {
				hostPort = p
				break
			}
		}
	}
	status := mapDockerStatus(inspect.State.Status)
	return model.ContainerInfo{
		ID: inspect.ID, Name: inspect.Name, Host: c.host, Port: hostPort,
		Image: inspect.Config.Image, Status: status,
	}, nil
}

// ContainerStats returns CPU/memory usage snapshot.
func (c *Client) ContainerStats(ctx context.Context, containerID string) (cpuPercent float64, memoryMB int64, err error) {
	stats, err := c.cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return 0, 0, err
	}
	defer stats.Body.Close()
	// Simplified: read one frame; production would parse JSON stream fully
	var data struct {
		MemoryStats struct {
			Usage uint64 `json:"usage"`
		} `json:"memory_stats"`
		CPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
		} `json:"cpu_stats"`
	}
	buf := make([]byte, 4096)
	n, _ := stats.Body.Read(buf)
	if n > 0 {
		_ = jsonUnmarshal(buf[:n], &data)
	}
	memoryMB = int64(data.MemoryStats.Usage / (1024 * 1024))
	if data.CPUStats.CPUUsage.TotalUsage > 0 {
		cpuPercent = float64(data.CPUStats.CPUUsage.TotalUsage) / 1e7
	}
	return cpuPercent, memoryMB, nil
}

// StreamLogs returns container log lines.
func (c *Client) StreamLogs(ctx context.Context, containerID string, tail int) ([]string, error) {
	if tail <= 0 {
		tail = 100
	}
	reader, err := c.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Tail: strconv.Itoa(tail),
	})
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(stripDockerLogHeader(line))
		if line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

// PruneImages removes dangling images.
func (c *Client) PruneImages(ctx context.Context) (int, error) {
	report, err := c.cli.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "true")))
	if err != nil {
		return 0, err
	}
	return len(report.ImagesDeleted), nil
}

// ListManagedContainers lists containers with agnivo labels.
func (c *Client) ListManagedContainers(ctx context.Context) ([]model.ContainerInfo, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	out := make([]model.ContainerInfo, 0)
	for _, ctr := range containers {
		if _, ok := ctr.Labels["agnivo.deployment_id"]; !ok && !strings.HasPrefix(ctr.Names[0], "/agnivo-") {
			continue
		}
		out = append(out, model.ContainerInfo{
			ID: ctr.ID, Name: strings.TrimPrefix(ctr.Names[0], "/"),
			Image: ctr.Image, Status: mapDockerStatus(ctr.State),
		})
	}
	return out, nil
}

func containerName(deploymentID string) string {
	if len(deploymentID) > 12 {
		deploymentID = deploymentID[:12]
	}
	return "agnivo-" + deploymentID
}

func mapDockerStatus(s string) model.ContainerStatus {
	switch strings.ToLower(s) {
	case "running":
		return model.StatusRunning
	case "created":
		return model.StatusCreated
	case "paused":
		return model.StatusPaused
	case "restarting":
		return model.StatusRestarting
	case "removing", "dead", "exited":
		return model.StatusStopped
	default:
		return model.ContainerStatus(s)
	}
}

func stripDockerLogHeader(s string) string {
	if len(s) > 8 {
		return s[8:]
	}
	return s
}

func jsonUnmarshal(data []byte, v any) error {
	return jsonDecode(data, v)
}

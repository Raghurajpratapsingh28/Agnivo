package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// ContainerInfo describes a running container.
type ContainerInfo struct {
	ID     string
	Name   string
	Host   string
	Port   int
	Image  string
	Status string
}

// Driver manages container lifecycle.
type Driver interface {
	Create(ctx context.Context, deploymentID string, cfg model.RuntimeConfig) (ContainerInfo, error)
	Start(ctx context.Context, containerID string) error
	Stop(ctx context.Context, containerID string, timeout time.Duration) error
	Remove(ctx context.Context, containerID string) error
	Inspect(ctx context.Context, containerID string) (ContainerInfo, error)
}

// DockerDriver uses the Docker CLI for container operations.
type DockerDriver struct {
	cli     string
	network string
}

// NewDockerDriver constructs a Docker runtime driver.
func NewDockerDriver(cfg config.Deployer) *DockerDriver {
	cli := cfg.DockerCLI
	if cli == "" {
		cli = "docker"
	}
	net := cfg.Network.DockerNetwork
	if net == "" {
		net = "bridge"
	}
	return &DockerDriver{cli: cli, network: net}
}

// Create creates a container without starting it.
func (d *DockerDriver) Create(ctx context.Context, deploymentID string, cfg model.RuntimeConfig) (ContainerInfo, error) {
	name := containerName(deploymentID)
	args := []string{"create", "--name", name, "--label", "agnivo.deployment_id=" + deploymentID}

	for k, v := range cfg.Labels {
		args = append(args, "--label", k+"="+v)
	}
	for k, v := range cfg.Env {
		args = append(args, "-e", k+"="+v)
	}
	for k, v := range cfg.Secrets {
		args = append(args, "-e", k+"="+v)
	}

	port := cfg.Port
	if port <= 0 {
		port = 8080
	}
	hostPort := cfg.HostPort
	if hostPort > 0 {
		args = append(args, "-p", fmt.Sprintf("%d:%d", hostPort, port))
	}

	if cfg.Network != "" {
		args = append(args, "--network", cfg.Network)
	} else if d.network != "" {
		args = append(args, "--network", d.network)
	}

	args = append(args, cfg.Image)

	out, err := exec.CommandContext(ctx, d.cli, args...).CombinedOutput()
	if err != nil {
		return ContainerInfo{}, errors.Wrap(err, errors.CodeFailedPrecond, "runtime: create container")
	}
	id := strings.TrimSpace(string(out))
	return ContainerInfo{ID: id, Name: name, Image: cfg.Image, Port: hostPort, Status: "created"}, nil
}

// Start starts a created container.
func (d *DockerDriver) Start(ctx context.Context, containerID string) error {
	out, err := exec.CommandContext(ctx, d.cli, "start", containerID).CombinedOutput()
	if err != nil {
		return errors.Wrap(err, errors.CodeFailedPrecond, "runtime: start: "+trimOut(out))
	}
	return nil
}

// Stop stops a running container with grace period.
func (d *DockerDriver) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	secs := int(timeout.Seconds())
	if secs < 1 {
		secs = 10
	}
	out, err := exec.CommandContext(ctx, d.cli, "stop", "-t", strconv.Itoa(secs), containerID).CombinedOutput()
	if err != nil {
		return errors.Wrap(err, errors.CodeFailedPrecond, "runtime: stop: "+trimOut(out))
	}
	return nil
}

// Remove removes a container.
func (d *DockerDriver) Remove(ctx context.Context, containerID string) error {
	out, err := exec.CommandContext(ctx, d.cli, "rm", "-f", containerID).CombinedOutput()
	if err != nil {
		return errors.Wrap(err, errors.CodeFailedPrecond, "runtime: remove: "+trimOut(out))
	}
	return nil
}

// Inspect returns container state.
func (d *DockerDriver) Inspect(ctx context.Context, containerID string) (ContainerInfo, error) {
	out, err := exec.CommandContext(ctx, d.cli, "inspect", "--format", "{{.Id}} {{.State.Status}}", containerID).CombinedOutput()
	if err != nil {
		return ContainerInfo{}, errors.Wrap(err, errors.CodeNotFound, "runtime: inspect")
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	info := ContainerInfo{ID: containerID, Status: "unknown"}
	if len(parts) >= 1 {
		info.ID = parts[0]
	}
	if len(parts) >= 2 {
		info.Status = parts[1]
	}
	return info, nil
}

func containerName(deploymentID string) string {
	if len(deploymentID) > 12 {
		deploymentID = deploymentID[:12]
	}
	return "agnivo-" + deploymentID
}

func trimOut(out []byte) string {
	s := string(out)
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

// AgentDriver delegates to runtime-agent HTTP API when configured.
type AgentDriver struct {
	baseURL string
	docker  *DockerDriver
	agent   *agentHTTP
}

// NewAgentDriver constructs a runtime driver with agent fallback.
func NewAgentDriver(cfg config.Deployer) Driver {
	docker := NewDockerDriver(cfg)
	if cfg.RuntimeAgentURL == "" {
		return docker
	}
	return &AgentDriver{baseURL: cfg.RuntimeAgentURL, docker: docker, agent: newAgentHTTP(cfg.RuntimeAgentURL)}
}

func (a *AgentDriver) Create(ctx context.Context, deploymentID string, cfg model.RuntimeConfig) (ContainerInfo, error) {
	if a.agent != nil {
		return a.agent.Create(ctx, deploymentID, cfg, a.baseURL)
	}
	return a.docker.Create(ctx, deploymentID, cfg)
}

func (a *AgentDriver) Start(ctx context.Context, containerID string) error {
	if a.agent != nil {
		return a.agent.Start(ctx, containerID)
	}
	return a.docker.Start(ctx, containerID)
}

func (a *AgentDriver) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	if a.agent != nil {
		return a.agent.Stop(ctx, containerID, timeout)
	}
	return a.docker.Stop(ctx, containerID, timeout)
}

func (a *AgentDriver) Remove(ctx context.Context, containerID string) error {
	if a.agent != nil {
		return a.agent.Remove(ctx, containerID)
	}
	return a.docker.Remove(ctx, containerID)
}

func (a *AgentDriver) Inspect(ctx context.Context, containerID string) (ContainerInfo, error) {
	if a.agent != nil {
		return a.agent.Inspect(ctx, containerID)
	}
	return a.docker.Inspect(ctx, containerID)
}

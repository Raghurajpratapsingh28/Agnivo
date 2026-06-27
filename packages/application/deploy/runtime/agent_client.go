package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	deploymodel "github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

type agentHTTP struct {
	baseURL    string
	httpClient *http.Client
}

func newAgentHTTP(baseURL string) *agentHTTP {
	return &agentHTTP{baseURL: trimSlash(baseURL), httpClient: &http.Client{Timeout: 120 * time.Second}}
}

func (c *agentHTTP) resolveBase(cfg deploymodel.RuntimeConfig, defaultURL string) string {
	if cfg.Annotations != nil {
		if u := cfg.Annotations["agnivo.agent_url"]; u != "" {
			return trimSlash(u)
		}
	}
	return c.baseURL
}

func (c *agentHTTP) Create(ctx context.Context, deploymentID string, cfg deploymodel.RuntimeConfig, defaultURL string) (ContainerInfo, error) {
	base := c.resolveBase(cfg, defaultURL)
	body, _ := json.Marshal(map[string]any{
		"deployment_id": deploymentID, "image": cfg.Image, "env": cfg.Env, "secrets": cfg.Secrets,
		"labels": cfg.Labels, "port": cfg.Port, "host_port": cfg.HostPort, "network": cfg.Network,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/internal/v1/runtime/containers", bytes.NewReader(body))
	if err != nil {
		return ContainerInfo{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doContainer(req)
}

func (c *agentHTTP) Start(ctx context.Context, containerID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/internal/v1/runtime/containers/%s/start", c.baseURL, containerID), nil)
	if err != nil {
		return err
	}
	return c.doOK(req)
}

func (c *agentHTTP) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	url := fmt.Sprintf("%s/internal/v1/runtime/containers/%s/stop?timeout=%d", c.baseURL, containerID, int(timeout.Seconds()))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	return c.doOK(req)
}

func (c *agentHTTP) Remove(ctx context.Context, containerID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/internal/v1/runtime/containers/%s", c.baseURL, containerID), nil)
	if err != nil {
		return err
	}
	return c.doOK(req)
}

func (c *agentHTTP) Inspect(ctx context.Context, containerID string) (ContainerInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/internal/v1/runtime/containers/%s", c.baseURL, containerID), nil)
	if err != nil {
		return ContainerInfo{}, err
	}
	return c.doContainer(req)
}

func (c *agentHTTP) doContainer(req *http.Request) (ContainerInfo, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ContainerInfo{}, errors.Wrap(err, errors.CodeUnavailable, "runtime agent: request")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return ContainerInfo{}, errors.New(errors.CodeFailedPrecond, "runtime agent: "+truncate(string(raw)))
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ContainerInfo{}, errors.Wrap(err, errors.CodeFailedPrecond, "runtime agent: decode")
	}
	var info ContainerInfo
	if len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, &info); err != nil {
			return ContainerInfo{}, errors.Wrap(err, errors.CodeFailedPrecond, "runtime agent: decode data")
		}
	}
	if info.ID == "" {
		if err := json.Unmarshal(raw, &info); err != nil {
			return ContainerInfo{}, errors.Wrap(err, errors.CodeFailedPrecond, "runtime agent: decode direct")
		}
	}
	return info, nil
}

func (c *agentHTTP) doOK(req *http.Request) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "runtime agent: request")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return errors.New(errors.CodeFailedPrecond, "runtime agent: "+truncate(string(raw)))
	}
	return nil
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func truncate(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

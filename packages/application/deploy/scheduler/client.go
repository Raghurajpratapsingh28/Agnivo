package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/agnivo/agnivo/packages/platform/errors"
)

// Placement is a scheduler placement decision.
type Placement struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	NodeID    string `json:"node_id"`
	Region    string `json:"region"`
	AgentURL  string `json:"agent_url,omitempty"`
	Reserved  bool   `json:"reserved"`
}

// Client requests placement from the scheduler service.
type Client struct {
	baseURL    string
	httpClient *http.Client
	localMode  bool
}

// NewClient constructs a scheduler client.
func NewClient(cfg config.Deployer) *Client {
	local := cfg.SchedulerURL == ""
	return &Client{
		baseURL: cfg.SchedulerURL,
		localMode: local,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Reserve requests resource reservation for a deployment.
func (c *Client) Reserve(ctx context.Context, orgID, projectID, deploymentID string, portMin, portMax int) (Placement, error) {
	if c.localMode {
		return c.localPlacement(portMin, portMax), nil
	}
	body, _ := json.Marshal(map[string]any{
		"org_id": orgID, "project_id": projectID, "deployment_id": deploymentID,
		"port_min": portMin, "port_max": portMax,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/v1/reserve", bytes.NewReader(body))
	if err != nil {
		return Placement{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return c.localPlacement(portMin, portMax), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return c.localPlacement(portMin, portMax), nil
	}
	var p Placement
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return c.localPlacement(portMin, portMax), nil
	}
	p.Reserved = true
	return p, nil
}

// Release releases reserved resources.
func (c *Client) Release(ctx context.Context, orgID, projectID, deploymentID string) error {
	if c.localMode {
		return nil
	}
	body, _ := json.Marshal(map[string]string{
		"org_id": orgID, "project_id": projectID, "deployment_id": deploymentID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/v1/release", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "scheduler: release")
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) localPlacement(portMin, portMax int) Placement {
	port := portMin
	if port <= 0 {
		port = 30000
	}
	return Placement{
		Host: "127.0.0.1", Port: port, NodeID: "local", Region: "local", Reserved: true,
	}
}

// FormatHostPort returns host:port for health checks.
func FormatHostPort(p Placement) string {
	return fmt.Sprintf("%s:%d", p.Host, p.Port)
}

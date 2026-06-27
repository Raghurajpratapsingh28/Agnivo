package ecr

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/agnivo/agnivo/packages/platform/errors"
)

// Client handles ECR authentication and image validation for pulls.
type Client struct {
	cfg       config.ECRConfig
	dockerCLI string
}

// NewClient constructs an ECR client.
func NewClient(cfg config.ECRConfig, dockerCLI string) *Client {
	if dockerCLI == "" {
		dockerCLI = "docker"
	}
	return &Client{cfg: cfg, dockerCLI: dockerCLI}
}

// Login authenticates Docker to ECR.
func (c *Client) Login(ctx context.Context) error {
	if !c.cfg.Enabled {
		return nil
	}
	region := c.region()
	registry := c.registry()
	pass, err := exec.CommandContext(ctx, "aws", "ecr", "get-login-password", "--region", region).Output()
	if err != nil {
		return errors.Wrap(err, errors.CodeUnauthenticated, "ecr: get login password")
	}
	cmd := exec.CommandContext(ctx, c.dockerCLI, "login", "--username", "AWS",
		"--password-stdin", registry)
	cmd.Stdin = strings.NewReader(strings.TrimSpace(string(pass)))
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, errors.CodeUnauthenticated, "ecr: docker login: "+sanitize(string(out)))
	}
	return nil
}

// ImageExists checks whether a tag exists in ECR.
func (c *Client) ImageExists(ctx context.Context, repository, tag string) (bool, error) {
	if !c.cfg.Enabled {
		return false, nil
	}
	_, err := exec.CommandContext(ctx, "aws", "ecr", "describe-images",
		"--repository-name", repository,
		"--image-ids", "imageTag="+tag,
		"--region", c.region()).CombinedOutput()
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (c *Client) registry() string {
	if c.cfg.Registry != "" {
		return c.cfg.Registry
	}
	return fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", "000000000000", c.region())
}

func (c *Client) region() string {
	if c.cfg.Region != "" {
		return c.cfg.Region
	}
	return "us-east-1"
}

func sanitize(s string) string {
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

package ecr

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// PushOptions configures an image push to ECR.
type PushOptions struct {
	LocalTag   string
	RemoteTags []string
	Repository string
	Region     string
	Labels     map[string]string
}

// PushResult is the outcome of a registry push.
type PushResult struct {
	Digest     string
	Repository string
	Tags       []string
	Duration   time.Duration
	Registry   string
}

// Client handles ECR authentication and image push.
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

// RepositoryName builds the ECR repository name for an org/project.
func (c *Client) RepositoryName(orgID, projectSlug string) string {
	prefix := c.cfg.RepositoryPrefix
	if prefix == "" {
		prefix = "agnivo"
	}
	slug := sanitizeRepoName(projectSlug)
	if slug == "" {
		slug = "app"
	}
	return fmt.Sprintf("%s/%s/%s", prefix, shorten(orgID), slug)
}

// FullImageRef returns the full registry image reference.
func (c *Client) FullImageRef(repository, tag string) string {
	registry := c.registry()
	return fmt.Sprintf("%s/%s:%s", registry, repository, tag)
}

// EnsureRepository creates the ECR repository if it does not exist.
func (c *Client) EnsureRepository(ctx context.Context, repository string) error {
	if !c.cfg.Enabled {
		return nil
	}
	region := c.region()
	out, err := exec.CommandContext(ctx, "aws", "ecr", "describe-repositories",
		"--repository-names", repository, "--region", region).CombinedOutput()
	if err == nil {
		_ = out
		return nil
	}
	_, err = exec.CommandContext(ctx, "aws", "ecr", "create-repository",
		"--repository-name", repository,
		"--image-scanning-configuration", "scanOnPush=true",
		"--encryption-configuration", "encryptionType=AES256",
		"--region", region).CombinedOutput()
	if err != nil {
		return errors.Wrap(err, errors.CodeFailedPrecond, "ecr: create repository")
	}
	return nil
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

// Push tags and pushes an image to ECR.
func (c *Client) Push(ctx context.Context, opts PushOptions) (PushResult, error) {
	start := time.Now()
	if !c.cfg.Enabled {
		return PushResult{
			Digest: opts.LocalTag, Repository: opts.Repository, Tags: opts.RemoteTags,
			Duration: time.Since(start), Registry: "local",
		}, nil
	}

	if err := c.Login(ctx); err != nil {
		return PushResult{}, err
	}
	if err := c.EnsureRepository(ctx, opts.Repository); err != nil {
		return PushResult{}, err
	}

	pushedTags := make([]string, 0, len(opts.RemoteTags))
	for _, tag := range opts.RemoteTags {
		remote := c.FullImageRef(opts.Repository, tag)
		if err := exec.CommandContext(ctx, c.dockerCLI, "tag", opts.LocalTag, remote).Run(); err != nil {
			return PushResult{}, errors.Wrap(err, errors.CodeFailedPrecond, "ecr: tag image")
		}
		out, err := exec.CommandContext(ctx, c.dockerCLI, "push", remote).CombinedOutput()
		if err != nil {
			return PushResult{}, errors.Wrap(err, errors.CodeFailedPrecond, "ecr: push: "+sanitize(string(out)))
		}
		pushedTags = append(pushedTags, tag)
	}

	digest := ""
	if len(pushedTags) > 0 {
		remote := c.FullImageRef(opts.Repository, pushedTags[0])
		out, err := exec.CommandContext(ctx, c.dockerCLI, "inspect", "--format={{index .RepoDigests 0}}", remote).Output()
		if err == nil {
			parts := strings.Split(strings.TrimSpace(string(out)), "@")
			if len(parts) == 2 {
				digest = parts[1]
			}
		}
	}

	return PushResult{
		Digest: digest, Repository: opts.Repository, Tags: pushedTags,
		Duration: time.Since(start), Registry: c.registry(),
	}, nil
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

func sanitizeRepoName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '/' {
			b.WriteRune(r)
		} else if r == ' ' {
			b.WriteRune('-')
		}
	}
	return b.String()
}

func shorten(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func sanitize(s string) string {
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

// BuildTags returns standard image tags for a deployment.
func BuildTags(commitSHA, deploymentID, branch string, tagLatest bool) []string {
	tags := []string{}
	if commitSHA != "" {
		tags = append(tags, commitSHA[:min(12, len(commitSHA))])
	}
	if deploymentID != "" {
		tags = append(tags, "dep-"+shorten(deploymentID))
	}
	if branch != "" {
		tags = append(tags, "branch-"+sanitizeRepoName(branch))
	}
	if tagLatest {
		tags = append(tags, "latest")
	}
	if len(tags) == 0 {
		tags = append(tags, "latest")
	}
	return tags
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

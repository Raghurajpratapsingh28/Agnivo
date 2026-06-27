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

// Puller retrieves images from ECR with authentication and digest verification.
type Puller struct {
	cfg       config.ECRConfig
	dockerCLI string
	login     *Client
}

// NewPuller constructs an image puller.
func NewPuller(cfg config.ECRConfig, dockerCLI string) *Puller {
	if dockerCLI == "" {
		dockerCLI = "docker"
	}
	return &Puller{cfg: cfg, dockerCLI: dockerCLI, login: NewClient(cfg, dockerCLI)}
}

// PullOptions configures an image pull.
type PullOptions struct {
	ImageRef       string
	ExpectedDigest string
	Platform       string
}

// PullResult is the outcome of a successful pull.
type PullResult struct {
	ImageRef string
	Digest   string
	Duration time.Duration
	Cached   bool
}

// Pull authenticates, pulls, and verifies an image.
func (p *Puller) Pull(ctx context.Context, opts PullOptions) (PullResult, error) {
	start := time.Now()
	if opts.ImageRef == "" {
		return PullResult{}, errors.New(errors.CodeInvalidArgument, "ecr pull: empty image ref")
	}

	if p.cfg.Enabled {
		if err := p.login.Login(ctx); err != nil {
			return PullResult{}, err
		}
	}

	args := []string{"pull"}
	if opts.Platform != "" {
		args = append(args, "--platform", opts.Platform)
	}
	args = append(args, opts.ImageRef)

	out, err := exec.CommandContext(ctx, p.dockerCLI, args...).CombinedOutput()
	if err != nil {
		return PullResult{}, errors.Wrap(err, errors.CodeFailedPrecond, "ecr pull: "+redact(string(out)))
	}

	digest, err := p.inspectDigest(ctx, opts.ImageRef)
	if err != nil {
		return PullResult{}, errors.Wrap(err, errors.CodeFailedPrecond, "ecr pull: digest inspect")
	}

	if opts.ExpectedDigest != "" && digest != "" && !strings.Contains(digest, opts.ExpectedDigest) && opts.ExpectedDigest != digest {
		if !strings.HasSuffix(digest, opts.ExpectedDigest) && !strings.HasSuffix(opts.ExpectedDigest, digest) {
			return PullResult{}, errors.New(errors.CodeFailedPrecond, "ecr pull: digest mismatch")
		}
	}

	cached := strings.Contains(strings.ToLower(string(out)), "image is up to date")
	return PullResult{
		ImageRef: opts.ImageRef, Digest: digest, Duration: time.Since(start), Cached: cached,
	}, nil
}

// Exists validates image presence in registry.
func (p *Puller) Exists(ctx context.Context, repository, tag string) (bool, error) {
	return p.login.ImageExists(ctx, repository, tag)
}

func (p *Puller) inspectDigest(ctx context.Context, ref string) (string, error) {
	out, err := exec.CommandContext(ctx, p.dockerCLI, "inspect", "--format={{index .RepoDigests 0}}", ref).Output()
	if err != nil {
		out, err = exec.CommandContext(ctx, p.dockerCLI, "inspect", "--format={{.Id}}", ref).Output()
		if err != nil {
			return "", err
		}
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "@")
	if len(parts) == 2 {
		return parts[1], nil
	}
	return strings.TrimSpace(string(out)), nil
}

func redact(s string) string {
	if len(s) > 300 {
		return s[:300] + "..."
	}
	return s
}

// LocalImageName returns a local tag for a deployment pull.
func LocalImageName(deploymentID string) string {
	return fmt.Sprintf("agnivo-deploy:%s", deploymentID)
}

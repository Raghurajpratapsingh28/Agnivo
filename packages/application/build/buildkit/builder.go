package buildkit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/cache"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// BuildOptions configures a container image build.
type BuildOptions struct {
	ContextDir   string
	Dockerfile   string
	Tags         []string
	BuildArgs    map[string]string
	Secrets      map[string]string
	CacheFrom    []string
	CacheTo      []string
	Platform     string
	Push         bool
	ProgressFunc func(stage, line string)
}

// BuildResult is the output of a successful build.
type BuildResult struct {
	Digest          string
	Tags            []string
	Duration        time.Duration
	CacheStats      *cache.Stats
	ImageSizeBytes  int64
	Warnings        []string
}

// Builder executes container builds via Docker BuildKit/buildx.
type Builder struct {
	cfg       config.Builder
	dockerCLI string
	buildkit  string
}

// NewBuilder constructs a BuildKit-backed builder.
func NewBuilder(cfg config.Builder) *Builder {
	cli := cfg.DockerCLI
	if cli == "" {
		cli = "docker"
	}
	return &Builder{cfg: cfg, dockerCLI: cli, buildkit: cfg.BuildKitAddr}
}

// Build runs a multi-stage BuildKit build with cache and secrets support.
func (b *Builder) Build(ctx context.Context, opts BuildOptions) (BuildResult, error) {
	start := time.Now()
	if opts.Dockerfile == "" {
		opts.Dockerfile = "Dockerfile"
	}
	if opts.Platform == "" {
		opts.Platform = "linux/amd64"
	}

	args := []string{"buildx", "build", "--progress=plain", "--platform", opts.Platform}
	for _, tag := range opts.Tags {
		args = append(args, "-t", tag)
	}
	args = append(args, "-f", opts.Dockerfile, opts.ContextDir)

	for k, v := range opts.BuildArgs {
		args = append(args, "--build-arg", k+"="+v)
	}

	secretDir, cleanup, err := b.writeSecrets(opts.Secrets)
	if err != nil {
		return BuildResult{}, err
	}
	defer cleanup()

	for id, path := range secretDir {
		args = append(args, "--secret", "id="+id+",src="+path)
	}

	if b.cfg.Cache.Enabled {
		for _, ref := range opts.CacheFrom {
			args = append(args, "--cache-from", ref)
		}
		for _, ref := range opts.CacheTo {
			args = append(args, "--cache-to", ref)
		}
		if b.cfg.Cache.InlineCache {
			args = append(args, "--build-arg", "BUILDKIT_INLINE_CACHE=1")
		}
	}

	if opts.Push {
		args = append(args, "--push")
	} else {
		args = append(args, "--load")
	}

	env := os.Environ()
	env = append(env, "DOCKER_BUILDKIT=1")
	if b.buildkit != "" {
		env = append(env, "BUILDKIT_HOST="+b.buildkit)
	}

	cmd := exec.CommandContext(ctx, b.dockerCLI, args...)
	cmd.Env = env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return BuildResult{}, errors.Wrap(err, errors.CodeInternal, "buildkit: stdout pipe")
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return BuildResult{}, errors.Wrap(err, errors.CodeInternal, "buildkit: stderr pipe")
	}

	if err := cmd.Start(); err != nil {
		return BuildResult{}, errors.Wrap(err, errors.CodeFailedPrecond, "buildkit: start build")
	}

	stats := &cache.Stats{}
	warnings := []string{}
	scan := func(r *bufio.Scanner) {
		for r.Scan() {
			line := r.Text()
			if opts.ProgressFunc != nil {
				opts.ProgressFunc("build", line)
			}
			stats.RecordLine(line)
			if strings.Contains(strings.ToLower(line), "warning") {
				warnings = append(warnings, line)
			}
		}
	}
	go scan(bufio.NewScanner(stdout))
	go scan(bufio.NewScanner(stderr))

	if err := cmd.Wait(); err != nil {
		return BuildResult{}, errors.Wrap(err, errors.CodeFailedPrecond, "buildkit: build failed")
	}

	digest := ""
	if len(opts.Tags) > 0 {
		digest, _ = b.inspectDigest(ctx, opts.Tags[0])
	}

	return BuildResult{
		Digest: digest, Tags: opts.Tags, Duration: time.Since(start),
		CacheStats: stats, Warnings: warnings,
	}, nil
}

func (b *Builder) writeSecrets(secrets map[string]string) (map[string]string, func(), error) {
	paths := make(map[string]string, len(secrets))
	var files []string
	for id, val := range secrets {
		f, err := os.CreateTemp("", "agnivo-build-secret-*")
		if err != nil {
			for _, p := range files {
				_ = os.Remove(p)
			}
			return nil, nil, err
		}
		if _, err := f.WriteString(val); err != nil {
			_ = f.Close()
			return nil, nil, err
		}
		_ = f.Close()
		_ = os.Chmod(f.Name(), 0o600)
		paths[id] = f.Name()
		files = append(files, f.Name())
	}
	cleanup := func() {
		for _, p := range files {
			_ = os.Remove(p)
		}
	}
	return paths, cleanup, nil
}

func (b *Builder) inspectDigest(ctx context.Context, tag string) (string, error) {
	out, err := exec.CommandContext(ctx, b.dockerCLI, "inspect", "--format={{index .RepoDigests 0}}", tag).Output()
	if err != nil {
		// Fallback: image id
		out, err = exec.CommandContext(ctx, b.dockerCLI, "inspect", "--format={{.Id}}", tag).Output()
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

// CacheRefs returns cache-from and cache-to references for a repository.
func CacheRefs(cfg config.Builder, repository string) (from, to []string) {
	if !cfg.Cache.Enabled {
		return nil, nil
	}
	ref := cfg.Cache.RegistryRef
	if ref == "" && cfg.ECR.Enabled && cfg.ECR.Registry != "" {
		ref = fmt.Sprintf("%s/%s:buildcache", cfg.ECR.Registry, repository)
	}
	if ref == "" {
		return nil, nil
	}
	from = []string{ref}
	mode := "mode=max"
	if cfg.Cache.InlineCache {
		to = []string{ref + "," + mode + ",inline=true"}
	} else {
		to = []string{ref + "," + mode}
	}
	return from, to
}

// ParseBuildOutput extracts cache stats from buildx JSON lines when present.
func ParseBuildOutput(line string) (*cache.Stats, bool) {
	var msg struct {
		Vertexes []struct {
			Digest string `json:"digest"`
			Cached bool   `json:"cached"`
		} `json:"vertexes"`
	}
	if !json.Valid([]byte(line)) {
		return nil, false
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil, false
	}
	s := &cache.Stats{}
	for _, v := range msg.Vertexes {
		if v.Cached {
			s.RecordLine("cached")
		} else {
			s.RecordLine("exporting layer")
		}
	}
	return s, s.TotalLayers() > 0
}

// WriteBuildContext validates context directory exists.
func WriteBuildContext(dir string) error {
	if dir == "" {
		return errors.New(errors.CodeInvalidArgument, "buildkit: empty context")
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return errors.New(errors.CodeInvalidArgument, "buildkit: invalid context dir")
	}
	df := filepath.Join(dir, "Dockerfile")
	if _, err := os.Stat(df); err != nil {
		return errors.New(errors.CodeInvalidArgument, "buildkit: Dockerfile missing")
	}
	return nil
}

// FormatTags joins tags for logging without secrets.
func FormatTags(tags []string) string {
	var b bytes.Buffer
	for i, t := range tags {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(t)
	}
	return b.String()
}

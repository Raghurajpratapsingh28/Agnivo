package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/buildkit"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/buildstore"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/cancel"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/detect"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/dockerfile"
	buildevents "github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/ecr"
	buildgit "github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/git"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/logs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/metrics"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/sbom"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/crypto"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpjobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/deployment"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/envvar"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/gitrepo"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/project"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/logger"
)

// Pipeline orchestrates the full build flow.
type Pipeline struct {
	cfg          config.Builder
	vault        *crypto.Vault
	projects     *project.Repository
	deployments  *deployment.Repository
	gitStore     *gitrepo.Store
	envRepo      *envvar.Repository
	builds       *buildstore.Repository
	artifacts    *buildstore.ArtifactRepository
	events       *buildevents.Publisher
	logs         *logs.Streamer
	git          *buildgit.Manager
	detector     *detect.Detector
	dfgen        *dockerfile.Generator
	builder      *buildkit.Builder
	ecr          *ecr.Client
	sbom         *sbom.Generator
	metrics      *metrics.Metrics
	cancels      *cancel.Registry
	workerID     string
}

// Deps wires pipeline dependencies.
type Deps struct {
	Config      config.Builder
	Vault       *crypto.Vault
	Projects    *project.Repository
	Deployments *deployment.Repository
	GitStore    *gitrepo.Store
	EnvRepo     *envvar.Repository
	Builds      *buildstore.Repository
	Artifacts   *buildstore.ArtifactRepository
	Events      *buildevents.Publisher
	Logs        *logs.Streamer
	Git         *buildgit.Manager
	Builder     *buildkit.Builder
	ECR         *ecr.Client
	Metrics     *metrics.Metrics
	Cancels     *cancel.Registry
	WorkerID    string
}

// NewPipeline constructs a build pipeline.
func NewPipeline(d Deps) *Pipeline {
	return &Pipeline{
		cfg: d.Config, vault: d.Vault, projects: d.Projects, deployments: d.Deployments,
		gitStore: d.GitStore, envRepo: d.EnvRepo, builds: d.Builds, artifacts: d.Artifacts,
		events: d.Events, logs: d.Logs, git: d.Git, detector: detect.NewDetector(),
		dfgen: dockerfile.NewGenerator(), builder: d.Builder, ecr: d.ECR,
		sbom: sbom.NewGenerator(), metrics: d.Metrics, cancels: d.Cancels, workerID: d.WorkerID,
	}
}

// Run executes a build job end-to-end.
func (p *Pipeline) Run(ctx context.Context, payload cpjobs.Payload, jobID string) error {
	start := time.Now()
	p.metrics.IncActive()
	defer p.metrics.DecActive()

	ctx, cancelFn := context.WithCancel(ctx)
	defer cancelFn()
	p.cancels.Register(payload.DeploymentID, cancelFn)
	defer p.cancels.Unregister(payload.DeploymentID)

	if payload.DeploymentID == "" {
		return errors.New(errors.CodeInvalidArgument, "build: missing deployment_id")
	}

	dep, err := p.deployments.GetByID(ctx, payload.OrgID, payload.DeploymentID)
	if err != nil {
		return err
	}
	if cancelled, _ := p.deployments.IsCancelled(ctx, payload.OrgID, payload.DeploymentID); cancelled {
		return p.handleCancel(ctx, payload, "deployment already cancelled")
	}

	proj, err := p.projects.GetByID(ctx, payload.OrgID, payload.ProjectID)
	if err != nil {
		return err
	}

	jobIDPtr := &jobID
	buildRec, err := p.builds.UpsertForDeployment(ctx, model.Build{
		OrgID: payload.OrgID, ProjectID: payload.ProjectID, DeploymentID: payload.DeploymentID,
		JobID: jobIDPtr, Status: model.StatusQueued, CommitSHA: dep.CommitSHA, Branch: dep.Branch,
		Environment: dep.Environment, CorrelationID: payload.CorrelationID,
	})
	if err != nil {
		return err
	}

	meta := buildevents.Meta{
		BuildID: buildRec.ID, DeploymentID: payload.DeploymentID,
		OrgID: payload.OrgID, ProjectID: payload.ProjectID, CorrelationID: payload.CorrelationID,
	}
	_ = p.events.Publish(ctx, buildevents.BuildQueued, meta, nil)

	if _, err := p.deployments.UpdateStatus(ctx, payload.OrgID, payload.DeploymentID, deployment.StatusBuilding, "build started", ""); err != nil {
		return err
	}
	_ = p.builds.MarkRunning(ctx, buildRec.ID, p.workerID)
	_ = p.events.Publish(ctx, buildevents.BuildStarted, meta, nil)
	_ = p.logs.Info(ctx, buildRec.ID, payload.DeploymentID, "init", "build started")

	if err := p.checkCancel(ctx, payload); err != nil {
		return err
	}

	// Git clone
	repo, err := p.gitStore.GetWithCredentials(ctx, payload.OrgID, payload.ProjectID)
	if err != nil {
		return p.fail(ctx, payload, buildRec.ID, meta, start, "repository not connected: "+err.Error())
	}
	creds, err := buildgit.DecryptCredentials(p.vault, payload.OrgID, payload.ProjectID, repo)
	if err != nil {
		return p.fail(ctx, payload, buildRec.ID, meta, start, "credential decryption failed")
	}
	creds.CloneURL = buildgit.ResolveCloneURL(repo)
	creds.Branch = firstNonEmpty(dep.Branch, repo.DefaultBranch, proj.Branch)
	creds.CommitSHA = dep.CommitSHA

	cloneRes, err := p.git.Clone(ctx, creds, payload.DeploymentID)
	if err != nil {
		return p.fail(ctx, payload, buildRec.ID, meta, start, "git clone failed: "+err.Error())
	}
	defer func() { _ = p.git.Cleanup(cloneRes.WorkspaceDir) }()
	p.metrics.ObserveClone(cloneRes.Duration.Seconds())
	_ = p.events.Publish(ctx, buildevents.RepositoryCloned, meta, map[string]any{
		"commit_sha": cloneRes.CommitSHA, "duration_ms": cloneRes.Duration.Milliseconds(),
	})
	_ = p.logs.Info(ctx, buildRec.ID, payload.DeploymentID, "clone", fmt.Sprintf("cloned %s@%s", cloneRes.Branch, cloneRes.CommitSHA))

	if err := p.checkCancel(ctx, payload); err != nil {
		return err
	}

	// Framework detection
	fw := p.detector.Detect(cloneRes.WorkspaceDir)
	if proj.Framework != "" {
		fw.Name = proj.Framework
	}
	if proj.DefaultRuntime != "" {
		fw.Runtime = proj.DefaultRuntime
	}
	_ = p.events.Publish(ctx, buildevents.FrameworkDetected, meta, map[string]string{
		"framework": fw.Name, "runtime": fw.Runtime, "source": fw.Source,
	})
	_ = p.logs.Info(ctx, buildRec.ID, payload.DeploymentID, "detect", fmt.Sprintf("detected %s (%s)", fw.Name, fw.Runtime))

	// Dockerfile
	dfRes, err := p.dfgen.Generate(cloneRes.WorkspaceDir, fw)
	if err != nil {
		return p.fail(ctx, payload, buildRec.ID, meta, start, "dockerfile generation failed")
	}
	if dfRes.Generated {
		_ = p.events.Publish(ctx, buildevents.DockerfileGenerated, meta, map[string]string{"version": dfRes.Version})
		_ = p.logs.Info(ctx, buildRec.ID, payload.DeploymentID, "dockerfile", "generated optimized Dockerfile")
	} else {
		_ = p.logs.Info(ctx, buildRec.ID, payload.DeploymentID, "dockerfile", "using custom Dockerfile")
	}

	if err := p.checkCancel(ctx, payload); err != nil {
		return err
	}

	// Build args and secrets from env vars
	buildArgs, secrets, err := p.loadBuildEnv(ctx, payload.OrgID, payload.ProjectID, dep.Environment)
	if err != nil {
		return p.fail(ctx, payload, buildRec.ID, meta, start, "load build environment failed")
	}

	repository := p.ecr.RepositoryName(payload.OrgID, proj.Slug)
	tags := ecr.BuildTags(cloneRes.CommitSHA, payload.DeploymentID, cloneRes.Branch, p.cfg.TagLatest)
	localTag := fmt.Sprintf("agnivo-build:%s", payload.DeploymentID)
	remoteTags := tags
	cacheFrom, cacheTo := buildkit.CacheRefs(p.cfg, repository)
	fullTags := []string{localTag}
	for _, t := range remoteTags {
		fullTags = append(fullTags, p.ecr.FullImageRef(repository, t))
	}

	buildRes, err := p.builder.Build(ctx, buildkit.BuildOptions{
		ContextDir: cloneRes.WorkspaceDir,
		Dockerfile: dfRes.Path,
		Tags:       fullTags,
		BuildArgs:  buildArgs,
		Secrets:    secrets,
		CacheFrom:  cacheFrom,
		CacheTo:    cacheTo,
		Push:       false,
		ProgressFunc: func(stage, line string) {
			_ = p.logs.Write(ctx, logs.Entry{
				BuildID: buildRec.ID, DeploymentID: payload.DeploymentID,
				Stage: stage, Level: logs.LevelInfo, Message: line,
			})
		},
	})
	if err != nil {
		return p.fail(ctx, payload, buildRec.ID, meta, start, "docker build failed: "+err.Error())
	}
	p.metrics.ObserveDocker(buildRes.Duration.Seconds())
	p.metrics.ObserveCacheHit(buildRes.CacheStats.HitRatio())

	if err := p.checkCancel(ctx, payload); err != nil {
		return err
	}

	// Push to ECR
	pushRes, err := p.ecr.Push(ctx, ecr.PushOptions{
		LocalTag: localTag, RemoteTags: remoteTags, Repository: repository,
	})
	if err != nil {
		return p.fail(ctx, payload, buildRec.ID, meta, start, "registry push failed: "+err.Error())
	}
	p.metrics.ObservePush(pushRes.Duration.Seconds())
	digest := firstNonEmpty(pushRes.Digest, buildRes.Digest)
	imageTag := remoteTags[0]
	if pushRes.Registry != "local" {
		imageTag = p.ecr.FullImageRef(repository, remoteTags[0])
	}
	_ = p.events.Publish(ctx, buildevents.ImagePushed, meta, map[string]any{
		"digest": digest, "tags": remoteTags, "registry": pushRes.Registry,
	})
	_ = p.logs.Info(ctx, buildRec.ID, payload.DeploymentID, "push", fmt.Sprintf("pushed %s", imageTag))

	sbomDoc := p.sbom.Generate(localTag)
	signMeta := sbom.SigningHook(imageTag, digest)
	warnings, _ := json.Marshal(buildRes.Warnings)
	artifactMeta, _ := json.Marshal(map[string]any{"signing": signMeta})

	totalMs := time.Since(start).Milliseconds()
	_, err = p.artifacts.Save(ctx, model.Artifact{
		BuildID: buildRec.ID, DeploymentID: payload.DeploymentID,
		OrgID: payload.OrgID, ProjectID: payload.ProjectID,
		ImageDigest: digest, ImageTag: imageTag, Registry: pushRes.Registry, Repository: repository,
		Framework: fw.Name, Runtime: fw.Runtime, DockerfileVersion: dfRes.Version,
		BuilderVersion: p.cfg.Version, CommitSHA: cloneRes.CommitSHA, Branch: cloneRes.Branch,
		BuildDurationMs: totalMs, CloneDurationMs: cloneRes.Duration.Milliseconds(),
		DockerDurationMs: buildRes.Duration.Milliseconds(), PushDurationMs: pushRes.Duration.Milliseconds(),
		CacheHitRatio: buildRes.CacheStats.HitRatio(),
		CacheLayersHit: buildRes.CacheStats.HitLayers(), CacheLayersTotal: buildRes.CacheStats.TotalLayers(),
		Warnings: warnings, SBOM: sbomDoc, Metadata: artifactMeta,
	})
	if err != nil {
		return p.fail(ctx, payload, buildRec.ID, meta, start, "artifact persistence failed")
	}

	buildMeta, _ := json.Marshal(map[string]any{
		"framework": fw.Name, "image_digest": digest, "builder_version": p.cfg.Version,
	})
	_, err = p.deployments.UpdateBuildComplete(ctx, payload.OrgID, payload.DeploymentID, deployment.BuildResult{
		ImageTag: imageTag, ImageDigest: digest, Runtime: fw.Runtime, Framework: fw.Name,
		BuildDurationMs: totalMs, Metadata: buildMeta,
	})
	if err != nil {
		return err
	}

	_ = p.builds.MarkSucceeded(ctx, buildRec.ID, fw.Name, fw.Runtime)
	_ = p.events.Publish(ctx, buildevents.BuildSucceeded, meta, map[string]any{
		"image_tag": imageTag, "digest": digest, "duration_ms": totalMs,
	})
	p.metrics.IncSuccess()
	p.metrics.ObserveBuild(fw.Name, "success", time.Since(start).Seconds())
	_ = p.logs.Info(ctx, buildRec.ID, payload.DeploymentID, "complete", "build succeeded")
	return nil
}

func (p *Pipeline) fail(ctx context.Context, payload cpjobs.Payload, buildID string, meta buildevents.Meta, start time.Time, reason string) error {
	totalMs := time.Since(start).Milliseconds()
	_, _ = p.deployments.MarkBuildFailed(ctx, payload.OrgID, payload.DeploymentID, reason, totalMs)
	_ = p.builds.MarkFailed(ctx, buildID, reason)
	_ = p.events.Publish(ctx, buildevents.BuildFailed, meta, map[string]string{"reason": reason})
	p.metrics.IncFailure()
	p.metrics.ObserveBuild("unknown", "failed", time.Since(start).Seconds())
	_ = p.logs.Error(ctx, buildID, payload.DeploymentID, "error", reason)
	return errors.New(errors.CodeFailedPrecond, reason)
}

func (p *Pipeline) handleCancel(ctx context.Context, payload cpjobs.Payload, msg string) error {
	buildRec, _ := p.builds.GetByDeployment(ctx, payload.DeploymentID)
	meta := buildevents.Meta{
		BuildID: buildRec.ID, DeploymentID: payload.DeploymentID,
		OrgID: payload.OrgID, ProjectID: payload.ProjectID, CorrelationID: payload.CorrelationID,
	}
	_ = p.builds.MarkCancelled(ctx, buildRec.ID)
	_ = p.events.Publish(ctx, buildevents.BuildCancelled, meta, map[string]string{"reason": msg})
	_ = p.logs.Warn(ctx, buildRec.ID, payload.DeploymentID, "cancel", msg)
	return errors.New(errors.CodeCanceled, msg)
}

func (p *Pipeline) checkCancel(ctx context.Context, payload cpjobs.Payload) error {
	cancelled, err := p.deployments.IsCancelled(ctx, payload.OrgID, payload.DeploymentID)
	if err != nil {
		return err
	}
	if cancelled || ctx.Err() != nil {
		return p.handleCancel(ctx, payload, "build cancelled")
	}
	return nil
}

func (p *Pipeline) loadBuildEnv(ctx context.Context, orgID, projectID, environment string) (map[string]string, map[string]string, error) {
	buildArgs := map[string]string{"BUILDKIT_INLINE_CACHE": "1"}
	secrets := map[string]string{}
	if p.envRepo == nil || p.vault == nil {
		return buildArgs, secrets, nil
	}
	vars, err := p.envRepo.List(ctx, orgID, projectID, envvar.Scope(environment))
	if err != nil {
		return nil, nil, err
	}
	aad := crypto.AAD(orgID, projectID)
	for _, v := range vars {
		plain, err := p.vault.Decrypt(v.ValueEnc, aad)
		if err != nil {
			continue
		}
		val := string(plain)
		if v.IsSecret {
			secrets[v.Key] = val
		} else {
			buildArgs[v.Key] = val
		}
	}
	return buildArgs, secrets, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// WithCorrelation attaches correlation ID to context.
func WithCorrelation(ctx context.Context, correlationID string) context.Context {
	if correlationID == "" {
		return ctx
	}
	return logger.WithCorrelationID(ctx, correlationID)
}

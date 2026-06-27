package deployment

import (
	"context"
	"fmt"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpevents"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpjobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/project"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/audit"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/rbac"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/tenant"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/logger"
)

// Service orchestrates deployment lifecycle.
type Service struct {
	deployments *Repository
	projects    *project.Repository
	jobs        *cpjobs.Enqueuer
	events      *cpevents.Publisher
	audit       *audit.Logger
}

// NewService constructs a deployment service.
func NewService(deployments *Repository, projects *project.Repository, jobs *cpjobs.Enqueuer, events *cpevents.Publisher, auditLog *audit.Logger) *Service {
	return &Service{deployments: deployments, projects: projects, jobs: jobs, events: events, audit: auditLog}
}

// DeployInput triggers a new deployment.
type DeployInput struct {
	CommitSHA     string `json:"commit_sha" validate:"omitempty,max=64"`
	CommitMessage string `json:"commit_message" validate:"omitempty,max=2000"`
	Branch        string `json:"branch" validate:"omitempty,max=255"`
	Author        string `json:"author" validate:"omitempty,max=255"`
	Environment   string `json:"environment" validate:"omitempty,oneof=development preview production"`
}

// Deploy creates a deployment and enqueues build/deploy jobs.
func (s *Service) Deploy(ctx context.Context, orgID, projectID, userID string, in DeployInput, ip, ua string) (Deployment, error) {
	if err := s.requireDeployWrite(ctx, orgID); err != nil {
		return Deployment{}, err
	}
	proj, err := s.projects.GetByID(ctx, orgID, projectID)
	if err != nil {
		return Deployment{}, err
	}
	if !proj.IsLive() {
		return Deployment{}, errors.FailedPrecondition("project is not active")
	}
	branch := in.Branch
	if branch == "" {
		branch = proj.Branch
	}
	env := in.Environment
	if env == "" {
		env = "production"
	}
	corrID := logger.CorrelationID(ctx)
	uid := userID
	d, err := s.deployments.Create(ctx, Deployment{
		OrgID: orgID, ProjectID: projectID, Status: StatusPending,
		CommitSHA: in.CommitSHA, CommitMessage: in.CommitMessage, Branch: branch,
		Author: in.Author, Runtime: proj.DefaultRuntime, TriggerSource: "manual",
		TriggerUserID: &uid, Environment: env, CorrelationID: corrID,
	})
	if err != nil {
		return Deployment{}, err
	}
	payload := cpjobs.Payload{
		OrgID: orgID, ProjectID: projectID, DeploymentID: d.ID,
		UserID: userID, CorrelationID: corrID,
	}
	idemKey := fmt.Sprintf("deploy:%s:%s", projectID, d.ID)
	_, err = s.jobs.EnqueueBuild(ctx, payload, idemKey+":build")
	if err != nil {
		return Deployment{}, err
	}
	d, err = s.deployments.UpdateStatus(ctx, orgID, d.ID, StatusQueued, "build job enqueued", "")
	if err != nil {
		return Deployment{}, err
	}
	_, _ = s.jobs.EnqueueDeploy(ctx, payload, idemKey+":deploy")
	s.recordAudit(ctx, userID, orgID, "deployment.deploy", d.ID, ip, ua)
	_ = s.events.PublishAsync(ctx, cpevents.DeploymentRequested, cpevents.Meta{
		OrgID: orgID, ProjectID: projectID, AggregateID: d.ID, ActorID: userID, CorrelationID: corrID,
	}, d)
	return d, nil
}

// Redeploy re-triggers deployment from an existing record.
func (s *Service) Redeploy(ctx context.Context, orgID, projectID, deploymentID, userID, ip, ua string) (Deployment, error) {
	prev, err := s.deployments.GetByID(ctx, orgID, deploymentID)
	if err != nil {
		return Deployment{}, err
	}
	return s.Deploy(ctx, orgID, projectID, userID, DeployInput{
		CommitSHA: prev.CommitSHA, CommitMessage: prev.CommitMessage,
		Branch: prev.Branch, Author: prev.Author, Environment: prev.Environment,
	}, ip, ua)
}

// Rollback rolls back to a previous deployment.
func (s *Service) Rollback(ctx context.Context, orgID, projectID, targetDeploymentID, userID, ip, ua string) (Deployment, error) {
	if err := s.requireDeployWrite(ctx, orgID); err != nil {
		return Deployment{}, err
	}
	target, err := s.deployments.GetByID(ctx, orgID, targetDeploymentID)
	if err != nil {
		return Deployment{}, err
	}
	if target.ProjectID != projectID {
		return Deployment{}, errors.NotFound("deployment not found")
	}
	corrID := logger.CorrelationID(ctx)
	uid := userID
	d, err := s.deployments.Create(ctx, Deployment{
		OrgID: orgID, ProjectID: projectID, Status: StatusRollingBack,
		CommitSHA: target.CommitSHA, CommitMessage: target.CommitMessage,
		Branch: target.Branch, Author: target.Author, ImageTag: target.ImageTag,
		Runtime: target.Runtime, TriggerSource: "rollback", TriggerUserID: &uid,
		Environment: target.Environment, CorrelationID: corrID,
	})
	if err != nil {
		return Deployment{}, err
	}
	payload := cpjobs.Payload{OrgID: orgID, ProjectID: projectID, DeploymentID: d.ID, UserID: userID, CorrelationID: corrID}
	_, err = s.jobs.EnqueueRollback(ctx, payload, "rollback:"+d.ID)
	if err != nil {
		return Deployment{}, err
	}
	s.recordAudit(ctx, userID, orgID, "deployment.rollback", d.ID, ip, ua)
	return d, nil
}

// Cancel cancels an in-progress deployment.
func (s *Service) Cancel(ctx context.Context, orgID, deploymentID, userID, ip, ua string) (Deployment, error) {
	if err := s.requireDeployWrite(ctx, orgID); err != nil {
		return Deployment{}, err
	}
	d, err := s.deployments.GetByID(ctx, orgID, deploymentID)
	if err != nil {
		return Deployment{}, err
	}
	if d.IsTerminal() {
		return Deployment{}, errors.FailedPrecondition("deployment already finished")
	}
	payload := cpjobs.Payload{OrgID: orgID, ProjectID: d.ProjectID, DeploymentID: d.ID, UserID: userID}
	_, _ = s.jobs.EnqueueCancel(ctx, payload)
	d, err = s.deployments.UpdateStatus(ctx, orgID, deploymentID, StatusCancelled, "cancelled by user", "")
	if err != nil {
		return Deployment{}, err
	}
	s.recordAudit(ctx, userID, orgID, "deployment.cancel", deploymentID, ip, ua)
	_ = s.events.PublishAsync(ctx, cpevents.DeploymentCancelled, cpevents.Meta{
		OrgID: orgID, ProjectID: d.ProjectID, AggregateID: deploymentID, ActorID: userID,
	}, d)
	return d, nil
}

// Restart enqueues a wake/restart for the project's latest live deployment.
func (s *Service) Restart(ctx context.Context, orgID, projectID, userID, ip, ua string) error {
	if err := s.requireDeployWrite(ctx, orgID); err != nil {
		return err
	}
	latest, err := s.deployments.GetLatest(ctx, orgID, projectID)
	if err != nil {
		return err
	}
	payload := cpjobs.Payload{OrgID: orgID, ProjectID: projectID, DeploymentID: latest.ID, UserID: userID}
	_, err = s.jobs.EnqueueWake(ctx, payload)
	if err != nil {
		return err
	}
	s.recordAudit(ctx, userID, orgID, "deployment.restart", latest.ID, ip, ua)
	return nil
}

// Pause puts a project to sleep.
func (s *Service) Pause(ctx context.Context, orgID, projectID, userID, ip, ua string) error {
	if err := s.requireDeployWrite(ctx, orgID); err != nil {
		return err
	}
	payload := cpjobs.Payload{OrgID: orgID, ProjectID: projectID, UserID: userID}
	_, err := s.jobs.EnqueueSleep(ctx, payload)
	if err != nil {
		return err
	}
	s.recordAudit(ctx, userID, orgID, "project.pause", projectID, ip, ua)
	return nil
}

// Resume wakes a sleeping project.
func (s *Service) Resume(ctx context.Context, orgID, projectID, userID, ip, ua string) error {
	return s.Restart(ctx, orgID, projectID, userID, ip, ua)
}

// Get returns deployment details.
func (s *Service) Get(ctx context.Context, orgID, deploymentID string) (Deployment, error) {
	if err := s.requireDeployRead(ctx, orgID); err != nil {
		return Deployment{}, err
	}
	return s.deployments.GetByID(ctx, orgID, deploymentID)
}

// GetLatest returns the latest deployment for a project.
func (s *Service) GetLatest(ctx context.Context, orgID, projectID string) (Deployment, error) {
	if err := s.requireDeployRead(ctx, orgID); err != nil {
		return Deployment{}, err
	}
	return s.deployments.GetLatest(ctx, orgID, projectID)
}

// ListHistory returns deployment history.
func (s *Service) ListHistory(ctx context.Context, orgID, projectID string, limit int) ([]Deployment, error) {
	if err := s.requireDeployRead(ctx, orgID); err != nil {
		return nil, err
	}
	return s.deployments.ListByProject(ctx, orgID, projectID, limit)
}

// Timeline returns deployment events.
func (s *Service) Timeline(ctx context.Context, orgID, deploymentID string) ([]Event, error) {
	if _, err := s.Get(ctx, orgID, deploymentID); err != nil {
		return nil, err
	}
	return s.deployments.ListEvents(ctx, deploymentID)
}

// DeployFromWebhook creates a deployment triggered by a git webhook.
func (s *Service) DeployFromWebhook(ctx context.Context, orgID, projectID string, in DeployInput) (Deployment, error) {
	return s.Deploy(ctx, orgID, projectID, "", in, "", "webhook")
}

func (s *Service) requireDeployRead(ctx context.Context, orgID string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	return rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermDeployRead)
}

func (s *Service) requireDeployWrite(ctx context.Context, orgID string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	return rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermDeployWrite)
}

func (s *Service) recordAudit(ctx context.Context, userID, orgID, action, resourceID, ip, ua string) {
	var uid, oid *string
	if userID != "" {
		uid = &userID
	}
	oid = &orgID
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: uid, OrgID: oid, Action: action,
		ResourceType: "deployment", ResourceID: resourceID, IPAddress: ip, UserAgent: ua,
	})
}

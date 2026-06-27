package project

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agnivo/agnivo/packages/application/controlplane/cpevents"
	"github.com/agnivo/agnivo/packages/application/identity/audit"
	"github.com/agnivo/agnivo/packages/application/identity/rbac"
	"github.com/agnivo/agnivo/packages/application/identity/tenant"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/agnivo/agnivo/packages/platform/logger"
)

// Service handles project business logic.
type Service struct {
	repo   *Repository
	audit  *audit.Logger
	events *cpevents.Publisher
	region string
}

// NewService constructs a project service.
func NewService(repo *Repository, auditLog *audit.Logger, events *cpevents.Publisher, defaultRegion string) *Service {
	if defaultRegion == "" {
		defaultRegion = "us-east-1"
	}
	return &Service{repo: repo, audit: auditLog, events: events, region: defaultRegion}
}

// CreateInput is the create project request.
type CreateInput struct {
	Name           string   `json:"name" validate:"required,min=1,max=100"`
	Slug           string   `json:"slug" validate:"omitempty,slug"`
	Description    string   `json:"description" validate:"omitempty,max=2000"`
	RepoURL        string   `json:"repo_url" validate:"omitempty,git_repo"`
	RepoProvider   string   `json:"repo_provider" validate:"omitempty,oneof=github gitlab bitbucket generic ''"`
	Branch         string   `json:"branch" validate:"omitempty,max=255"`
	DefaultRuntime string   `json:"default_runtime" validate:"omitempty,max=64"`
	Framework      string   `json:"framework" validate:"omitempty,max=64"`
	BuildMethod    string   `json:"build_method" validate:"omitempty,oneof=dockerfile buildpack nixpacks"`
	Region         string   `json:"region" validate:"omitempty,max=64"`
	Visibility     Visibility `json:"visibility" validate:"omitempty,oneof=private public"`
	Tags           []string `json:"tags" validate:"omitempty,dive,max=64"`
}

// Create creates a project in the current organization.
func (s *Service) Create(ctx context.Context, orgID, userID string, in CreateInput, ip, ua string) (Project, error) {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return Project{}, err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermProjectWrite); err != nil {
		return Project{}, err
	}
	slug := in.Slug
	if slug == "" {
		slug = NormalizeSlug(in.Name)
	} else {
		slug = NormalizeSlug(slug)
	}
	exists, err := s.repo.SlugExists(ctx, orgID, slug, "")
	if err != nil {
		return Project{}, err
	}
	if exists {
		return Project{}, errors.AlreadyExists("project slug already taken")
	}
	region := in.Region
	if region == "" {
		region = s.region
	}
	buildMethod := in.BuildMethod
	if buildMethod == "" {
		buildMethod = "dockerfile"
	}
	p, err := s.repo.Create(ctx, Project{
		OrgID: orgID, Name: in.Name, Slug: slug, Description: in.Description,
		RepoURL: in.RepoURL, RepoProvider: in.RepoProvider, Branch: in.Branch,
		DefaultRuntime: in.DefaultRuntime, Framework: in.Framework, BuildMethod: buildMethod,
		Region: region, Visibility: in.Visibility, Tags: in.Tags, CreatedBy: userID,
	})
	if err != nil {
		return Project{}, err
	}
	s.recordAudit(ctx, userID, orgID, "project.create", p.ID, ip, ua)
	_ = s.events.PublishAsync(ctx, cpevents.ProjectCreated, cpevents.Meta{
		OrgID: orgID, ProjectID: p.ID, AggregateID: p.ID, ActorID: userID,
		CorrelationID: logger.CorrelationID(ctx),
	}, p)
	return p, nil
}

// UpdateInput updates mutable project fields.
type UpdateInput struct {
	Name           *string          `json:"name" validate:"omitempty,min=1,max=100"`
	Description    *string          `json:"description" validate:"omitempty,max=2000"`
	Branch         *string          `json:"branch" validate:"omitempty,max=255"`
	DefaultRuntime *string          `json:"default_runtime" validate:"omitempty,max=64"`
	Framework      *string          `json:"framework" validate:"omitempty,max=64"`
	BuildMethod    *string          `json:"build_method" validate:"omitempty,oneof=dockerfile buildpack nixpacks"`
	Region         *string          `json:"region" validate:"omitempty,max=64"`
	Visibility     *Visibility      `json:"visibility" validate:"omitempty,oneof=private public"`
	Tags           *[]string        `json:"tags"`
	Labels         *json.RawMessage `json:"labels"`
	Metadata       *json.RawMessage `json:"metadata"`
	SleepConfig    *json.RawMessage `json:"sleep_config"`
}

// Update updates a project.
func (s *Service) Update(ctx context.Context, orgID, projectID, userID string, in UpdateInput, ip, ua string) (Project, error) {
	if err := s.requireProjectWrite(ctx, orgID); err != nil {
		return Project{}, err
	}
	vals := map[string]any{}
	if in.Name != nil {
		vals["name"] = *in.Name
	}
	if in.Description != nil {
		vals["description"] = *in.Description
	}
	if in.Branch != nil {
		vals["branch"] = *in.Branch
	}
	if in.DefaultRuntime != nil {
		vals["default_runtime"] = *in.DefaultRuntime
	}
	if in.Framework != nil {
		vals["framework"] = *in.Framework
	}
	if in.BuildMethod != nil {
		vals["build_method"] = *in.BuildMethod
	}
	if in.Region != nil {
		vals["region"] = *in.Region
	}
	if in.Visibility != nil {
		vals["visibility"] = *in.Visibility
	}
	if in.Tags != nil {
		vals["tags"] = *in.Tags
	}
	if in.Labels != nil {
		vals["labels"] = *in.Labels
	}
	if in.Metadata != nil {
		vals["metadata"] = *in.Metadata
	}
	if in.SleepConfig != nil {
		vals["sleep_config"] = *in.SleepConfig
	}
	if len(vals) == 0 {
		return s.repo.GetByID(ctx, orgID, projectID)
	}
	p, err := s.repo.Update(ctx, orgID, projectID, vals)
	if err != nil {
		return Project{}, err
	}
	s.recordAudit(ctx, userID, orgID, "project.update", projectID, ip, ua)
	_ = s.events.PublishAsync(ctx, cpevents.ProjectUpdated, cpevents.Meta{
		OrgID: orgID, ProjectID: projectID, AggregateID: projectID, ActorID: userID,
	}, p)
	return p, nil
}

// Rename renames a project and optionally its slug.
func (s *Service) Rename(ctx context.Context, orgID, projectID, userID, name, slug, ip, ua string) (Project, error) {
	if err := s.requireProjectWrite(ctx, orgID); err != nil {
		return Project{}, err
	}
	vals := map[string]any{"name": name}
	if slug != "" {
		slug = NormalizeSlug(slug)
		exists, err := s.repo.SlugExists(ctx, orgID, slug, projectID)
		if err != nil {
			return Project{}, err
		}
		if exists {
			return Project{}, errors.AlreadyExists("project slug already taken")
		}
		vals["slug"] = slug
	}
	p, err := s.repo.Update(ctx, orgID, projectID, vals)
	if err != nil {
		return Project{}, err
	}
	s.recordAudit(ctx, userID, orgID, "project.rename", projectID, ip, ua)
	return p, nil
}

// Get returns a project.
func (s *Service) Get(ctx context.Context, orgID, projectID string) (Project, error) {
	if err := s.requireProjectRead(ctx, orgID); err != nil {
		return Project{}, err
	}
	return s.repo.GetByID(ctx, orgID, projectID)
}

// List returns projects for the organization.
func (s *Service) List(ctx context.Context, orgID string, includeArchived bool) ([]Project, error) {
	if err := s.requireProjectRead(ctx, orgID); err != nil {
		return nil, err
	}
	return s.repo.ListByOrg(ctx, orgID, includeArchived)
}

// Archive archives a project.
func (s *Service) Archive(ctx context.Context, orgID, projectID, userID, ip, ua string) (Project, error) {
	if err := s.requireProjectWrite(ctx, orgID); err != nil {
		return Project{}, err
	}
	p, err := s.repo.Archive(ctx, orgID, projectID)
	if err != nil {
		return Project{}, err
	}
	s.recordAudit(ctx, userID, orgID, "project.archive", projectID, ip, ua)
	_ = s.events.PublishAsync(ctx, cpevents.ProjectArchived, cpevents.Meta{
		OrgID: orgID, ProjectID: projectID, AggregateID: projectID, ActorID: userID,
	}, p)
	return p, nil
}

// Restore restores an archived project.
func (s *Service) Restore(ctx context.Context, orgID, projectID, userID, ip, ua string) (Project, error) {
	if err := s.requireProjectWrite(ctx, orgID); err != nil {
		return Project{}, err
	}
	p, err := s.repo.Restore(ctx, orgID, projectID)
	if err != nil {
		return Project{}, err
	}
	s.recordAudit(ctx, userID, orgID, "project.restore", projectID, ip, ua)
	return p, nil
}

// Delete soft-deletes a project.
func (s *Service) Delete(ctx context.Context, orgID, projectID, userID, ip, ua string) error {
	if err := s.requireProjectWrite(ctx, orgID); err != nil {
		return err
	}
	if err := s.repo.SoftDelete(ctx, orgID, projectID); err != nil {
		return err
	}
	s.recordAudit(ctx, userID, orgID, "project.delete", projectID, ip, ua)
	_ = s.events.PublishAsync(ctx, cpevents.ProjectDeleted, cpevents.Meta{
		OrgID: orgID, ProjectID: projectID, AggregateID: projectID, ActorID: userID,
	}, nil)
	return nil
}

// Transfer moves a project to another organization.
func (s *Service) Transfer(ctx context.Context, fromOrg, toOrg, projectID, userID, ip, ua string) (Project, error) {
	if err := tenant.AssertOrgMatch(ctx, fromOrg); err != nil {
		return Project{}, err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermProjectWrite); err != nil {
		return Project{}, err
	}
	p, err := s.repo.Transfer(ctx, projectID, fromOrg, toOrg)
	if err != nil {
		return Project{}, err
	}
	s.recordAudit(ctx, userID, fromOrg, "project.transfer", projectID, ip, ua)
	return p, nil
}

// Duplicate creates a copy of a project with a new slug.
func (s *Service) Duplicate(ctx context.Context, orgID, projectID, userID, newName, ip, ua string) (Project, error) {
	if err := s.requireProjectWrite(ctx, orgID); err != nil {
		return Project{}, err
	}
	src, err := s.repo.GetByID(ctx, orgID, projectID)
	if err != nil {
		return Project{}, err
	}
	if newName == "" {
		newName = src.Name + " Copy"
	}
	slug := NormalizeSlug(newName)
	for i := 0; ; i++ {
		candidate := slug
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", slug, i)
		}
		exists, err := s.repo.SlugExists(ctx, orgID, candidate, "")
		if err != nil {
			return Project{}, err
		}
		if !exists {
			slug = candidate
			break
		}
	}
	dup := src
	dup.ID = idx.NewUUID()
	dup.Name = newName
	dup.Slug = slug
	dup.CreatedBy = userID
	dup.Status = StatusActive
	dup.ArchivedAt = nil
	dup.DeletedAt = nil
	p, err := s.repo.Create(ctx, dup)
	if err != nil {
		return Project{}, err
	}
	s.recordAudit(ctx, userID, orgID, "project.duplicate", p.ID, ip, ua)
	return p, nil
}

func (s *Service) requireProjectRead(ctx context.Context, orgID string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	return rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermProjectRead)
}

func (s *Service) requireProjectWrite(ctx context.Context, orgID string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	return rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermProjectWrite)
}

func (s *Service) recordAudit(ctx context.Context, userID, orgID, action, resourceID, ip, ua string) {
	uid, oid := userID, orgID
	_ = s.audit.Record(ctx, audit.Entry{
		UserID: &uid, OrgID: &oid, Action: action,
		ResourceType: "project", ResourceID: resourceID, IPAddress: ip, UserAgent: ua,
	})
}

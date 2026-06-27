package organization

import (
	"context"
	"encoding/json"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/audit"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/member"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/rbac"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/tenant"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// Service handles organization business logic.
type Service struct {
	repo    *Repository
	members *member.Repository
	audit   *audit.Logger
}

// NewService constructs an organization service.
func NewService(repo *Repository, members *member.Repository, auditLog *audit.Logger) *Service {
	return &Service{repo: repo, members: members, audit: auditLog}
}

// CreateInput is the create organization request.
type CreateInput struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
	Slug string `json:"slug" validate:"omitempty,slug"`
}

// Create creates an organization and adds the caller as owner.
func (s *Service) Create(ctx context.Context, userID string, in CreateInput, ip, ua string) (Organization, error) {
	slug := in.Slug
	if slug == "" {
		slug = NormalizeSlug(in.Name)
	} else {
		slug = NormalizeSlug(slug)
	}
	o, err := s.repo.Create(ctx, in.Name, slug, userID)
	if err != nil {
		return Organization{}, err
	}
	if _, err := s.members.AddOwner(ctx, o.ID, userID); err != nil {
		return Organization{}, err
	}
	uid, oid := userID, o.ID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: audit.ActionOrgCreate, IPAddress: ip, UserAgent: ua})
	return o, nil
}

// UpdateInput updates mutable org fields.
type UpdateInput struct {
	Name      *string          `json:"name" validate:"omitempty,min=1,max=100"`
	AvatarURL *string          `json:"avatar_url" validate:"omitempty,url"`
	Settings  *json.RawMessage `json:"settings"`
	Metadata  *json.RawMessage `json:"metadata"`
	Limits    *json.RawMessage `json:"limits"`
}

// Update updates an organization after permission check.
func (s *Service) Update(ctx context.Context, orgID string, in UpdateInput, ip, ua string) (Organization, error) {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return Organization{}, err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermOrgWrite); err != nil {
		return Organization{}, err
	}
	vals := map[string]any{}
	if in.Name != nil {
		vals["name"] = *in.Name
	}
	if in.AvatarURL != nil {
		vals["avatar_url"] = *in.AvatarURL
	}
	if in.Settings != nil {
		vals["settings"] = *in.Settings
	}
	if in.Metadata != nil {
		vals["metadata"] = *in.Metadata
	}
	if in.Limits != nil {
		vals["limits"] = *in.Limits
	}
	if len(vals) == 0 {
		return s.repo.GetByID(ctx, orgID)
	}
	o, err := s.repo.Update(ctx, orgID, vals)
	if err != nil {
		return Organization{}, err
	}
	uid := tenant.UserID(ctx)
	oid := orgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: audit.ActionOrgUpdate, IPAddress: ip, UserAgent: ua})
	return o, nil
}

// Get returns an organization after permission check.
func (s *Service) Get(ctx context.Context, orgID string) (Organization, error) {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return Organization{}, err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermOrgRead); err != nil {
		return Organization{}, err
	}
	return s.repo.GetByID(ctx, orgID)
}

// List returns organizations for the authenticated user.
func (s *Service) List(ctx context.Context, userID string) ([]Organization, error) {
	return s.repo.ListForUser(ctx, userID)
}

// Delete soft-deletes an organization (owner only).
func (s *Service) Delete(ctx context.Context, orgID, ip, ua string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	role := rbac.Role(tenant.MemberRole(ctx))
	if role != rbac.RoleOwner {
		return errors.PermissionDenied("only owners can delete organizations")
	}
	if err := s.repo.SoftDelete(ctx, orgID); err != nil {
		return err
	}
	uid := tenant.UserID(ctx)
	oid := orgID
	return s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: audit.ActionOrgDelete, IPAddress: ip, UserAgent: ua})
}

package apikey

import (
	"context"
	"strings"
	"time"

	"github.com/agnivo/agnivo/packages/application/identity/audit"
	"github.com/agnivo/agnivo/packages/application/identity/rbac"
	"github.com/agnivo/agnivo/packages/application/identity/tenant"
	"github.com/agnivo/agnivo/packages/platform/errors"
)

// Service handles API key business logic.
type Service struct {
	repo  *Repository
	audit *audit.Logger
}

// NewService constructs an API key service.
func NewService(repo *Repository, auditLog *audit.Logger) *Service {
	return &Service{repo: repo, audit: auditLog}
}

// CreateInput creates an API key.
type CreateInput struct {
	Name      string     `json:"name" validate:"required,min=1,max=100"`
	Scopes    []string   `json:"scopes" validate:"required,min=1,dive,min=1"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Create creates an API key and returns the one-time secret.
func (s *Service) Create(ctx context.Context, orgID, userID string, in CreateInput, ip, ua string) (CreateResult, error) {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return CreateResult{}, err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermAPIKeyManage); err != nil {
		return CreateResult{}, err
	}
	res, err := s.repo.Create(ctx, orgID, in.Name, in.Scopes, in.ExpiresAt, userID)
	if err != nil {
		return CreateResult{}, err
	}
	uid, oid := userID, orgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: audit.ActionAPIKeyCreate, ResourceType: "api_key", ResourceID: res.Key.ID, IPAddress: ip, UserAgent: ua})
	return res, nil
}

// List returns API keys for an organization.
func (s *Service) List(ctx context.Context, orgID string) ([]APIKey, error) {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return nil, err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermAPIKeyRead); err != nil {
		return nil, err
	}
	return s.repo.ListByOrg(ctx, orgID)
}

// Rotate rotates an API key.
func (s *Service) Rotate(ctx context.Context, orgID, keyID, userID, ip, ua string) (CreateResult, error) {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return CreateResult{}, err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermAPIKeyManage); err != nil {
		return CreateResult{}, err
	}
	res, err := s.repo.Rotate(ctx, orgID, keyID)
	if err != nil {
		return CreateResult{}, err
	}
	uid, oid := userID, orgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: audit.ActionAPIKeyRotate, ResourceType: "api_key", ResourceID: keyID, IPAddress: ip, UserAgent: ua})
	return res, nil
}

// Disable disables an API key.
func (s *Service) Disable(ctx context.Context, orgID, keyID, userID, ip, ua string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermAPIKeyManage); err != nil {
		return err
	}
	if err := s.repo.Disable(ctx, orgID, keyID); err != nil {
		return err
	}
	uid, oid := userID, orgID
	return s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: "apikey.disable", ResourceType: "api_key", ResourceID: keyID, IPAddress: ip, UserAgent: ua})
}

// Delete deletes an API key.
func (s *Service) Delete(ctx context.Context, orgID, keyID, userID, ip, ua string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermAPIKeyManage); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, orgID, keyID); err != nil {
		return err
	}
	uid, oid := userID, orgID
	return s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: audit.ActionAPIKeyDelete, ResourceType: "api_key", ResourceID: keyID, IPAddress: ip, UserAgent: ua})
}

// Authenticate validates an API key string and returns the key record.
func (s *Service) Authenticate(ctx context.Context, rawKey, ip string) (APIKey, error) {
	prefix, secret, err := SplitKey(rawKey)
	if err != nil {
		return APIKey{}, errors.Unauthenticated("invalid api key")
	}
	k, err := s.repo.GetByPrefix(ctx, prefix)
	if err != nil {
		return APIKey{}, errors.Unauthenticated("invalid api key")
	}
	if !VerifySecret(k, secret) {
		return APIKey{}, errors.Unauthenticated("invalid api key")
	}
	_ = s.repo.RecordUsage(ctx, k.ID, ip)
	return k, nil
}

// SplitKey parses "prefix_secret" format.
func SplitKey(raw string) (prefix, secret string, err error) {
	raw = strings.TrimSpace(raw)
	i := strings.LastIndex(raw, "_")
	if i <= 0 || i >= len(raw)-1 {
		return "", "", errors.InvalidArgument("malformed api key")
	}
	return raw[:i], raw[i+1:], nil
}

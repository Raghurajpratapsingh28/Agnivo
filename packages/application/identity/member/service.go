package member

import (
	"context"
	"strings"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/audit"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/rbac"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/tenant"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/tokencrypto"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/user"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// Service handles membership business logic.
type Service struct {
	db    *postgres.DB
	repo  *Repository
	users *user.Repository
	audit *audit.Logger
}

// NewService constructs a member service.
func NewService(db *postgres.DB, repo *Repository, users *user.Repository, auditLog *audit.Logger) *Service {
	return &Service{db: db, repo: repo, users: users, audit: auditLog}
}

// InviteInput invites a user by email.
type InviteInput struct {
	Email string    `json:"email" validate:"required,email"`
	Role  rbac.Role `json:"role" validate:"required"`
}

// Invite creates a pending invitation and returns the raw token for email delivery.
func (s *Service) Invite(ctx context.Context, orgID, invitedBy string, in InviteInput, ip, ua string) (Invitation, string, error) {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return Invitation{}, "", err
	}
	actorRole := rbac.Role(tenant.MemberRole(ctx))
	if err := rbac.RequirePermission(actorRole, rbac.PermMemberInvite); err != nil {
		return Invitation{}, "", err
	}
	if !rbac.CanManageRole(actorRole, in.Role) {
		return Invitation{}, "", errors.PermissionDenied("cannot assign this role")
	}
	raw, hash := tokencrypto.Generate(32)
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	inv, err := s.repo.CreateInvitation(ctx, orgID, strings.ToLower(in.Email), in.Role, hash, invitedBy, expires)
	if err != nil {
		return Invitation{}, "", err
	}
	uid, oid := invitedBy, orgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: audit.ActionMemberInvite, IPAddress: ip, UserAgent: ua})
	return inv, raw, nil
}

// AcceptInput accepts an invitation.
type AcceptInput struct {
	Token string `json:"token" validate:"required"`
}

// Accept accepts an invitation for the authenticated user.
func (s *Service) Accept(ctx context.Context, userID string, in AcceptInput, ip, ua string) (Member, error) {
	hash := tokencrypto.Hash(in.Token)
	inv, err := s.repo.GetInvitationByTokenHash(ctx, hash)
	if err != nil {
		return Member{}, errors.Unauthenticated("invalid or expired invitation")
	}
	m, err := s.repo.AcceptInvitation(ctx, inv, userID)
	if err != nil {
		return Member{}, err
	}
	uid, oid := userID, inv.OrgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: "member.accept", IPAddress: ip, UserAgent: ua})
	return m, nil
}

// Reject rejects an invitation.
func (s *Service) Reject(ctx context.Context, in AcceptInput, ip, ua string) error {
	hash := tokencrypto.Hash(in.Token)
	inv, err := s.repo.GetInvitationByTokenHash(ctx, hash)
	if err != nil {
		return errors.Unauthenticated("invalid or expired invitation")
	}
	if err := s.repo.RejectInvitation(ctx, inv.ID); err != nil {
		return err
	}
	oid := inv.OrgID
	return s.audit.Record(ctx, audit.Entry{OrgID: &oid, Action: "member.reject", IPAddress: ip, UserAgent: ua})
}

// Remove removes a member from the organization.
func (s *Service) Remove(ctx context.Context, orgID, targetUserID, actorID string, ip, ua string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	actorRole := rbac.Role(tenant.MemberRole(ctx))
	if err := rbac.RequirePermission(actorRole, rbac.PermMemberRemove); err != nil {
		return err
	}
	target, err := s.repo.GetActive(ctx, orgID, targetUserID)
	if err != nil {
		return err
	}
	if target.Role == rbac.RoleOwner {
		return errors.PermissionDenied("cannot remove organization owner")
	}
	if !rbac.CanManageRole(actorRole, target.Role) {
		return errors.PermissionDenied("insufficient permissions")
	}
	if err := s.repo.Remove(ctx, orgID, targetUserID); err != nil {
		return err
	}
	uid, oid := actorID, orgID
	meta := []byte(`{"target_user_id":"` + targetUserID + `"}`)
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: audit.ActionMemberRemove, IPAddress: ip, UserAgent: ua, Metadata: meta})
	return nil
}

// UpdateRoleInput changes a member's role.
type UpdateRoleInput struct {
	Role rbac.Role `json:"role" validate:"required"`
}

// UpdateRole changes a member's role.
func (s *Service) UpdateRole(ctx context.Context, orgID, targetUserID, actorID string, in UpdateRoleInput, ip, ua string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	actorRole := rbac.Role(tenant.MemberRole(ctx))
	if err := rbac.RequirePermission(actorRole, rbac.PermMemberManage); err != nil {
		return err
	}
	if !rbac.CanManageRole(actorRole, in.Role) {
		return errors.PermissionDenied("cannot assign this role")
	}
	target, err := s.repo.GetActive(ctx, orgID, targetUserID)
	if err != nil {
		return err
	}
	if target.Role == rbac.RoleOwner || in.Role == rbac.RoleOwner {
		return errors.PermissionDenied("use transfer ownership to change owner")
	}
	if err := s.repo.UpdateRole(ctx, orgID, targetUserID, in.Role); err != nil {
		return err
	}
	uid, oid := actorID, orgID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: audit.ActionMemberRoleChange, IPAddress: ip, UserAgent: ua})
	return nil
}

// TransferOwnership transfers org ownership to another member.
func (s *Service) TransferOwnership(ctx context.Context, orgID, newOwnerID, currentOwnerID string, ip, ua string) error {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return err
	}
	if rbac.Role(tenant.MemberRole(ctx)) != rbac.RoleOwner {
		return errors.PermissionDenied("only owners can transfer ownership")
	}
	if _, err := s.repo.GetActive(ctx, orgID, newOwnerID); err != nil {
		return err
	}
	err := s.db.Transact(ctx, func(ctx context.Context) error {
		if err := s.repo.UpdateRole(ctx, orgID, currentOwnerID, rbac.RoleAdmin); err != nil {
			return err
		}
		return s.repo.UpdateRole(ctx, orgID, newOwnerID, rbac.RoleOwner)
	})
	if err != nil {
		return err
	}
	uid, oid := currentOwnerID, orgID
	return s.audit.Record(ctx, audit.Entry{UserID: &uid, OrgID: &oid, Action: "member.transfer_ownership", IPAddress: ip, UserAgent: ua})
}

// List returns active members.
func (s *Service) List(ctx context.Context, orgID string) ([]Member, error) {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return nil, err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermMemberRead); err != nil {
		return nil, err
	}
	return s.repo.ListByOrg(ctx, orgID)
}

// ListInvitations returns pending invitations.
func (s *Service) ListInvitations(ctx context.Context, orgID string) ([]Invitation, error) {
	if err := tenant.AssertOrgMatch(ctx, orgID); err != nil {
		return nil, err
	}
	if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(ctx)), rbac.PermMemberRead); err != nil {
		return nil, err
	}
	return s.repo.ListPendingInvitations(ctx, orgID)
}

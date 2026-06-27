package user

import (
	"context"
	"encoding/json"

	"github.com/agnivo/agnivo/packages/application/identity/audit"
	"github.com/agnivo/agnivo/packages/application/identity/tenant"
)

// Service handles user profile business logic.
type Service struct {
	repo  *Repository
	audit *audit.Logger
}

// NewService constructs a user service.
func NewService(repo *Repository, auditLog *audit.Logger) *Service {
	return &Service{repo: repo, audit: auditLog}
}

// UpdateProfileInput updates user profile fields.
type UpdateProfileInput struct {
	DisplayName *string          `json:"display_name" validate:"omitempty,min=1,max=100"`
	AvatarURL   *string          `json:"avatar_url" validate:"omitempty,url"`
	Timezone    *string          `json:"timezone" validate:"omitempty,max=64"`
	Locale      *string          `json:"locale" validate:"omitempty,max=16"`
	Preferences *json.RawMessage `json:"preferences"`
}

// UpdateProfile updates the authenticated user's profile.
func (s *Service) UpdateProfile(ctx context.Context, userID string, in UpdateProfileInput, ip, ua string) (User, error) {
	displayName, avatarURL, timezone, locale := "", "", "", ""
	if in.DisplayName != nil {
		displayName = *in.DisplayName
	}
	if in.AvatarURL != nil {
		avatarURL = *in.AvatarURL
	}
	if in.Timezone != nil {
		timezone = *in.Timezone
	}
	if in.Locale != nil {
		locale = *in.Locale
	}
	var prefs json.RawMessage
	if in.Preferences != nil {
		prefs = *in.Preferences
	}
	u, err := s.repo.UpdateProfile(ctx, userID, displayName, avatarURL, timezone, locale, prefs)
	if err != nil {
		return User{}, err
	}
	uid := userID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: "user.profile_update", IPAddress: ip, UserAgent: ua})
	return u, nil
}

// Get returns the authenticated user.
func (s *Service) Get(ctx context.Context, userID string) (User, error) {
	return s.repo.GetByID(ctx, userID)
}

// Suspend suspends a user account (admin operation).
func (s *Service) Suspend(ctx context.Context, targetID, ip, ua string) error {
	if err := s.repo.Suspend(ctx, targetID); err != nil {
		return err
	}
	uid := tenant.UserID(ctx)
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: "user.suspend", ResourceType: "user", ResourceID: targetID, IPAddress: ip, UserAgent: ua})
	return nil
}

// Delete soft-deletes the authenticated user's account.
func (s *Service) Delete(ctx context.Context, userID, ip, ua string) error {
	if err := s.repo.SoftDelete(ctx, userID); err != nil {
		return err
	}
	uid := userID
	return s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: "user.delete", IPAddress: ip, UserAgent: ua})
}

// PublicUser strips sensitive fields for API responses.
func PublicUser(u User) map[string]any {
	return map[string]any{
		"id": u.ID, "email": u.Email, "display_name": u.DisplayName,
		"avatar_url": u.AvatarURL, "timezone": u.Timezone, "locale": u.Locale,
		"status": u.Status, "email_verified": u.EmailVerifiedAt != nil,
		"last_login_at": u.LastLoginAt, "created_at": u.CreatedAt,
	}
}

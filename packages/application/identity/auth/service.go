package auth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agnivo/agnivo/packages/application/identity/audit"
	"github.com/agnivo/agnivo/packages/application/identity/jwt"
	"github.com/agnivo/agnivo/packages/application/identity/member"
	"github.com/agnivo/agnivo/packages/application/identity/organization"
	"github.com/agnivo/agnivo/packages/application/identity/password"
	"github.com/agnivo/agnivo/packages/application/identity/session"
	"github.com/agnivo/agnivo/packages/application/identity/tokencrypto"
	"github.com/agnivo/agnivo/packages/application/identity/user"
	"github.com/agnivo/agnivo/packages/platform/cache/redis"
	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/errors"
)

// TokenPair is returned on successful authentication.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// Service handles authentication flows.
type Service struct {
	db         *postgres.DB
	users      *user.Repository
	orgs       *organization.Repository
	members    *member.Repository
	sessions   *session.Repository
	revocation *session.RevocationStore
	jwt        *jwt.Manager
	hasher     *password.Hasher
	audit      *audit.Logger
	refreshTTL time.Duration
	loginLimit *redis.TokenBucket
}

// Deps are the auth service dependencies.
type Deps struct {
	DB         *postgres.DB
	Users      *user.Repository
	Orgs       *organization.Repository
	Members    *member.Repository
	Sessions   *session.Repository
	Revocation *session.RevocationStore
	JWT        *jwt.Manager
	Hasher     *password.Hasher
	Audit      *audit.Logger
	Config     *config.Config
	Redis      *redis.Client
}

// NewService constructs the auth service.
func NewService(d Deps) *Service {
	refreshTTL := d.Config.Identity.JWT.RefreshTTL
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	var bucket *redis.TokenBucket
	if d.Redis != nil {
		bucket = d.Redis.NewTokenBucket(10, 0.2)
	}
	return &Service{
		db: d.DB, users: d.Users, orgs: d.Orgs, members: d.Members,
		sessions: d.Sessions, revocation: d.Revocation,
		jwt: d.JWT, hasher: d.Hasher, audit: d.Audit,
		refreshTTL: refreshTTL, loginLimit: bucket,
	}
}

// RegisterInput is the registration request.
type RegisterInput struct {
	Email       string `json:"email" validate:"required,email"`
	Password    string `json:"password" validate:"required,min=12"`
	DisplayName string `json:"display_name" validate:"required,min=1,max=100"`
	OrgName     string `json:"org_name" validate:"omitempty,min=1,max=100"`
}

// Register creates a user and optionally their first organization.
func (s *Service) Register(ctx context.Context, in RegisterInput, ip, ua string) (user.User, error) {
	if err := s.rateLimitLogin(ctx, ip); err != nil {
		return user.User{}, err
	}
	exists, err := s.users.ExistsEmail(ctx, in.Email)
	if err != nil {
		return user.User{}, err
	}
	if exists {
		return user.User{}, errors.InvalidArgument("registration failed")
	}
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return user.User{}, err
	}
	var u user.User
	err = s.db.Transact(ctx, func(ctx context.Context) error {
		var terr error
		u, terr = s.users.Create(ctx, in.Email, hash, in.DisplayName)
		if terr != nil {
			return terr
		}
		if err := s.users.Activate(ctx, u.ID); err != nil {
			return err
		}
		if in.OrgName != "" {
			slug := organization.NormalizeSlug(in.OrgName)
			o, err := s.orgs.Create(ctx, in.OrgName, slug, u.ID)
			if err != nil {
				return err
			}
			if _, err := s.members.AddOwner(ctx, o.ID, u.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return user.User{}, err
	}
	uid := u.ID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: audit.ActionRegister, IPAddress: ip, UserAgent: ua})
	u, _ = s.users.GetByID(ctx, u.ID)
	return u, nil
}

// LoginInput is the login request.
type LoginInput struct {
	Email      string `json:"email" validate:"required,email"`
	Password   string `json:"password" validate:"required"`
	RememberMe bool   `json:"remember_me"`
	DeviceName string `json:"device_name" validate:"omitempty,max=100"`
	OrgID      string `json:"org_id" validate:"omitempty,uuid"`
}

// Login authenticates and returns tokens.
func (s *Service) Login(ctx context.Context, in LoginInput, ip, ua string) (TokenPair, error) {
	if err := s.rateLimitLogin(ctx, ip); err != nil {
		return TokenPair{}, err
	}
	u, err := s.users.GetByEmail(ctx, in.Email)
	if err != nil {
		_, _ = s.hasher.Verify(in.Password, "")
		return TokenPair{}, errors.Unauthenticated("invalid credentials")
	}
	ok, err := s.hasher.Verify(in.Password, u.PasswordHash)
	if err != nil || !ok {
		return TokenPair{}, errors.Unauthenticated("invalid credentials")
	}
	if u.Status == user.StatusSuspended {
		return TokenPair{}, errors.PermissionDenied("account suspended")
	}
	if !u.CanLogin() {
		return TokenPair{}, errors.Unauthenticated("invalid credentials")
	}
	var orgID *string
	if in.OrgID != "" {
		if _, err := s.members.GetActive(ctx, in.OrgID, u.ID); err != nil {
			return TokenPair{}, errors.PermissionDenied("not a member of organization")
		}
		orgID = &in.OrgID
	}
	return s.issueTokens(ctx, u.ID, orgID, session.DeviceBrowser, in.DeviceName, ip, ua, in.RememberMe)
}

// Refresh rotates refresh token and issues new access token.
func (s *Service) Refresh(ctx context.Context, refreshToken, ip, ua string) (TokenPair, error) {
	hash := tokencrypto.Hash(refreshToken)
	sess, err := s.sessions.GetByRefreshHash(ctx, hash)
	if err != nil {
		return TokenPair{}, errors.Unauthenticated("invalid refresh token")
	}
	revoked, err := s.revocation.IsSessionRevoked(ctx, sess.ID)
	if err != nil || revoked {
		return TokenPair{}, errors.Unauthenticated("session revoked")
	}
	newRaw, newHash := tokencrypto.Generate(32)
	if err := s.sessions.RotateRefresh(ctx, sess.ID, newHash); err != nil {
		return TokenPair{}, err
	}
	orgID := ""
	if sess.OrgID != nil {
		orgID = *sess.OrgID
	}
	access, exp, err := s.jwt.IssueAccessToken(sess.UserID, sess.ID, orgID)
	if err != nil {
		return TokenPair{}, err
	}
	uid := sess.UserID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: "auth.refresh", IPAddress: ip, UserAgent: ua})
	return TokenPair{
		AccessToken: access, RefreshToken: newRaw, ExpiresAt: exp, TokenType: "Bearer",
	}, nil
}

// Logout revokes the session identified by refresh token.
func (s *Service) Logout(ctx context.Context, refreshToken, ip, ua string) error {
	hash := tokencrypto.Hash(refreshToken)
	sess, err := s.sessions.GetByRefreshHash(ctx, hash)
	if err != nil {
		return nil
	}
	if err := s.sessions.Revoke(ctx, sess.ID, sess.UserID); err != nil {
		return err
	}
	_ = s.revocation.RevokeSession(ctx, sess.ID, sess.ExpiresAt)
	uid := sess.UserID
	return s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: audit.ActionLogout, IPAddress: ip, UserAgent: ua})
}

// LogoutAll revokes all sessions for the user.
func (s *Service) LogoutAll(ctx context.Context, userID, exceptSessionID, ip, ua string) error {
	_, err := s.sessions.RevokeAll(ctx, userID, exceptSessionID)
	if err != nil {
		return err
	}
	uid := userID
	meta, _ := json.Marshal(map[string]any{"all_devices": true})
	return s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: audit.ActionLogout, IPAddress: ip, UserAgent: ua, Metadata: meta})
}

// RequestPasswordResetInput triggers a password reset email flow.
type RequestPasswordResetInput struct {
	Email string `json:"email" validate:"required,email"`
}

// RequestPasswordReset creates a reset token. Returns raw token for email delivery.
func (s *Service) RequestPasswordReset(ctx context.Context, in RequestPasswordResetInput, ip, ua string) (string, error) {
	u, err := s.users.GetByEmail(ctx, in.Email)
	if err != nil {
		// Always succeed to prevent user enumeration.
		return "", nil
	}
	raw, hash := tokencrypto.Generate(32)
	expires := time.Now().UTC().Add(1 * time.Hour)
	if err := s.users.CreatePasswordResetToken(ctx, u.ID, hash, expires); err != nil {
		return "", err
	}
	uid := u.ID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: audit.ActionPasswordReset, IPAddress: ip, UserAgent: ua})
	return raw, nil
}

// ResetPasswordInput completes a password reset.
type ResetPasswordInput struct {
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=12"`
}

// ResetPassword sets a new password using a reset token.
func (s *Service) ResetPassword(ctx context.Context, in ResetPasswordInput, ip, ua string) error {
	hash := tokencrypto.Hash(in.Token)
	userID, err := s.users.ConsumePasswordResetToken(ctx, hash)
	if err != nil {
		return errors.Unauthenticated("invalid or expired reset token")
	}
	pwHash, err := s.hasher.Hash(in.NewPassword)
	if err != nil {
		return err
	}
	if err := s.users.SetPassword(ctx, userID, pwHash); err != nil {
		return err
	}
	_, _ = s.sessions.RevokeAll(ctx, userID, "")
	uid := userID
	return s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: audit.ActionPasswordReset, IPAddress: ip, UserAgent: ua})
}

// ChangePasswordInput changes password for an authenticated user.
type ChangePasswordInput struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=12"`
}

// ChangePassword updates the password for the authenticated user.
func (s *Service) ChangePassword(ctx context.Context, userID string, in ChangePasswordInput, ip, ua string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	ok, err := s.hasher.Verify(in.CurrentPassword, u.PasswordHash)
	if err != nil || !ok {
		return errors.Unauthenticated("invalid current password")
	}
	pwHash, err := s.hasher.Hash(in.NewPassword)
	if err != nil {
		return err
	}
	if err := s.users.SetPassword(ctx, userID, pwHash); err != nil {
		return err
	}
	_, _ = s.sessions.RevokeAll(ctx, userID, tenantSessionID(ctx))
	uid := userID
	return s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: audit.ActionPasswordChange, IPAddress: ip, UserAgent: ua})
}

// VerifyEmailInput verifies an email address.
type VerifyEmailInput struct {
	Token string `json:"token" validate:"required"`
}

// VerifyEmail marks the user's email as verified.
func (s *Service) VerifyEmail(ctx context.Context, in VerifyEmailInput, ip, ua string) error {
	hash := tokencrypto.Hash(in.Token)
	userID, err := s.users.ConsumeEmailToken(ctx, hash)
	if err != nil {
		return errors.Unauthenticated("invalid or expired verification token")
	}
	if err := s.users.Activate(ctx, userID); err != nil {
		return err
	}
	uid := userID
	return s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: "auth.email_verify", IPAddress: ip, UserAgent: ua})
}

// CreateEmailVerificationToken creates a verification token for a user.
func (s *Service) CreateEmailVerificationToken(ctx context.Context, userID string) (string, error) {
	raw, hash := tokencrypto.Generate(32)
	expires := time.Now().UTC().Add(24 * time.Hour)
	if err := s.users.CreateEmailToken(ctx, userID, hash, expires); err != nil {
		return "", err
	}
	return raw, nil
}

func (s *Service) issueTokens(ctx context.Context, userID string, orgID *string, deviceType session.DeviceType, deviceName, ip, ua string, remember bool) (TokenPair, error) {
	refreshRaw, refreshHash := tokencrypto.Generate(32)
	ttl := s.refreshTTL
	if !remember {
		ttl = 24 * time.Hour
	}
	expiresAt := time.Now().UTC().Add(ttl)
	sess, err := s.sessions.Create(ctx, userID, orgID, refreshHash, deviceName, deviceType, ip, ua, remember, expiresAt)
	if err != nil {
		return TokenPair{}, err
	}
	oid := ""
	if orgID != nil {
		oid = *orgID
	}
	access, exp, err := s.jwt.IssueAccessToken(userID, sess.ID, oid)
	if err != nil {
		return TokenPair{}, err
	}
	_ = s.users.RecordLogin(ctx, userID)
	uid := userID
	_ = s.audit.Record(ctx, audit.Entry{UserID: &uid, Action: audit.ActionLogin, IPAddress: ip, UserAgent: ua})
	return TokenPair{
		AccessToken: access, RefreshToken: refreshRaw, ExpiresAt: exp, TokenType: "Bearer",
	}, nil
}

func (s *Service) rateLimitLogin(ctx context.Context, ip string) error {
	if s.loginLimit == nil || ip == "" {
		return nil
	}
	res, err := s.loginLimit.Allow(ctx, "identity:login:"+ip)
	if err != nil {
		return err
	}
	if !res.Allowed {
		return errors.RateLimited("too many attempts").WithRetryable(true)
	}
	return nil
}

type sessionCtxKey struct{}

func tenantSessionID(ctx context.Context) string {
	if v, ok := ctx.Value(sessionCtxKey{}).(string); ok {
		return v
	}
	return ""
}

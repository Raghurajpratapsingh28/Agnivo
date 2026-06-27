// Package http exposes REST handlers and middleware for the identity platform.
package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/apikey"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/auth"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/jwt"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/member"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/organization"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/pat"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/rbac"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/session"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/tenant"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/user"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/dto"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/httpx"
)

// MiddlewareDeps configures identity HTTP middleware.
type MiddlewareDeps struct {
	JWT        *jwt.Manager
	Revocation *session.RevocationStore
	Members    *member.Repository
	APIKeys    *apikey.Service
	PATs       *pat.Repository
}

// Middleware provides authentication and authorization middleware.
type Middleware struct {
	jwt        *jwt.Manager
	revocation *session.RevocationStore
	members    *member.Repository
	apiKeys    *apikey.Service
	pats       *pat.Repository
}

// NewMiddleware constructs identity middleware.
func NewMiddleware(d MiddlewareDeps) *Middleware {
	return &Middleware{jwt: d.JWT, revocation: d.Revocation, members: d.Members, apiKeys: d.APIKeys, pats: d.PATs}
}

// Authenticate validates Bearer JWT, API key, or PAT and populates tenant context.
func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		authHeader := httpx.Header(r, "Authorization")

		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if strings.HasPrefix(token, "agn_") || strings.HasPrefix(token, "pat_") {
				ctx, err := m.authenticateToken(ctx, r, token)
				if err != nil {
					dto.Error(w, r, err)
					return
				}
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			ctx, err := m.authenticateJWT(ctx, token)
			if err != nil {
				dto.Error(w, r, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuth ensures a user is authenticated.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tenant.UserID(r.Context()) == "" {
			dto.Error(w, r, errors.Unauthenticated("authentication required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// OrgContext validates org membership from URL and sets org + role in context.
func (m *Middleware) OrgContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, err := httpx.RequirePathParam(r, "orgID")
		if err != nil {
			dto.Error(w, r, err)
			return
		}
		userID, err := tenant.RequireUser(r.Context())
		if err != nil {
			dto.Error(w, r, err)
			return
		}
		mem, err := m.members.GetActive(r.Context(), orgID, userID)
		if err != nil {
			dto.Error(w, r, errors.PermissionDenied("not a member of this organization"))
			return
		}
		ctx := tenant.WithOrg(r.Context(), orgID)
		ctx = tenant.WithMemberRole(ctx, string(mem.Role))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequirePermission checks RBAC for the current org role.
func (m *Middleware) RequirePermission(perm rbac.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := rbac.RequirePermission(rbac.Role(tenant.MemberRole(r.Context())), perm); err != nil {
				dto.Error(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (m *Middleware) authenticateJWT(ctx context.Context, tokenStr string) (context.Context, error) {
	claims, err := m.jwt.ValidateAccessToken(tokenStr)
	if err != nil {
		return ctx, err
	}
	revoked, err := m.revocation.IsSessionRevoked(ctx, claims.SessionID)
	if err != nil || revoked {
		return ctx, errors.Unauthenticated("session revoked")
	}
	ctx = tenant.WithUser(ctx, claims.UserID)
	ctx = tenant.WithSession(ctx, claims.SessionID)
	ctx = tenant.WithAuthMethod(ctx, tenant.AuthJWT)
	if claims.OrgID != "" {
		ctx = tenant.WithOrg(ctx, claims.OrgID)
		if m.members != nil {
			if mem, err := m.members.GetActive(ctx, claims.OrgID, claims.UserID); err == nil {
				ctx = tenant.WithMemberRole(ctx, string(mem.Role))
			}
		}
	}
	return ctx, nil
}

func (m *Middleware) authenticateToken(ctx context.Context, r *http.Request, raw string) (context.Context, error) {
	ip := httpx.ClientIP(r)
	if strings.HasPrefix(raw, "agn_") {
		k, err := m.apiKeys.Authenticate(ctx, raw, ip)
		if err != nil {
			return ctx, err
		}
		ctx = tenant.WithOrg(ctx, k.OrgID)
		ctx = tenant.WithUser(ctx, k.CreatedBy)
		ctx = tenant.WithAuthMethod(ctx, tenant.AuthAPIKey)
		return ctx, nil
	}
	if strings.HasPrefix(raw, "pat_") {
		prefix, secret, err := apikey.SplitKey(raw)
		if err != nil {
			return ctx, errors.Unauthenticated("invalid token")
		}
		t, err := m.pats.GetByPrefix(ctx, prefix)
		if err != nil {
			return ctx, errors.Unauthenticated("invalid token")
		}
		if !pat.VerifySecret(t, secret) {
			return ctx, errors.Unauthenticated("invalid token")
		}
		_ = m.pats.RecordUsage(ctx, t.ID)
		ctx = tenant.WithUser(ctx, t.UserID)
		ctx = tenant.WithAuthMethod(ctx, tenant.AuthPAT)
		if t.OrgID != nil {
			ctx = tenant.WithOrg(ctx, *t.OrgID)
		}
		return ctx, nil
	}
	return ctx, errors.Unauthenticated("unsupported token type")
}

// CSRFProtection validates Origin/Referer for state-changing cookie-based requests.
// allowedOrigins should match http.cors.allowed_origins; "*" disables validation.
func CSRFProtection(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := false
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			allowAll = true
			break
		}
		allowed[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			if allowAll {
				next.ServeHTTP(w, r)
				return
			}
			origin := httpx.Header(r, "Origin")
			if origin == "" {
				origin = httpx.Header(r, "Referer")
			}
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := allowed[origin]; !ok {
				dto.Error(w, r, errors.PermissionDenied("CSRF validation failed: origin not allowed"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// HandlersDeps configures HTTP handlers.
type HandlersDeps struct {
	Auth     *auth.Service
	Users    *user.Service
	Orgs     *organization.Service
	Members  *member.Service
	APIKeys  *apikey.Service
	Sessions *session.Service
}

// Handlers exposes identity REST handlers.
type Handlers struct {
	auth     *auth.Service
	users    *user.Service
	orgs     *organization.Service
	members  *member.Service
	apiKeys  *apikey.Service
	sessions *session.Service
}

// NewHandlers constructs identity handlers.
func NewHandlers(d HandlersDeps) *Handlers {
	return &Handlers{
		auth: d.Auth, users: d.Users, orgs: d.Orgs, members: d.Members,
		apiKeys: d.APIKeys, sessions: d.Sessions,
	}
}

func clientMeta(r *http.Request) (ip, ua string) {
	return httpx.ClientIP(r), httpx.Header(r, "User-Agent")
}

// Register handles POST /auth/register.
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	var in auth.RegisterInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	u, err := h.auth.Register(r.Context(), in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, user.PublicUser(u))
}

// Login handles POST /auth/login.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var in auth.LoginInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	tokens, err := h.auth.Login(r.Context(), in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, tokens)
}

// Refresh handles POST /auth/refresh.
func (h *Handlers) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token" validate:"required"`
	}
	if err := dto.DecodeValidate(w, r, &body); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	tokens, err := h.auth.Refresh(r.Context(), body.RefreshToken, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, tokens)
}

// Logout handles POST /auth/logout.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token" validate:"required"`
	}
	if err := dto.DecodeValidate(w, r, &body); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	if err := h.auth.Logout(r.Context(), body.RefreshToken, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// VerifyEmail handles POST /auth/verify-email.
func (h *Handlers) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var in auth.VerifyEmailInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	if err := h.auth.VerifyEmail(r.Context(), in, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// RequestPasswordReset handles POST /auth/password-reset.
func (h *Handlers) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var in auth.RequestPasswordResetInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	_, _ = h.auth.RequestPasswordReset(r.Context(), in, ip, ua)
	dto.NoContent(w)
}

// ResetPassword handles POST /auth/password-reset/confirm.
func (h *Handlers) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var in auth.ResetPasswordInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	if err := h.auth.ResetPassword(r.Context(), in, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// ChangePassword handles POST /auth/change-password.
func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	var in auth.ChangePasswordInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	if err := h.auth.ChangePassword(r.Context(), userID, in, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// Me handles GET /auth/me.
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	u, err := h.users.Get(r.Context(), userID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, user.PublicUser(u))
}

// UpdateProfile handles PATCH /auth/me.
func (h *Handlers) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	var in user.UpdateProfileInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	u, err := h.users.UpdateProfile(r.Context(), userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, user.PublicUser(u))
}

// CreateOrg handles POST /orgs.
func (h *Handlers) CreateOrg(w http.ResponseWriter, r *http.Request) {
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	var in organization.CreateInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	o, err := h.orgs.Create(r.Context(), userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, o)
}

// ListOrgs handles GET /orgs.
func (h *Handlers) ListOrgs(w http.ResponseWriter, r *http.Request) {
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	orgs, err := h.orgs.List(r.Context(), userID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, orgs)
}

// GetOrg handles GET /orgs/{orgID}.
func (h *Handlers) GetOrg(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpx.RequirePathParam(r, "orgID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	o, err := h.orgs.Get(r.Context(), orgID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, o)
}

// UpdateOrg handles PATCH /orgs/{orgID}.
func (h *Handlers) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpx.RequirePathParam(r, "orgID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	var in organization.UpdateInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	o, err := h.orgs.Update(r.Context(), orgID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, o)
}

// DeleteOrg handles DELETE /orgs/{orgID}.
func (h *Handlers) DeleteOrg(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpx.RequirePathParam(r, "orgID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	if err := h.orgs.Delete(r.Context(), orgID, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// ListMembers handles GET /orgs/{orgID}/members.
func (h *Handlers) ListMembers(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpx.RequirePathParam(r, "orgID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	members, err := h.members.List(r.Context(), orgID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, members)
}

// InviteMember handles POST /orgs/{orgID}/members.
func (h *Handlers) InviteMember(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpx.RequirePathParam(r, "orgID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	var in member.InviteInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	if _, roleErr := rbac.ParseRole(string(in.Role)); roleErr != nil {
		dto.Error(w, r, roleErr)
		return
	}
	ip, ua := clientMeta(r)
	inv, token, err := h.members.Invite(r.Context(), orgID, userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, map[string]any{"invitation": inv, "token": token})
}

// AcceptInvitation handles POST /auth/invitations/accept.
func (h *Handlers) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	var in member.AcceptInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	m, err := h.members.Accept(r.Context(), userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, m)
}

// RemoveMember handles DELETE /orgs/{orgID}/members/{userID}.
func (h *Handlers) RemoveMember(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpx.RequirePathParam(r, "orgID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	targetID, err := httpx.RequirePathParam(r, "userID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	actorID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	if err := h.members.Remove(r.Context(), orgID, targetID, actorID, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// UpdateMemberRole handles PATCH /orgs/{orgID}/members/{userID}.
func (h *Handlers) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpx.RequirePathParam(r, "orgID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	targetID, err := httpx.RequirePathParam(r, "userID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	actorID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	var in member.UpdateRoleInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	if err := h.members.UpdateRole(r.Context(), orgID, targetID, actorID, in, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// ListAPIKeys handles GET /orgs/{orgID}/api-keys.
func (h *Handlers) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpx.RequirePathParam(r, "orgID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	keys, err := h.apiKeys.List(r.Context(), orgID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, keys)
}

// CreateAPIKey handles POST /orgs/{orgID}/api-keys.
func (h *Handlers) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpx.RequirePathParam(r, "orgID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	var in apikey.CreateInput
	if err := dto.DecodeValidate(w, r, &in); err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	res, err := h.apiKeys.Create(r.Context(), orgID, userID, in, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, res)
}

// RotateAPIKey handles POST /orgs/{orgID}/api-keys/{keyID}/rotate.
func (h *Handlers) RotateAPIKey(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpx.RequirePathParam(r, "orgID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	keyID, err := httpx.RequirePathParam(r, "keyID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	res, err := h.apiKeys.Rotate(r.Context(), orgID, keyID, userID, ip, ua)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, res)
}

// DeleteAPIKey handles DELETE /orgs/{orgID}/api-keys/{keyID}.
func (h *Handlers) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	orgID, err := httpx.RequirePathParam(r, "orgID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	keyID, err := httpx.RequirePathParam(r, "keyID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	if err := h.apiKeys.Delete(r.Context(), orgID, keyID, userID, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// ListSessions handles GET /auth/sessions.
func (h *Handlers) ListSessions(w http.ResponseWriter, r *http.Request) {
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	sessions, err := h.sessions.List(r.Context(), userID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, sessions)
}

// RevokeSession handles DELETE /auth/sessions/{sessionID}.
func (h *Handlers) RevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	sessionID, err := httpx.RequirePathParam(r, "sessionID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	ip, ua := clientMeta(r)
	if err := h.sessions.Revoke(r.Context(), userID, sessionID, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

// LogoutAll handles POST /auth/logout-all.
func (h *Handlers) LogoutAll(w http.ResponseWriter, r *http.Request) {
	userID, err := tenant.RequireUser(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	except := tenant.SessionID(r.Context())
	ip, ua := clientMeta(r)
	if _, err := h.sessions.RevokeAll(r.Context(), userID, except, ip, ua); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

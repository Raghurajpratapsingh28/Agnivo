// Package tenant provides organization-scoped request context propagation for
// strict multi-tenant isolation. Every authenticated request carries a user;
// org-scoped routes additionally carry an organization validated against
// membership.
package tenant

import (
	"context"

	"github.com/agnivo/agnivo/packages/platform/errors"
)

type ctxKey int

const (
	userKey ctxKey = iota
	orgKey
	sessionKey
	memberRoleKey
	authMethodKey
)

// AuthMethod describes how the caller authenticated.
type AuthMethod string

const (
	AuthJWT    AuthMethod = "jwt"
	AuthAPIKey AuthMethod = "api_key"
	AuthPAT    AuthMethod = "pat"
)

// WithUser stores the authenticated user ID in ctx.
func WithUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userKey, userID)
}

// UserID returns the authenticated user ID, or "" when absent.
func UserID(ctx context.Context) string {
	id, _ := ctx.Value(userKey).(string)
	return id
}

// RequireUser returns the user ID or CodeUnauthenticated.
func RequireUser(ctx context.Context) (string, error) {
	id := UserID(ctx)
	if id == "" {
		return "", errors.Unauthenticated("authentication required")
	}
	return id, nil
}

// WithOrg stores the validated organization ID in ctx.
func WithOrg(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, orgKey, orgID)
}

// OrgID returns the organization ID from ctx, or "".
func OrgID(ctx context.Context) string {
	id, _ := ctx.Value(orgKey).(string)
	return id
}

// RequireOrg returns the org ID or CodeFailedPrecond when missing.
func RequireOrg(ctx context.Context) (string, error) {
	id := OrgID(ctx)
	if id == "" {
		return "", errors.FailedPrecondition("organization context required")
	}
	return id, nil
}

// WithSession stores the session ID in ctx.
func WithSession(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionKey, sessionID)
}

// SessionID returns the session ID from ctx.
func SessionID(ctx context.Context) string {
	id, _ := ctx.Value(sessionKey).(string)
	return id
}

// WithMemberRole stores the caller's role in the current organization.
func WithMemberRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, memberRoleKey, role)
}

// MemberRole returns the member role in the current org.
func MemberRole(ctx context.Context) string {
	r, _ := ctx.Value(memberRoleKey).(string)
	return r
}

// WithAuthMethod records how the request was authenticated.
func WithAuthMethod(ctx context.Context, m AuthMethod) context.Context {
	return context.WithValue(ctx, authMethodKey, m)
}

// AuthMethodOf returns the authentication method used.
func AuthMethodOf(ctx context.Context) AuthMethod {
	m, _ := ctx.Value(authMethodKey).(AuthMethod)
	return m
}

// AssertOrgMatch returns PermissionDenied when resourceOrgID does not match the
// request org context. Use in services to enforce row-level tenant isolation.
func AssertOrgMatch(ctx context.Context, resourceOrgID string) error {
	ctxOrg, err := RequireOrg(ctx)
	if err != nil {
		return err
	}
	if resourceOrgID != ctxOrg {
		return errors.PermissionDenied("cross-organization access denied")
	}
	return nil
}

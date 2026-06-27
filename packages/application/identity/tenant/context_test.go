package tenant_test

import (
	"context"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextPropagation(t *testing.T) {
	ctx := context.Background()
	ctx = tenant.WithUser(ctx, "user-1")
	ctx = tenant.WithOrg(ctx, "org-1")
	ctx = tenant.WithSession(ctx, "sess-1")
	ctx = tenant.WithMemberRole(ctx, "admin")
	ctx = tenant.WithAuthMethod(ctx, tenant.AuthJWT)

	assert.Equal(t, "user-1", tenant.UserID(ctx))
	assert.Equal(t, "org-1", tenant.OrgID(ctx))
	assert.Equal(t, "sess-1", tenant.SessionID(ctx))
	assert.Equal(t, "admin", tenant.MemberRole(ctx))
	assert.Equal(t, tenant.AuthJWT, tenant.AuthMethodOf(ctx))
}

func TestRequireUser(t *testing.T) {
	_, err := tenant.RequireUser(context.Background())
	require.Error(t, err)

	id, err := tenant.RequireUser(tenant.WithUser(context.Background(), "u"))
	require.NoError(t, err)
	assert.Equal(t, "u", id)
}

func TestAssertOrgMatch(t *testing.T) {
	ctx := tenant.WithOrg(context.Background(), "org-a")
	require.NoError(t, tenant.AssertOrgMatch(ctx, "org-a"))
	err := tenant.AssertOrgMatch(ctx, "org-b")
	require.Error(t, err)
}

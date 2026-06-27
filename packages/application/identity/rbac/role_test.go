package rbac_test

import (
	"testing"

	"github.com/agnivo/agnivo/packages/application/identity/rbac"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasPermission(t *testing.T) {
	assert.True(t, rbac.HasPermission(rbac.RoleOwner, rbac.PermOrgDelete))
	assert.True(t, rbac.HasPermission(rbac.RoleAdmin, rbac.PermMemberInvite))
	assert.False(t, rbac.HasPermission(rbac.RoleViewer, rbac.PermMemberInvite))
	assert.False(t, rbac.HasPermission(rbac.RoleBilling, rbac.PermDeployWrite))
}

func TestCanManageRole(t *testing.T) {
	assert.True(t, rbac.CanManageRole(rbac.RoleOwner, rbac.RoleAdmin))
	assert.False(t, rbac.CanManageRole(rbac.RoleAdmin, rbac.RoleOwner))
	assert.True(t, rbac.CanManageRole(rbac.RoleAdmin, rbac.RoleDeveloper))
	assert.False(t, rbac.CanManageRole(rbac.RoleDeveloper, rbac.RoleViewer))
}

func TestRequirePermission(t *testing.T) {
	require.NoError(t, rbac.RequirePermission(rbac.RoleOwner, rbac.PermOrgWrite))
	err := rbac.RequirePermission(rbac.RoleViewer, rbac.PermOrgWrite)
	require.Error(t, err)
}

func TestParseRole(t *testing.T) {
	r, err := rbac.ParseRole("developer")
	require.NoError(t, err)
	assert.Equal(t, rbac.RoleDeveloper, r)

	_, err = rbac.ParseRole("superuser")
	require.Error(t, err)
}

func TestAllRolesComplete(t *testing.T) {
	for _, role := range rbac.AllRoles {
		assert.True(t, rbac.HasPermission(role, rbac.PermOrgRead) || role == rbac.RoleBilling,
			"role %s should have org:read or be billing-only", role)
	}
}

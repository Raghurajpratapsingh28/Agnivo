// Package rbac implements enterprise role-based access control for organizations.
package rbac

import "github.com/agnivo/agnivo/packages/platform/errors"

// Role is an organization membership role.
type Role string

const (
	RoleOwner     Role = "owner"
	RoleAdmin     Role = "admin"
	RoleDeveloper Role = "developer"
	RoleOperator  Role = "operator"
	RoleBilling   Role = "billing"
	RoleViewer    Role = "viewer"
)

// AllRoles lists every valid role in privilege order (highest first).
var AllRoles = []Role{RoleOwner, RoleAdmin, RoleDeveloper, RoleOperator, RoleBilling, RoleViewer}

// Permission is a fine-grained capability checked by services and middleware.
type Permission string

const (
	PermOrgRead       Permission = "org:read"
	PermOrgWrite      Permission = "org:write"
	PermOrgDelete     Permission = "org:delete"
	PermMemberRead    Permission = "member:read"
	PermMemberInvite  Permission = "member:invite"
	PermMemberManage  Permission = "member:manage"
	PermMemberRemove  Permission = "member:remove"
	PermAPIKeyRead    Permission = "apikey:read"
	PermAPIKeyManage  Permission = "apikey:manage"
	PermSessionRead   Permission = "session:read"
	PermSessionRevoke Permission = "session:revoke"
	PermBillingRead   Permission = "billing:read"
	PermBillingManage Permission = "billing:manage"
	PermDeployRead    Permission = "deploy:read"
	PermDeployWrite   Permission = "deploy:write"
	PermProjectRead   Permission = "project:read"
	PermProjectWrite  Permission = "project:write"
)

// rolePermissions maps each role to its granted permissions. Owner inherits all.
var rolePermissions = map[Role]map[Permission]struct{}{
	RoleOwner: permSet(allPermissions()...),
	RoleAdmin: permSet(
		PermOrgRead, PermOrgWrite,
		PermMemberRead, PermMemberInvite, PermMemberManage, PermMemberRemove,
		PermAPIKeyRead, PermAPIKeyManage,
		PermSessionRead, PermSessionRevoke,
		PermDeployRead, PermDeployWrite, PermProjectRead, PermProjectWrite,
	),
	RoleDeveloper: permSet(
		PermOrgRead, PermMemberRead,
		PermAPIKeyRead,
		PermSessionRead, PermSessionRevoke,
		PermDeployRead, PermDeployWrite, PermProjectRead, PermProjectWrite,
	),
	RoleOperator: permSet(
		PermOrgRead, PermMemberRead,
		PermDeployRead, PermDeployWrite, PermProjectRead,
	),
	RoleBilling: permSet(PermOrgRead, PermBillingRead, PermBillingManage),
	RoleViewer:  permSet(PermOrgRead, PermMemberRead, PermDeployRead, PermProjectRead),
}

func allPermissions() []Permission {
	return []Permission{
		PermOrgRead, PermOrgWrite, PermOrgDelete,
		PermMemberRead, PermMemberInvite, PermMemberManage, PermMemberRemove,
		PermAPIKeyRead, PermAPIKeyManage,
		PermSessionRead, PermSessionRevoke,
		PermBillingRead, PermBillingManage,
		PermDeployRead, PermDeployWrite, PermProjectRead, PermProjectWrite,
	}
}

func permSet(perms ...Permission) map[Permission]struct{} {
	m := make(map[Permission]struct{}, len(perms))
	for _, p := range perms {
		m[p] = struct{}{}
	}
	return m
}

// ParseRole validates and normalizes a role string.
func ParseRole(s string) (Role, error) {
	r := Role(s)
	if _, ok := rolePermissions[r]; !ok {
		return "", errors.InvalidArgument("invalid role: " + s)
	}
	return r, nil
}

// HasPermission reports whether role grants perm.
func HasPermission(role Role, perm Permission) bool {
	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	_, ok = perms[perm]
	return ok
}

// CanManageRole reports whether actor may assign targetRole (owners can assign
// anything except owner transfer; admins cannot assign owner).
func CanManageRole(actor Role, targetRole Role) bool {
	if actor == RoleOwner {
		return true
	}
	if actor == RoleAdmin {
		return targetRole != RoleOwner
	}
	return false
}

// RequirePermission returns PermissionDenied when role lacks perm.
func RequirePermission(role Role, perm Permission) error {
	if !HasPermission(role, perm) {
		return errors.PermissionDenied("insufficient permissions")
	}
	return nil
}

// Package rbac defines a small, pure role-based access-control model: a fixed
// set of roles, each granting a set of permissions. It performs no I/O and can
// back authorization checks or settings screens.
package rbac

import "sort"

// Role identifies a permission set.
type Role string

const (
	RoleOwner    Role = "owner"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// Permission identifies a capability.
type Permission string

const (
	PermRead    Permission = "read"
	PermWrite   Permission = "write"
	PermApprove Permission = "approve"
	PermAdmin   Permission = "admin"
)

// rolePermissions is the canonical grant table.
var rolePermissions = map[Role]map[Permission]bool{
	RoleOwner:    {PermRead: true, PermWrite: true, PermApprove: true, PermAdmin: true},
	RoleOperator: {PermRead: true, PermWrite: true, PermApprove: true},
	RoleViewer:   {PermRead: true},
}

// Can reports whether role grants permission. Unknown roles grant nothing.
func Can(role Role, perm Permission) bool {
	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	return perms[perm]
}

// IsRole reports whether the given string is a known role.
func IsRole(role Role) bool {
	_, ok := rolePermissions[role]
	return ok
}

// Roles returns all known roles sorted for stable output.
func Roles() []Role {
	out := make([]Role, 0, len(rolePermissions))
	for r := range rolePermissions {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Permissions returns the permissions granted to a role, sorted.
func Permissions(role Role) []Permission {
	perms := rolePermissions[role]
	out := make([]Permission, 0, len(perms))
	for p, granted := range perms {
		if granted {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

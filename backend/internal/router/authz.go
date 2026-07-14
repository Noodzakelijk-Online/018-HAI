package router

import (
	"net/http"
	"strings"

	"automation-hub-backend/internal/apierror"
	"automation-hub-backend/internal/rbac"

	"github.com/gin-gonic/gin"
)

const roleHeader = "X-HAI-Role"

// resolveRole returns the caller's effective role. It prefers a role established
// by a verified IDP JWT (identityMiddleware), falls back to the gateway-
// propagated X-HAI-Role header, and otherwise defaults to the least-privilege
// viewer — so a caller with only the shared API key can read but not mutate.
func resolveRole(c *gin.Context) rbac.Role {
	roleStr, _ := c.Get(contextRoleKey)
	role := rbac.Role(toRoleString(roleStr))
	if !rbac.IsRole(role) {
		role = rbac.Role(strings.ToLower(strings.TrimSpace(c.GetHeader(roleHeader))))
	}
	if !rbac.IsRole(role) {
		role = rbac.RoleViewer
	}
	return role
}

// permissionForMethod maps an HTTP method to the permission it requires: safe
// (read) methods need PermRead, everything that can mutate needs PermWrite.
func permissionForMethod(method string) rbac.Permission {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return rbac.PermRead
	default:
		return rbac.PermWrite
	}
}

func forbid(c *gin.Context, role rbac.Role, perm rbac.Permission) {
	err := apierror.New(apierror.CodeForbidden, "role does not grant the required permission").
		WithDetail("requiredPermission", string(perm)).
		WithDetail("role", string(role))
	c.AbortWithStatusJSON(err.HTTPStatus(), err.Envelope())
}

// enforcePermissions gates every request on the permission its HTTP method
// requires, wiring the RBAC model across the whole API surface rather than
// leaving it as unused, separately-tested code. Reads need viewer; mutations
// need operator/owner. Because an unauthenticated caller resolves to viewer, a
// leaked shared key alone can read but cannot create, update, or delete —
// mutations require an authenticated identity (the gateway propagates the
// operator/owner role for a verified session).
func enforcePermissions() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := resolveRole(c)
		perm := permissionForMethod(c.Request.Method)
		if !rbac.Can(role, perm) {
			forbid(c, role, perm)
			return
		}
		c.Set(contextRoleKey, string(role))
		c.Next()
	}
}

// requirePermission enforces a specific permission on a route, for operations
// whose sensitivity is not captured by the HTTP method alone (e.g. an admin-only
// action served over POST). It composes with enforcePermissions.
func requirePermission(perm rbac.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := resolveRole(c)
		if !rbac.Can(role, perm) {
			forbid(c, role, perm)
			return
		}
		c.Set(contextRoleKey, string(role))
		c.Next()
	}
}

func toRoleString(v any) string {
	if s, ok := v.(string); ok {
		return strings.ToLower(strings.TrimSpace(s))
	}
	return ""
}

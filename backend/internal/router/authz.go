package router

import (
	"strings"

	"automation-hub-backend/internal/apierror"
	"automation-hub-backend/internal/rbac"

	"github.com/gin-gonic/gin"
)

const roleHeader = "X-HAI-Role"

// requirePermission enforces that the caller's role (from the X-HAI-Role header)
// grants the required permission. Absent/unknown roles default to viewer
// (least privilege), so a missing role can only ever read. This wires the RBAC
// model into real request handling.
func requirePermission(perm rbac.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prefer a role established by a verified IDP JWT (identityMiddleware);
		// fall back to the X-HAI-Role header (gateway-propagated), then to the
		// least-privilege viewer default.
		roleStr, _ := c.Get(contextRoleKey)
		role := rbac.Role(toRoleString(roleStr))
		if !rbac.IsRole(role) {
			role = rbac.Role(strings.ToLower(strings.TrimSpace(c.GetHeader(roleHeader))))
		}
		if !rbac.IsRole(role) {
			role = rbac.RoleViewer
		}
		if !rbac.Can(role, perm) {
			// Live adoption of the shared apierror envelope on a real route.
			err := apierror.New(apierror.CodeForbidden, "role does not grant the required permission").
				WithDetail("requiredPermission", string(perm)).
				WithDetail("role", string(role))
			c.AbortWithStatusJSON(err.HTTPStatus(), err.Envelope())
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

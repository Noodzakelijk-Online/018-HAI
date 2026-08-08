package router

import (
	"strings"

	"automation-hub-backend/internal/apierror"
	"automation-hub-backend/internal/rbac"

	"github.com/gin-gonic/gin"
)

// requirePermission enforces the role established by identityMiddleware from a
// verified IDP JWT. Absent or unknown roles default to viewer (least privilege).
// Request headers must never grant authority: callers can always forge them.
func requirePermission(perm rbac.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		// identityMiddleware writes this only after verifying the JWT signature.
		roleStr, _ := c.Get(contextRoleKey)
		role := rbac.Role(toRoleString(roleStr))
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

// requireRecognizedRole rejects malformed or newly introduced role values at
// security-sensitive module boundaries. The legacy permission middleware keeps
// its viewer fallback for existing read-only routes, but governance modules
// must not silently reinterpret an unknown verified claim as a known role.
func requireRecognizedRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		roleValue, _ := c.Get(contextRoleKey)
		role := rbac.Role(toRoleString(roleValue))
		if !rbac.IsRole(role) {
			err := apierror.New(
				apierror.CodeForbidden,
				"identity token contains an unsupported role",
			).WithDetail("role", string(role))
			c.AbortWithStatusJSON(err.HTTPStatus(), err.Envelope())
			return
		}
		c.Next()
	}
}

func toRoleString(v any) string {
	if s, ok := v.(string); ok {
		return strings.ToLower(strings.TrimSpace(s))
	}
	return ""
}

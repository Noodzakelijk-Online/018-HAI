package router

import (
	"net/http"
	"strings"

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
		role := rbac.Role(strings.ToLower(strings.TrimSpace(c.GetHeader(roleHeader))))
		if !rbac.IsRole(role) {
			role = rbac.RoleViewer
		}
		if !rbac.Can(role, perm) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "forbidden: role does not grant the required permission",
			})
			return
		}
		c.Set("role", string(role))
		c.Next()
	}
}

package router

import (
	"strings"
	"time"

	"automation-hub-backend/internal/apierror"
	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

const (
	contextRoleKey    = "role"
	contextSubjectKey = "subject"
)

// identityMiddleware resolves a per-user identity from an IDP-issued JWT
// (Authorization: Bearer <token>), verified against the shared JWT secret, and
// stores the caller's role + subject in the request context for RBAC. It is a
// no-op when no JWT secret is configured or no bearer token is presented — so
// the shared-API-key single-operator model keeps working — but a *present but
// invalid* token is rejected with 401 rather than silently ignored.
func identityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		secret := strings.TrimSpace(config.AppConfig.JWTSecret)
		token := bearerToken(c)
		if secret == "" || token == "" {
			c.Next()
			return
		}
		claims, err := identity.Verify(token, secret, time.Now())
		if err != nil {
			e := apierror.New(apierror.CodeUnauthorized, "invalid or expired identity token")
			c.AbortWithStatusJSON(e.HTTPStatus(), e.Envelope())
			return
		}
		if claims.Role != "" {
			c.Set(contextRoleKey, strings.ToLower(strings.TrimSpace(claims.Role)))
		}
		if claims.Subject != "" {
			c.Set(contextSubjectKey, claims.Subject)
		}
		c.Next()
	}
}

func bearerToken(c *gin.Context) string {
	h := strings.TrimSpace(c.GetHeader("Authorization"))
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

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
	contextRoleKey    = identity.ContextRoleKey
	contextSubjectKey = identity.ContextSubjectKey
)

// identityMiddleware resolves a per-user identity from an IDP-issued JWT.
// API clients may use Authorization: Bearer <token>; browser requests use
// HAI's HttpOnly access_token cookie. Claims are verified against the shared
// JWT secret before their role and principal are used for RBAC or auditing.
// The middleware is a no-op when no JWT secret is configured or no token is
// presented, so the shared-API-key single-operator model keeps working. A
// present but invalid token is rejected with 401 rather than silently ignored.
func identityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		secret := strings.TrimSpace(config.AppConfig.JWTSecret)
		token := identityToken(c)
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
		if principal := claims.Principal(); principal != "" {
			c.Set(contextSubjectKey, principal)
		}
		c.Next()
	}
}

// requireAuthenticatedOwner separates browser and API requests from controlled
// in-process workers. Owner-scoped handlers must not interpret a missing
// principal as a global or legacy owner when a request crosses the HTTP
// boundary.
func requireAuthenticatedOwner() gin.HandlerFunc {
	return func(c *gin.Context) {
		value, ok := c.Get(contextSubjectKey)
		owner, ok := value.(string)
		if !ok || strings.TrimSpace(owner) == "" {
			e := apierror.New(apierror.CodeUnauthorized, "an authenticated owner session is required for this operation")
			c.AbortWithStatusJSON(e.HTTPStatus(), e.Envelope())
			return
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

// identityToken first accepts an explicit bearer token for API clients and then
// falls back to HAI's HttpOnly IDP cookie. The gateway already validates that
// cookie before forwarding protected API requests; the backend verifies the
// signature again before using it for audit attribution.
func identityToken(c *gin.Context) string {
	if token := bearerToken(c); token != "" {
		return token
	}
	token, err := c.Cookie("access_token")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(token)
}

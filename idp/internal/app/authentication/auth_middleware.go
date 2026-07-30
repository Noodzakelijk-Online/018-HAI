package authentication

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func AuthMiddleware(h *Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		accessToken, ok := h.resolveAuthenticatedAccessToken(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Please login again"})
			return
		}
		c.Set(authenticatedTokenContextKey, accessToken)

		// Resolve identity only after a refresh succeeds. An expired access token
		// must not make a valid refresh session unusable for protected routes.
		userID, err := h.authService.GetIdFromToken(accessToken)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}
		c.Set("userID", userID)

		c.Next()
	}
}

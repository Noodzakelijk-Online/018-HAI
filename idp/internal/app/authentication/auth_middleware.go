package authentication

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func AuthMiddleware(h *Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		accessToken, err := c.Cookie("access_token")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Please login again"})
			return
		}
		refreshToken, err := c.Cookie("refresh_token")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Please login again"})
			return
		}

		if isValid, _ := h.authService.IsUserAuthenticated(accessToken); !isValid {
			newAccessToken, err := h.authService.RefreshToken(refreshToken)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Please login again"})
				return
			}

			accessToken = newAccessToken.AccessToken
			atExpiresTime := time.Unix(newAccessToken.AtExpires, 0)
			setAccessTokenCookie(c.Writer, accessToken, atExpiresTime)
		}

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

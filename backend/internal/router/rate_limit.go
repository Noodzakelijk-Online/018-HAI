package router

import (
	"net/http"
	"time"

	"automation-hub-backend/internal/ratelimit"

	"github.com/gin-gonic/gin"
)

// rateLimitMiddleware enforces a per-client-IP fixed-window rate limit. When the
// limiter is disabled (RATE_LIMIT_PER_MINUTE <= 0) it is a pass-through, so the
// default behaviour is unchanged. Over-limit requests receive 429 with a
// Retry-After hint.
func rateLimitMiddleware(limiter *ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !limiter.Enabled() {
			c.Next()
			return
		}
		if !limiter.Allow(c.ClientIP(), time.Now()) {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded; retry after the window resets"})
			return
		}
		c.Next()
	}
}

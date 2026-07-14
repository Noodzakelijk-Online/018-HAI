package router

import (
	"math"
	"net/http"
	"strconv"

	"automation-hub-backend/internal/ratelimit"

	"github.com/gin-gonic/gin"
)

// rateLimitMiddleware enforces a per-client-IP fixed-window rate limit. When the
// enforcer is disabled (RATE_LIMIT_PER_MINUTE <= 0) it is a pass-through, so the
// default behaviour is unchanged. Over-limit requests receive 429 with a
// Retry-After hint derived from the actual window reset.
func rateLimitMiddleware(enforcer ratelimit.Enforcer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enforcer.Enabled() {
			c.Next()
			return
		}
		decision := enforcer.Allow(c.Request.Context(), c.ClientIP())
		if !decision.Allowed {
			retryAfter := int(math.Ceil(decision.RetryAfter.Seconds()))
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded; retry after the window resets"})
			return
		}
		c.Next()
	}
}

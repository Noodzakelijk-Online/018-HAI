package router

import (
	"net/http"
	"strings"
	"time"

	"automation-hub-backend/internal/idempotency"

	"github.com/gin-gonic/gin"
)

const idempotencyKeyHeader = "Idempotency-Key"

// idempotencyMiddleware rejects a repeated state-changing request that reuses an
// Idempotency-Key already seen within the store's window, returning 409. It is
// strictly opt-in: requests without the header, and safe methods (GET/HEAD/
// OPTIONS), pass through untouched, so default behaviour is unchanged.
func idempotencyMiddleware(store *idempotency.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.GetHeader(idempotencyKeyHeader))
		if key == "" || isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}
		if !store.FirstSeen(c.Request.Method+" "+c.FullPath()+" "+key, time.Now()) {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{
				"error": "duplicate request: this Idempotency-Key was already processed",
			})
			return
		}
		c.Next()
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

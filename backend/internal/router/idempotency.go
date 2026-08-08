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
		if key == "" || isSafeMethod(c.Request.Method) || usesDurableIdempotency(c.Request.URL.Path) {
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

// Task plan/run endpoints own a PostgreSQL-backed, owner-scoped idempotency
// contract that can replay the exact durable result and detect changed input.
// The legacy process-local middleware must not reject those safe replays before
// they reach the authoritative operation ledger.
func usesDurableIdempotency(path string) bool {
	cleanPath := strings.TrimSuffix(strings.TrimSpace(path), "/")
	switch cleanPath {
	case "/api/v1/task/plan", "/api/v1/task/run", "/api/v1/task/success", "/api/v1/proactivity/feedback":
		return true
	}
	parts := strings.Split(strings.Trim(cleanPath, "/"), "/")
	return len(parts) == 6 && parts[0] == "api" && parts[1] == "v1" &&
		parts[2] == "workflow" && parts[4] != "" &&
		((parts[3] == "reminder-proposals" && parts[5] == "activation-requests") ||
			(parts[3] == "reminder-activation-requests" && parts[5] == "decisions"))
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

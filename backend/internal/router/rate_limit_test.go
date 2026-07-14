package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"automation-hub-backend/internal/ratelimit"

	"github.com/gin-gonic/gin"
)

func newRateLimitedEngine(limit int, window time.Duration) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(rateLimitMiddleware(ratelimit.Memory(limit, window)))
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	return r
}

func TestRateLimitMiddlewareBlocksOverLimit(t *testing.T) {
	// 2 requests/minute; the third within the window must be rejected.
	engine := newRateLimitedEngine(2, time.Minute)
	codes := []int{}
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "203.0.113.7:12345"
		engine.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
		if i == 2 {
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("3rd request code = %d, want 429", rec.Code)
			}
			if rec.Header().Get("Retry-After") == "" {
				t.Fatalf("429 response missing Retry-After header")
			}
		}
	}
	if codes[0] != http.StatusOK || codes[1] != http.StatusOK {
		t.Fatalf("first two requests = %v, want 200/200", codes[:2])
	}
}

func TestRateLimitMiddlewareDisabledIsPassthrough(t *testing.T) {
	engine := newRateLimitedEngine(0, time.Minute)
	for i := 0; i < 50; i++ {
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("disabled limiter blocked request %d (code %d)", i, rec.Code)
		}
	}
}

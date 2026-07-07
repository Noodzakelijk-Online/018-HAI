package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"automation-hub-backend/internal/idempotency"

	"github.com/gin-gonic/gin"
)

func newIdempotentEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(idempotencyMiddleware(idempotency.New(time.Minute)))
	r.POST("/api/v1/memory/", func(c *gin.Context) { c.JSON(http.StatusCreated, gin.H{"ok": true}) })
	r.GET("/api/v1/memory/", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func postWithKey(engine *gin.Engine, method, key string) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/api/v1/memory/", nil)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	engine.ServeHTTP(rec, req)
	return rec.Code
}

func TestIdempotencyBlocksDuplicateMutation(t *testing.T) {
	engine := newIdempotentEngine()
	if code := postWithKey(engine, http.MethodPost, "abc-123"); code != http.StatusCreated {
		t.Fatalf("first POST = %d, want 201", code)
	}
	if code := postWithKey(engine, http.MethodPost, "abc-123"); code != http.StatusConflict {
		t.Fatalf("duplicate POST = %d, want 409", code)
	}
}

func TestIdempotencyIgnoresRequestsWithoutKey(t *testing.T) {
	engine := newIdempotentEngine()
	for i := 0; i < 3; i++ {
		if code := postWithKey(engine, http.MethodPost, ""); code != http.StatusCreated {
			t.Fatalf("keyless POST %d = %d, want 201 (opt-in only)", i, code)
		}
	}
}

func TestIdempotencyIgnoresSafeMethods(t *testing.T) {
	engine := newIdempotentEngine()
	if code := postWithKey(engine, http.MethodGet, "same-key"); code != http.StatusOK {
		t.Fatalf("first GET = %d, want 200", code)
	}
	if code := postWithKey(engine, http.MethodGet, "same-key"); code != http.StatusOK {
		t.Fatalf("repeat GET = %d, want 200 (GET is safe, never blocked)", code)
	}
}

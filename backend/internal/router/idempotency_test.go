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

func TestIdempotencyDefersTaskRoutesToDurableOperationLedger(t *testing.T) {
	r := gin.New()
	r.Use(idempotencyMiddleware(idempotency.New(time.Minute)))
	requests := 0
	r.POST("/api/v1/task/run", func(c *gin.Context) {
		requests++
		c.Status(http.StatusOK)
	})
	for index := 0; index < 2; index++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/task/run", nil)
		req.Header.Set("Idempotency-Key", "durable-task-key")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("request %d status = %d", index, response.Code)
		}
	}
	if requests != 2 {
		t.Fatalf("durable task handler calls = %d, want 2", requests)
	}
}

func TestIdempotencyDefersProactivityFeedbackToDurableOwnerLedger(t *testing.T) {
	r := gin.New()
	r.Use(idempotencyMiddleware(idempotency.New(time.Minute)))
	requests := 0
	r.POST("/api/v1/proactivity/feedback", func(c *gin.Context) {
		requests++
		c.Status(http.StatusOK)
	})
	for index := 0; index < 2; index++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/proactivity/feedback", nil)
		req.Header.Set("Idempotency-Key", "durable-feedback-key")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("request %d status = %d", index, response.Code)
		}
	}
	if requests != 2 {
		t.Fatalf("durable feedback handler calls = %d, want 2", requests)
	}
}

func TestIdempotencyDefersReminderRoutesToDurableOwnerLedger(t *testing.T) {
	r := gin.New()
	r.Use(idempotencyMiddleware(idempotency.New(time.Minute)))
	requests := 0
	for _, route := range []string{
		"/api/v1/workflow/reminder-proposals/:itemId/activation-requests",
		"/api/v1/workflow/reminder-activation-requests/:requestId/decisions",
	} {
		r.POST(route, func(c *gin.Context) {
			requests++
			c.Status(http.StatusOK)
		})
	}
	paths := []string{
		"/api/v1/workflow/reminder-proposals/checklist-1/activation-requests",
		"/api/v1/workflow/reminder-activation-requests/request-1/decisions",
	}
	for _, path := range paths {
		for attempt := 0; attempt < 2; attempt++ {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.Header.Set("Idempotency-Key", "durable-reminder-key")
			response := httptest.NewRecorder()
			r.ServeHTTP(response, req)
			if response.Code != http.StatusOK {
				t.Fatalf("%s attempt %d status = %d", path, attempt, response.Code)
			}
		}
	}
	if requests != 4 {
		t.Fatalf("durable reminder handler calls = %d, want 4", requests)
	}
}

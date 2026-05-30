package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Mirrors the exact path set registered in initializeAutomationsRoutes to
// confirm gin builds the route tree without panicking (static + param at the
// same level) and resolves each new endpoint to the right handler.
func TestAutomationRoutesNoConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	hit := ""
	mark := func(name string) gin.HandlerFunc {
		return func(c *gin.Context) {
			hit = name
			c.Status(http.StatusOK)
		}
	}

	a := r.Group("/api/v1").Group("/automation")
	a.GET("/swap/:id1/:id2", mark("swap"))
	a.GET("/", mark("getAll"))
	a.GET("/health/summary", mark("summary"))
	a.GET("/:id", mark("getByID"))
	a.POST("/:id/health-check", mark("healthCheck"))
	a.GET("/:id/diagnostics", mark("diagnostics"))
	a.POST("/", mark("create"))
	a.PATCH("/", mark("update"))
	a.DELETE("/:id", mark("delete"))
	a.GET("/images/:imageName", mark("image"))

	cases := []struct {
		method, path, want string
	}{
		{"GET", "/api/v1/automation/health/summary", "summary"},
		{"POST", "/api/v1/automation/abc/health-check", "healthCheck"},
		{"GET", "/api/v1/automation/abc/diagnostics", "diagnostics"},
		{"GET", "/api/v1/automation/abc", "getByID"},
		{"GET", "/api/v1/automation/images/logo.png", "image"},
		{"GET", "/api/v1/automation/swap/1/2", "swap"},
	}
	for _, tc := range cases {
		hit = ""
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s -> code %d, want 200", tc.method, tc.path, w.Code)
		}
		if hit != tc.want {
			t.Fatalf("%s %s -> handler %q, want %q", tc.method, tc.path, hit, tc.want)
		}
	}
}

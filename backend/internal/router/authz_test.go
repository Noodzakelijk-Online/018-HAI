package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/rbac"

	"github.com/gin-gonic/gin"
)

func newAuthzEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	admin := r.Group("/api/v1/admin")
	admin.Use(requirePermission(rbac.PermAdmin))
	admin.GET("/thing", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	read := r.Group("/api/v1/read")
	read.Use(requirePermission(rbac.PermRead))
	read.GET("/thing", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func requestWithRole(engine *gin.Engine, path, role string) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if role != "" {
		req.Header.Set("X-HAI-Role", role)
	}
	engine.ServeHTTP(rec, req)
	return rec.Code
}

func TestAdminRouteRequiresAdminRole(t *testing.T) {
	e := newAuthzEngine()
	if code := requestWithRole(e, "/api/v1/admin/thing", "owner"); code != http.StatusOK {
		t.Fatalf("owner should access admin route, got %d", code)
	}
	if code := requestWithRole(e, "/api/v1/admin/thing", "operator"); code != http.StatusForbidden {
		t.Fatalf("operator lacks admin, want 403, got %d", code)
	}
	if code := requestWithRole(e, "/api/v1/admin/thing", "viewer"); code != http.StatusForbidden {
		t.Fatalf("viewer lacks admin, want 403, got %d", code)
	}
}

func TestMissingOrUnknownRoleDefaultsToViewer(t *testing.T) {
	e := newAuthzEngine()
	// No role → viewer → can read.
	if code := requestWithRole(e, "/api/v1/read/thing", ""); code != http.StatusOK {
		t.Fatalf("no role should default to viewer and read, got %d", code)
	}
	// No role → viewer → cannot admin.
	if code := requestWithRole(e, "/api/v1/admin/thing", ""); code != http.StatusForbidden {
		t.Fatalf("no role must not reach admin, got %d", code)
	}
	// Unknown role → treated as viewer.
	if code := requestWithRole(e, "/api/v1/admin/thing", "superhacker"); code != http.StatusForbidden {
		t.Fatalf("unknown role must not reach admin, got %d", code)
	}
}

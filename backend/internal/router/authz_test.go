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

func newMethodGatedEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.Use(enforcePermissions())
	v1.GET("/thing", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	v1.POST("/thing", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	v1.DELETE("/thing/:id", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func methodRequest(engine *gin.Engine, method, path, role string) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	if role != "" {
		req.Header.Set("X-HAI-Role", role)
	}
	engine.ServeHTTP(rec, req)
	return rec.Code
}

// A caller with no role (e.g. only the shared API key) is a viewer: it can read
// but must not mutate. This is the core hardening — a leaked key cannot write.
func TestEnforcePermissionsViewerReadsButCannotMutate(t *testing.T) {
	e := newMethodGatedEngine()
	if code := methodRequest(e, http.MethodGet, "/api/v1/thing", ""); code != http.StatusOK {
		t.Fatalf("viewer GET = %d, want 200", code)
	}
	if code := methodRequest(e, http.MethodPost, "/api/v1/thing", ""); code != http.StatusForbidden {
		t.Fatalf("viewer POST = %d, want 403", code)
	}
	if code := methodRequest(e, http.MethodDelete, "/api/v1/thing/1", ""); code != http.StatusForbidden {
		t.Fatalf("viewer DELETE = %d, want 403", code)
	}
}

// The gateway propagates owner for an authenticated session, so the real UI can
// read and mutate. Operator (read+write) can too.
func TestEnforcePermissionsOwnerAndOperatorCanMutate(t *testing.T) {
	e := newMethodGatedEngine()
	for _, role := range []string{"owner", "operator"} {
		if code := methodRequest(e, http.MethodPost, "/api/v1/thing", role); code != http.StatusOK {
			t.Fatalf("%s POST = %d, want 200", role, code)
		}
		if code := methodRequest(e, http.MethodDelete, "/api/v1/thing/1", role); code != http.StatusOK {
			t.Fatalf("%s DELETE = %d, want 200", role, code)
		}
	}
}

func TestPermissionForMethod(t *testing.T) {
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		if permissionForMethod(m) != rbac.PermRead {
			t.Fatalf("%s should require read", m)
		}
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if permissionForMethod(m) != rbac.PermWrite {
			t.Fatalf("%s should require write", m)
		}
	}
}

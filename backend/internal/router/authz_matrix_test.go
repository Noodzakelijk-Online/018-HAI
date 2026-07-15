package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/rbac"

	"github.com/gin-gonic/gin"
)

// TestForbiddenUsesApiErrorEnvelope proves the live RBAC middleware returns the
// shared apierror envelope (not an ad-hoc shape).
func TestForbiddenUsesApiErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	engine := newAuthzEngine()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/thing", nil)
	req.Header.Set("X-Test-Verified-Role", "viewer")
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
	var body struct {
		Error struct {
			Code    string            `json:"code"`
			Message string            `json:"message"`
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	if body.Error.Code != "forbidden" {
		t.Fatalf("error.code = %q, want forbidden", body.Error.Code)
	}
	if body.Error.Details["requiredPermission"] != "admin" || body.Error.Details["role"] != "viewer" {
		t.Fatalf("envelope details missing/wrong: %+v", body.Error.Details)
	}
}

// TestRolePermissionMatrixEnforcement exercises read/write/approve/admin routes
// across owner/operator/viewer and confirms each role gets exactly its grants.
func TestRolePermissionMatrixEnforcement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		if role := c.GetHeader("X-Test-Verified-Role"); role != "" {
			c.Set(contextRoleKey, role)
		}
		c.Next()
	})
	for path, perm := range map[string]rbac.Permission{
		"/read":    rbac.PermRead,
		"/write":   rbac.PermWrite,
		"/approve": rbac.PermApprove,
		"/admin":   rbac.PermAdmin,
	} {
		g := engine.Group("/api/v1" + path)
		g.Use(requirePermission(perm))
		g.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	}

	// role -> expected-allowed paths
	expect := map[string]map[string]bool{
		"owner":    {"/read": true, "/write": true, "/approve": true, "/admin": true},
		"operator": {"/read": true, "/write": true, "/approve": true, "/admin": false},
		"viewer":   {"/read": true, "/write": false, "/approve": false, "/admin": false},
	}
	for role, paths := range expect {
		for path, allowed := range paths {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1"+path+"/x", nil)
			req.Header.Set("X-Test-Verified-Role", role)
			engine.ServeHTTP(rec, req)
			wantCode := http.StatusForbidden
			if allowed {
				wantCode = http.StatusOK
			}
			if rec.Code != wantCode {
				t.Fatalf("role=%s path=%s code=%d, want %d", role, path, rec.Code, wantCode)
			}
		}
	}
}

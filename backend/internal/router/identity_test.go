package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"automation-hub-backend/internal/config"
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/rbac"

	"github.com/gin-gonic/gin"
)

func newIdentityEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(identityMiddleware())
	admin := r.Group("/api/v1/admin")
	admin.Use(requirePermission(rbac.PermAdmin))
	admin.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func doJWT(engine *gin.Engine, token string) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/x", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	engine.ServeHTTP(rec, req)
	return rec.Code
}

func TestJWTRoleIsEnforced(t *testing.T) {
	prev := config.AppConfig.JWTSecret
	config.AppConfig.JWTSecret = "router-secret"
	defer func() { config.AppConfig.JWTSecret = prev }()

	engine := newIdentityEngine()
	now := time.Now()
	exp := now.Add(time.Hour).Unix()

	// owner JWT -> reaches the admin route
	owner := identity.SignToken(identity.Claims{Subject: "u1", Role: "owner", Expiry: exp}, "router-secret")
	if code := doJWT(engine, owner); code != http.StatusOK {
		t.Fatalf("owner JWT should reach admin route, got %d", code)
	}

	// viewer JWT -> forbidden on the admin route
	viewer := identity.SignToken(identity.Claims{Subject: "u2", Role: "viewer", Expiry: exp}, "router-secret")
	if code := doJWT(engine, viewer); code != http.StatusForbidden {
		t.Fatalf("viewer JWT should be 403 on admin route, got %d", code)
	}

	// tampered/invalid JWT -> 401
	if code := doJWT(engine, owner+"tampered"); code != http.StatusUnauthorized {
		t.Fatalf("invalid JWT should be 401, got %d", code)
	}

	// no token -> viewer default -> 403 on admin
	if code := doJWT(engine, ""); code != http.StatusForbidden {
		t.Fatalf("no token should default to viewer (403 on admin), got %d", code)
	}
}

func TestJWTDisabledWhenNoSecret(t *testing.T) {
	prev := config.AppConfig.JWTSecret
	config.AppConfig.JWTSecret = "" // identity disabled
	defer func() { config.AppConfig.JWTSecret = prev }()

	engine := newIdentityEngine()
	// Even a would-be owner token is ignored (no secret configured) -> falls back
	// to viewer default -> 403 on the admin route. (No 401 for a present token.)
	tok := identity.SignToken(identity.Claims{Role: "owner"}, "whatever")
	if code := doJWT(engine, tok); code != http.StatusForbidden {
		t.Fatalf("with no secret, identity is disabled and admin default-denies (403), got %d", code)
	}
}

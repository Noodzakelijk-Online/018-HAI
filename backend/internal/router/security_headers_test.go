package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newSecuredEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(securityHeadersMiddleware())
	r.GET("/api/v1/os/overview", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/swagger/index.html", func(c *gin.Context) { c.String(http.StatusOK, "swagger") })
	return r
}

func TestSecurityHeadersSetOnApiResponses(t *testing.T) {
	rec := httptest.NewRecorder()
	newSecuredEngine().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/os/overview", nil))

	want := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"X-XSS-Protection":             "0",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'; base-uri 'none'",
	}
	for header, expected := range want {
		if got := rec.Header().Get(header); got != expected {
			t.Fatalf("%s = %q, want %q", header, got, expected)
		}
	}
}

func TestSwaggerIsExemptFromCSPOnly(t *testing.T) {
	rec := httptest.NewRecorder()
	newSecuredEngine().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))

	if got := rec.Header().Get("Content-Security-Policy"); got != "" {
		t.Fatalf("swagger CSP = %q, want empty (exempt so the UI can load)", got)
	}
	// Non-CSP protections still apply to swagger.
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("swagger X-Content-Type-Options = %q, want nosniff", got)
	}
}

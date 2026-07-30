package domainpack

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestHandlerRequiresOwnerAndRejectsClientIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := mustDomainPackHandler(t)
	router := gin.New()
	router.GET("/catalog", handler.Catalog)
	router.POST("/classify", handler.Classify)

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/catalog", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing owner status = %d", missing.Code)
	}

	withOwner := gin.New()
	withOwner.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Next()
	})
	withOwner.POST("/classify", handler.Classify)
	spoofed := httptest.NewRecorder()
	withOwner.ServeHTTP(spoofed, httptest.NewRequest(
		http.MethodPost,
		"/classify",
		strings.NewReader(`{"text":"client deliverable","ownerIdentity":"bob"}`),
	))
	if spoofed.Code != http.StatusBadRequest {
		t.Fatalf("client identity status = %d, body=%s", spoofed.Code, spoofed.Body.String())
	}
}

func TestHandlerOwnerIsolationAndEffectiveRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := mustDomainPackHandler(t)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, c.GetHeader("X-Test-Owner"))
		c.Next()
	})
	router.PUT("/preferences/:id", handler.UpsertPreference)
	router.GET("/preferences", handler.Preferences)
	router.GET("/effective/:id", handler.Effective)
	router.POST("/classify", handler.Classify)

	aliceUpdate := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/preferences/work_venture", strings.NewReader(
		`{"status":"active","enabled":false,"classificationBoost":-10,"forceLocalOnly":true,"adaptation":{"notes":"alice"}}`,
	))
	request.Header.Set("X-Test-Owner", "alice")
	router.ServeHTTP(aliceUpdate, request)
	if aliceUpdate.Code != http.StatusOK {
		t.Fatalf("alice update status = %d, body=%s", aliceUpdate.Code, aliceUpdate.Body.String())
	}

	for owner, wantCount := range map[string]string{"alice": `"preferences":[{`, "bob": `"preferences":[]`} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/preferences", nil)
		req.Header.Set("X-Test-Owner", owner)
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), wantCount) {
			t.Fatalf("%s preferences = %d %s", owner, recorder.Code, recorder.Body.String())
		}
	}

	aliceClassify := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/classify", strings.NewReader(
		`{"text":"Review this github repository and client deliverable."}`,
	))
	req.Header.Set("X-Test-Owner", "alice")
	router.ServeHTTP(aliceClassify, req)
	if aliceClassify.Code != http.StatusOK ||
		!strings.Contains(aliceClassify.Body.String(), `"disabled by owner-scoped preference"`) {
		t.Fatalf("alice classification = %d %s", aliceClassify.Code, aliceClassify.Body.String())
	}

	bobEffective := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/effective/work_venture", nil)
	req.Header.Set("X-Test-Owner", "bob")
	router.ServeHTTP(bobEffective, req)
	if bobEffective.Code != http.StatusOK ||
		!strings.Contains(bobEffective.Body.String(), `"enabled":true`) ||
		strings.Contains(bobEffective.Body.String(), `"alice"`) {
		t.Fatalf("bob effective = %d %s", bobEffective.Code, bobEffective.Body.String())
	}
}

func mustDomainPackHandler(t *testing.T) *Handler {
	t.Helper()
	registry := mustBuiltinRegistry(t)
	handler, err := NewHandler(registry, NewMemoryPreferenceRepository(nil))
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return handler
}

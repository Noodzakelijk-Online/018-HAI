package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/lifeontology"

	"github.com/gin-gonic/gin"
)

func TestLifeOntologyRoutesEnforceOwnerRoleAndPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(testIdentityMiddleware())
	service := lifeontology.NewService(lifeontology.NewMemoryRepository(), nil)
	if err := initializeLifeOntologyRoutes(engine.Group("/api/v1"), lifeontology.NewHandler(service)); err != nil {
		t.Fatalf("initialize routes: %v", err)
	}

	for _, test := range []struct {
		name   string
		method string
		role   string
		body   string
		want   int
	}{
		{name: "viewer may inspect", method: http.MethodGet, role: "viewer", want: http.StatusOK},
		{name: "unknown role fails closed", method: http.MethodGet, role: "future-role", want: http.StatusForbidden},
		{name: "viewer cannot write", method: http.MethodPost, role: "viewer", body: `{}`, want: http.StatusForbidden},
		{name: "operator reaches strict validation", method: http.MethodPost, role: "operator", body: `{}`, want: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, "/api/v1/life-ontology/entities", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Test-Verified-Role", test.role)
			engine.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestLifeOntologyRouteRegistrationRejectsMissingHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if err := initializeLifeOntologyRoutes(gin.New().Group("/api/v1"), nil); err == nil {
		t.Fatal("expected nil handler registration failure")
	}
}

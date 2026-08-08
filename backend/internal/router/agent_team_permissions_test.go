package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/frameworkregistry"

	"github.com/gin-gonic/gin"
)

func TestAgentTeamRoutesRequireKnownRolesAndOwnerGovernance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(testIdentityMiddleware())
	err := initializeAgentTeamRoutes(
		engine.Group("/api/v1"),
		frameworkregistry.NewAgentTeamHandler(
			frameworkregistry.NewAgentTeamService(frameworkregistry.NewMemoryAgentTeamRepository()),
		),
	)
	if err != nil {
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
		{name: "operator cannot govern teams", method: http.MethodPost, role: "operator", body: `{}`, want: http.StatusForbidden},
		{name: "owner reaches strict request validation", method: http.MethodPost, role: "owner", body: `{}`, want: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, "/api/v1/framework-registry/teams", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Test-Verified-Role", test.role)
			engine.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestAgentTeamRouteRegistrationRejectsMissingSecurityComposition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if err := initializeAgentTeamRoutes(engine.Group("/api/v1"), nil); err == nil {
		t.Fatal("expected nil handler registration failure")
	}
}

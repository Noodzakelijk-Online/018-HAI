package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/knowledgegraph"

	"github.com/gin-gonic/gin"
)

func TestKnowledgeClaimRoutesEnforceOwnerRoleAndPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(testIdentityMiddleware())
	service := knowledgegraph.NewService(knowledgegraph.NewMemoryRepository(), nil)
	if err := initializeKnowledgeClaimRoutes(engine.Group("/api/v1"), knowledgegraph.NewClaimHandler(service)); err != nil {
		t.Fatalf("initialize routes: %v", err)
	}

	for _, test := range []struct {
		name, method, path, role, body string
		want                           int
	}{
		{name: "viewer may inspect", method: http.MethodGet, path: "/api/v1/knowledge/claims?workspaceId=hai", role: "viewer", want: http.StatusOK},
		{name: "unknown role fails closed", method: http.MethodGet, path: "/api/v1/knowledge/claims?workspaceId=hai", role: "future-role", want: http.StatusForbidden},
		{name: "viewer cannot write", method: http.MethodPost, path: "/api/v1/knowledge/claims?workspaceId=hai", role: "viewer", body: `{}`, want: http.StatusForbidden},
		{name: "operator reaches strict validation", method: http.MethodPost, path: "/api/v1/knowledge/claims?workspaceId=hai", role: "operator", body: `{}`, want: http.StatusBadRequest},
		{name: "operator cannot approve correction", method: http.MethodPost, path: "/api/v1/knowledge/claims/missing/corrections", role: "operator", body: `{}`, want: http.StatusForbidden},
		{name: "owner reaches correction lookup", method: http.MethodPost, path: "/api/v1/knowledge/claims/missing/corrections", role: "owner", body: `{"workspaceId":"hai","requestId":"test","correctedObject":"new","reason":"confirmed"}`, want: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Test-Verified-Role", test.role)
			engine.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestKnowledgeClaimRouteRegistrationRejectsMissingHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if err := initializeKnowledgeClaimRoutes(gin.New().Group("/api/v1"), nil); err == nil {
		t.Fatal("expected nil handler registration failure")
	}
}

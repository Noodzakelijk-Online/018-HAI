package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/pursuit"
	"automation-hub-backend/internal/workflow"

	"github.com/gin-gonic/gin"
)

func TestWorkflowAndPursuitDecisionsRequireOwnerApproval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(testIdentityMiddleware())
	api := engine.Group("/api/v1")
	initializeWorkflowRoutes(api, workflow.NewHandler(nil))
	initializePursuitRoutes(api, pursuit.NewHandler(nil))

	decisionPaths := []string{
		"/api/v1/workflow/not-a-uuid/approval",
		"/api/v1/workflow/not-a-uuid/interruption/resolve",
		"/api/v1/workflow/not-a-uuid/proposals/not-a-uuid/resolve",
		"/api/v1/pursuits/not-a-uuid/decisions/resolve",
		"/api/v1/pursuits/not-a-uuid/candidate/accept",
	}
	for _, path := range decisionPaths {
		t.Run(path, func(t *testing.T) {
			for _, role := range []string{"viewer", "operator", "unknown"} {
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, path, nil)
				request.Header.Set("X-Test-Verified-Role", role)
				engine.ServeHTTP(recorder, request)
				if recorder.Code != http.StatusForbidden {
					t.Errorf("role %s status = %d, want 403: %s", role, recorder.Code, recorder.Body.String())
				}
			}

			owner := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, path, nil)
			request.Header.Set("X-Test-Verified-Role", "owner")
			engine.ServeHTTP(owner, request)
			if owner.Code != http.StatusBadRequest {
				t.Errorf("owner status = %d, want handler validation 400: %s", owner.Code, owner.Body.String())
			}
		})
	}
}

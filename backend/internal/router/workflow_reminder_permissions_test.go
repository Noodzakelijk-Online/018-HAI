package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/workflow"

	"github.com/gin-gonic/gin"
)

func TestWorkflowReminderProposalsRequireAuthenticatedReadPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(testIdentityMiddleware())
	api := engine.Group("/api/v1")
	initializeWorkflowRoutes(api, workflow.NewHandler(nil))

	for _, test := range []struct {
		role string
		want int
	}{
		{role: "viewer", want: http.StatusServiceUnavailable},
		{role: "operator", want: http.StatusServiceUnavailable},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/workflow/reminder-proposals", nil)
		request.Header.Set("X-Test-Verified-Role", test.role)
		engine.ServeHTTP(recorder, request)
		if recorder.Code != test.want {
			t.Fatalf("role %s status = %d, want %d: %s", test.role, recorder.Code, test.want, recorder.Body.String())
		}
	}
}

package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/plangraph"

	"github.com/gin-gonic/gin"
)

func TestPlanGraphRoutesEnforceReadWriteAndApprovePermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(testIdentityMiddleware())
	service := plangraph.NewService(plangraph.NewMemoryRepository(), nil)
	initializePlanGraphRoutes(engine.Group("/api/v1"), plangraph.NewHandler(service))

	for _, test := range []struct {
		name   string
		method string
		path   string
		role   string
		body   string
		want   int
	}{
		{name: "viewer lists plans", method: http.MethodGet, path: "/api/v1/plans", role: "viewer", want: http.StatusOK},
		{name: "viewer cannot create preview", method: http.MethodPost, path: "/api/v1/plans/preview", role: "viewer", body: `{}`, want: http.StatusForbidden},
		{name: "operator reaches preview validation", method: http.MethodPost, path: "/api/v1/plans/preview", role: "operator", body: `{}`, want: http.StatusBadRequest},
		{name: "operator cannot accept", method: http.MethodPost, path: "/api/v1/plans/11111111-1111-4111-8111-111111111111/accept", role: "operator", body: `{}`, want: http.StatusForbidden},
		{name: "owner reaches accept lookup", method: http.MethodPost, path: "/api/v1/plans/11111111-1111-4111-8111-111111111111/accept", role: "owner", body: `{"expectedRevision":1,"expectedDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, want: http.StatusNotFound},
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

package router

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"automation-hub-backend/internal/agentregistry"

	"github.com/gin-gonic/gin"
)

func TestAgentRegistryPermissionsKeepConfigurationAndAssignmentOwnerControlled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, err := agentregistry.NewService(
		agentregistry.NewMemoryRepository(),
		func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("new agent registry: %v", err)
	}
	engine := gin.New()
	engine.Use(testIdentityMiddleware())
	initializeAgentRegistryRoutes(engine.Group("/api/v1"), agentregistry.NewHandler(service))

	checkedAt := "2026-07-30T12:00:00Z"
	registerBody := fmt.Sprintf(`{
		"id":"planner-local",
		"name":"Local planner",
		"type":"planner",
		"runtime":{"id":"hermes","type":"claw","protocolVersion":"1.0.0"},
		"capabilities":[{"id":"planner","version":"1.0.0","operations":["plan"]}],
		"authorityCeiling":4,
		"autonomyCeiling":4,
		"toolAllowlist":["memory.retrieve"],
		"dataAllowlist":["project:hai"],
		"folderAllowlist":[],
		"health":{"status":"healthy","ready":true,"checkedAt":%q,"freshFor":3600000000000},
		"availability":{"available":true,"activeAssignments":0,"maxConcurrent":1},
		"performance":{"estimatedCostEur":0,"p95LatencyMs":1000,"locality":"local"}
	}`, checkedAt)
	assignBody := `{
		"taskId":"task-1",
		"capabilities":[{"id":"planner","minVersion":"1.0.0","maxVersion":"1.9.0","operations":["plan"]}],
		"compatibility":{"runtimeType":"claw","minProtocolVersion":"1.0.0","maxProtocolVersion":"1.9.0"},
		"requiredAuthority":2,
		"requiredAutonomy":2,
		"policyMaxAuthority":4,
		"policyMaxAutonomy":4,
		"requiredTools":["memory.retrieve"],
		"requiredData":["project:hai"],
		"requireLocal":true
	}`

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		role   string
		want   int
	}{
		{name: "viewer may inspect", method: http.MethodGet, path: "/api/v1/agents", role: "viewer", want: http.StatusOK},
		{name: "viewer cannot register", method: http.MethodPost, path: "/api/v1/agents", body: registerBody, role: "viewer", want: http.StatusForbidden},
		{name: "operator cannot register", method: http.MethodPost, path: "/api/v1/agents", body: registerBody, role: "operator", want: http.StatusForbidden},
		{name: "owner registers", method: http.MethodPost, path: "/api/v1/agents", body: registerBody, role: "owner", want: http.StatusCreated},
		{name: "operator cannot delegate authority", method: http.MethodPost, path: "/api/v1/agents/assignments", body: assignBody, role: "operator", want: http.StatusForbidden},
		{
			name:   "operator cannot attest assignment outcome",
			method: http.MethodPost,
			path:   "/api/v1/agents/assignments/nonexistent/outcome",
			body:   `{"expectedRevision":1,"success":true,"latency":0}`,
			role:   "operator",
			want:   http.StatusForbidden,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Test-Verified-Role", test.role)
			engine.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

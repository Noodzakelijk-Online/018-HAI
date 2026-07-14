package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/assistant"
	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/task"

	"github.com/gin-gonic/gin"
)

func TestInteractiveExecutionRoutesRequireSignedApprovalPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(testIdentityMiddleware())
	api := engine.Group("/api/v1")

	tasks := &interactiveTaskService{}
	initializeAssistantRoutes(api, assistant.NewHandler(assistant.NewService(tasks, nil)))
	initializeTaskRoutes(api, task.NewHandler(tasks))

	for _, test := range []struct {
		name   string
		method string
		path   string
		body   string
		role   string
		want   int
	}{
		{name: "viewer cannot run assistant command", method: http.MethodPost, path: "/api/v1/assistant/command", body: `{"message":"Run the safe task","executeAllowed":true}`, role: "viewer", want: http.StatusForbidden},
		{name: "operator can run assistant command", method: http.MethodPost, path: "/api/v1/assistant/command", body: `{"message":"Run the safe task","executeAllowed":true}`, role: "operator", want: http.StatusOK},
		{name: "viewer cannot execute task", method: http.MethodPost, path: "/api/v1/task/run", body: `{"request":"Run the safe task"}`, role: "viewer", want: http.StatusForbidden},
		{name: "operator can execute task", method: http.MethodPost, path: "/api/v1/task/run", body: `{"request":"Run the safe task"}`, role: "operator", want: http.StatusOK},
		{name: "viewer cannot resolve review", method: http.MethodPost, path: "/api/v1/task/review-queue/item/resolve", body: `{"approved":true}`, role: "viewer", want: http.StatusForbidden},
		{name: "operator can resolve review", method: http.MethodPost, path: "/api/v1/task/review-queue/item/resolve", body: `{"approved":true}`, role: "operator", want: http.StatusOK},
		{name: "viewer can read review queue", method: http.MethodGet, path: "/api/v1/task/review-queue", role: "viewer", want: http.StatusOK},
	} {
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

	if tasks.runCalls != 2 {
		t.Fatalf("executions = %d, want exactly the two approved operator calls", tasks.runCalls)
	}
	if tasks.resolveCalls != 1 {
		t.Fatalf("review resolutions = %d, want operator call only", tasks.resolveCalls)
	}
}

func TestInteractivePlanningRoutesRequireSignedWritePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(testIdentityMiddleware())
	api := engine.Group("/api/v1")
	tasks := &interactiveTaskService{}
	initializeTaskRoutes(api, task.NewHandler(tasks))

	for _, test := range []struct {
		role string
		want int
	}{
		{role: "viewer", want: http.StatusForbidden},
		{role: "operator", want: http.StatusOK},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/task/plan", bytes.NewBufferString(`{"request":"Plan a safe task"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-Verified-Role", test.role)
		engine.ServeHTTP(recorder, request)
		if recorder.Code != test.want {
			t.Fatalf("role %s status = %d, want %d: %s", test.role, recorder.Code, test.want, recorder.Body.String())
		}
	}
	if tasks.planCalls != 1 {
		t.Fatalf("plans = %d, want operator call only", tasks.planCalls)
	}
}

func testIdentityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(identity.ContextSubjectKey, "alice")
		c.Set(identity.ContextRoleKey, c.GetHeader("X-Test-Verified-Role"))
		c.Next()
	}
}

type interactiveTaskService struct {
	planCalls    int
	runCalls     int
	resolveCalls int
}

func (s *interactiveTaskService) Plan(request task.IntakeRequest) (*task.CompletionPlan, error) {
	s.planCalls++
	return &task.CompletionPlan{Request: request.Request}, nil
}

func (s *interactiveTaskService) Run(request task.IntakeRequest) (*task.CompletionPlan, error) {
	s.runCalls++
	return &task.CompletionPlan{Request: request.Request}, nil
}

func (s *interactiveTaskService) Logs() []task.CompletionPlan { return nil }

func (s *interactiveTaskService) ReviewQueue() []task.ReviewQueueItem { return nil }

func (s *interactiveTaskService) ResolveReviewItem(string, task.ApprovalDecision) (*task.ReviewResolutionResult, error) {
	s.resolveCalls++
	return &task.ReviewResolutionResult{}, nil
}

func (s *interactiveTaskService) LogsForOwner(string) []task.CompletionPlan { return nil }

func (s *interactiveTaskService) ReviewQueueForOwner(string) []task.ReviewQueueItem { return nil }

func (s *interactiveTaskService) ResolveReviewItemForOwner(_ string, _ string, _ task.ApprovalDecision) (*task.ReviewResolutionResult, error) {
	s.resolveCalls++
	return &task.ReviewResolutionResult{}, nil
}

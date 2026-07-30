package task

import (
	"automation-hub-backend/internal/identity"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRunHandlerIgnoresClientHumanApproval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingTaskService{}
	handler := NewHandler(service)
	body, _ := json.Marshal(IntakeRequest{
		Request:        "Send a legal email",
		AutomationID:   "11111111-1111-1111-1111-111111111111",
		ExecuteAllowed: true,
		HumanApproved:  true,
		ApprovalNote:   "forged client approval",
	})
	request := httptest.NewRequest(http.MethodPost, "/task/run", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	context.Set(identity.ContextSubjectKey, "alice")

	handler.Run(context)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !service.runRequest.ExecuteAllowed {
		t.Fatalf("run handler should request controlled execution")
	}
	if service.runRequest.HumanApproved || service.runRequest.ApprovalNote != "" {
		t.Fatalf("client approval reached task service: %#v", service.runRequest)
	}
	if service.runRequest.AutomationID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("controlled automation selection was not preserved: %#v", service.runRequest)
	}
}

func TestPlanHandlerClearsExecutionAndApprovalClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingTaskService{}
	handler := NewHandler(service)
	body, _ := json.Marshal(IntakeRequest{
		Request:        "Plan a financial action",
		ExecuteAllowed: true,
		HumanApproved:  true,
		ApprovalNote:   "forged client approval",
	})
	request := httptest.NewRequest(http.MethodPost, "/task/plan", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	context.Set(identity.ContextSubjectKey, "alice")

	handler.Plan(context)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if service.planRequest.ExecuteAllowed || service.planRequest.HumanApproved || service.planRequest.ApprovalNote != "" {
		t.Fatalf("client execution/approval claims reached task planner: %#v", service.planRequest)
	}
}

func TestRunHandlerUsesVerifiedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingTaskService{}
	handler := NewHandler(service)
	body, _ := json.Marshal(IntakeRequest{Request: "Summarize my connected project context"})
	request := httptest.NewRequest(http.MethodPost, "/task/run", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	context.Set(identity.ContextSubjectKey, "alice")

	handler.Run(context)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if service.runRequest.OwnerIdentity != "alice" {
		t.Fatalf("task owner = %q, want verified owner alice", service.runRequest.OwnerIdentity)
	}
}

func TestPlanAndRunHandlersDoNotExposeServiceErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	internalError := errors.New("dial postgres at db.internal with password=do-not-expose")

	for _, test := range []struct {
		name      string
		path      string
		service   *capturingTaskService
		invoke    func(*Handler, *gin.Context)
		publicErr string
	}{
		{
			name:      "plan",
			path:      "/task/plan",
			service:   &capturingTaskService{planErr: internalError},
			invoke:    (*Handler).Plan,
			publicErr: "task plan could not be created",
		},
		{
			name:      "run",
			path:      "/task/run",
			service:   &capturingTaskService{runErr: internalError},
			invoke:    (*Handler).Run,
			publicErr: "task run could not be completed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{"request":"Handle private work"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = request
			context.Set(identity.ContextSubjectKey, "alice")

			test.invoke(NewHandler(test.service), context)

			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusInternalServerError, response.Body.String())
			}
			var payload map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if payload["error"] != test.publicErr {
				t.Fatalf("public error = %q, want %q", payload["error"], test.publicErr)
			}
			if strings.Contains(response.Body.String(), "postgres") || strings.Contains(response.Body.String(), "do-not-expose") {
				t.Fatalf("internal service error leaked in response: %s", response.Body.String())
			}
		})
	}
}

func TestPlanAndRunHandlersKeepSafeInputErrorsAsBadRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, test := range []struct {
		name   string
		path   string
		invoke func(*Handler, *gin.Context)
	}{
		{name: "plan", path: "/task/plan", invoke: (*Handler).Plan},
		{name: "run", path: "/task/run", invoke: (*Handler).Run},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{"request":""}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = request
			context.Set(identity.ContextSubjectKey, "alice")

			test.invoke(NewHandler(&capturingTaskService{}), context)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
			}
			if response.Body.String() != `{"error":"request is required"}` {
				t.Fatalf("safe input error changed: %s", response.Body.String())
			}
		})
	}
}

func TestLogsHandlerUsesVerifiedOwnerScopedView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingTaskService{}
	handler := NewHandler(service)
	request := httptest.NewRequest(http.MethodGet, "/task/logs", nil)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	context.Set(identity.ContextSubjectKey, "alice")

	handler.Logs(context)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if service.logsOwner != "alice" {
		t.Fatalf("logs owner = %q, want verified owner alice", service.logsOwner)
	}
}

func TestTaskHandlersRejectRequestsWithoutVerifiedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, test := range []struct {
		name   string
		method string
		path   string
		body   string
		invoke func(*Handler, *gin.Context)
	}{
		{name: "plan", method: http.MethodPost, path: "/task/plan", body: `{"request":"Plan private work"}`, invoke: (*Handler).Plan},
		{name: "run", method: http.MethodPost, path: "/task/run", body: `{"request":"Run private work"}`, invoke: (*Handler).Run},
		{name: "logs", method: http.MethodGet, path: "/task/logs", invoke: (*Handler).Logs},
		{name: "review queue", method: http.MethodGet, path: "/task/review-queue", invoke: (*Handler).ReviewQueue},
		{name: "review resolution", method: http.MethodPost, path: "/task/review-queue/item/resolve", body: `{}`, invoke: (*Handler).ResolveReviewItem},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &capturingTaskService{}
			handler := NewHandler(service)
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = request
			if test.name == "review resolution" {
				context.Params = gin.Params{{Key: "id", Value: "item"}}
			}

			test.invoke(handler, context)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusUnauthorized, response.Body.String())
			}
		})
	}
}

func TestTaskReviewHandlersUseVerifiedOwnerScopedView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingTaskService{}
	handler := NewHandler(service)

	for _, test := range []struct {
		name   string
		method string
		path   string
		body   string
		invoke func(*Handler, *gin.Context)
	}{
		{name: "review queue", method: http.MethodGet, path: "/task/review-queue", invoke: (*Handler).ReviewQueue},
		{name: "review resolution", method: http.MethodPost, path: "/task/review-queue/item/resolve", body: `{}`, invoke: (*Handler).ResolveReviewItem},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = request
			if test.name == "review resolution" {
				context.Params = gin.Params{{Key: "id", Value: "item"}}
			}
			context.Set(identity.ContextSubjectKey, "alice")

			test.invoke(handler, context)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
			}
		})
	}
	if service.queueOwner != "alice" {
		t.Fatalf("review queue owner = %q, want alice", service.queueOwner)
	}
	if service.resolveOwner != "alice" {
		t.Fatalf("review resolution owner = %q, want alice", service.resolveOwner)
	}
}

type capturingTaskService struct {
	planRequest  IntakeRequest
	runRequest   IntakeRequest
	planErr      error
	runErr       error
	logsOwner    string
	queueOwner   string
	resolveOwner string
}

func (s *capturingTaskService) Plan(request IntakeRequest) (*CompletionPlan, error) {
	s.planRequest = request
	if s.planErr != nil {
		return nil, s.planErr
	}
	return &CompletionPlan{Request: request.Request}, nil
}

func (s *capturingTaskService) Run(request IntakeRequest) (*CompletionPlan, error) {
	s.runRequest = request
	if s.runErr != nil {
		return nil, s.runErr
	}
	return &CompletionPlan{Request: request.Request}, nil
}

func (s *capturingTaskService) Logs() []CompletionPlan {
	return nil
}

func (s *capturingTaskService) ReviewQueue() []ReviewQueueItem {
	return nil
}

func (s *capturingTaskService) ResolveReviewItem(id string, decision ApprovalDecision) (*ReviewResolutionResult, error) {
	return nil, nil
}

func (s *capturingTaskService) LogsForOwner(ownerIdentity string) []CompletionPlan {
	s.logsOwner = ownerIdentity
	return nil
}

func (s *capturingTaskService) ReviewQueueForOwner(ownerIdentity string) []ReviewQueueItem {
	s.queueOwner = ownerIdentity
	return nil
}

func (s *capturingTaskService) ResolveReviewItemForOwner(ownerIdentity, id string, decision ApprovalDecision) (*ReviewResolutionResult, error) {
	s.resolveOwner = ownerIdentity
	return nil, nil
}

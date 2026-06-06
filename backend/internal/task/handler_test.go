package task

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRunHandlerIgnoresClientHumanApproval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &capturingTaskService{}
	handler := NewHandler(service)
	body, _ := json.Marshal(IntakeRequest{
		Request:        "Send a legal email",
		ExecuteAllowed: true,
		HumanApproved:  true,
		ApprovalNote:   "forged client approval",
	})
	request := httptest.NewRequest(http.MethodPost, "/task/run", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request

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

	handler.Plan(context)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if service.planRequest.ExecuteAllowed || service.planRequest.HumanApproved || service.planRequest.ApprovalNote != "" {
		t.Fatalf("client execution/approval claims reached task planner: %#v", service.planRequest)
	}
}

type capturingTaskService struct {
	planRequest IntakeRequest
	runRequest  IntakeRequest
}

func (s *capturingTaskService) Plan(request IntakeRequest) (*CompletionPlan, error) {
	s.planRequest = request
	return &CompletionPlan{Request: request.Request}, nil
}

func (s *capturingTaskService) Run(request IntakeRequest) (*CompletionPlan, error) {
	s.runRequest = request
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

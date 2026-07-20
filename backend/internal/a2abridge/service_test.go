package a2abridge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"automation-hub-backend/internal/task"

	"github.com/gin-gonic/gin"
)

const testBridgeToken = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type previewStub struct {
	requests []task.IntakeRequest
}

func (s *previewStub) Preview(request task.IntakeRequest) (*task.CompletionPlan, error) {
	s.requests = append(s.requests, request)
	return &task.CompletionPlan{
		Intake:           task.IntakeAnalysis{TaskType: "research", SuccessCriteria: []string{"produce a source-backed draft"}},
		RiskAssessment:   task.RiskAssessment{Level: "high", ApprovalRequired: true},
		ValidationResult: task.ValidationResult{NextAction: "ask Robert to review the draft"},
		CompletionStatus: "review_required",
		Steps:            []task.TaskStep{{Name: "Gather evidence", Purpose: "find relevant records", RequiresApproval: false, Status: "ready"}},
	}, nil
}

func configuredService(planner task.PreviewService) *Service {
	return NewService(Config{Enabled: true, Token: testBridgeToken, OwnerID: "owner@example.test", URL: "http://127.0.0.1/api/v1/a2a"}, planner)
}

func TestDraftCreatesSanitizedPreviewWithoutExecution(t *testing.T) {
	planner := &previewStub{}
	service := configuredService(planner)
	proposal, err := service.Draft("Prepare a legal draft")
	if err != nil {
		t.Fatalf("Draft returned error: %v", err)
	}
	if len(planner.requests) != 1 || planner.requests[0].OwnerIdentity != "owner@example.test" || planner.requests[0].ExecuteAllowed || planner.requests[0].HumanApproved {
		t.Fatalf("preview request was not bounded: %#v", planner.requests)
	}
	if proposal.NeedsApproval != true || proposal.RiskLevel != "high" || len(proposal.Steps) != 1 {
		t.Fatalf("proposal = %#v", proposal)
	}
	if strings.Contains(strings.ToLower(proposal.Scope), "execute") == false || strings.Contains(strings.ToLower(proposal.Scope), "did not create") == false {
		t.Fatalf("proposal scope is not explicit: %q", proposal.Scope)
	}
}

func TestStatusRejectsExternalEndpointAndWrongToken(t *testing.T) {
	service := NewService(Config{Enabled: true, Token: testBridgeToken, OwnerID: "owner", URL: "https://example.com/a2a"}, &previewStub{})
	if service.Status().Configured {
		t.Fatal("external endpoint must not configure bridge")
	}
	if configuredService(&previewStub{}).Authorize("wrong") {
		t.Fatal("wrong token authorized")
	}
}

func TestHandlerProvidesCardAndTokenBoundedTaskSend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(configuredService(&previewStub{}))
	router := gin.New()
	router.GET("/.well-known/agent-card.json", handler.AgentCard)
	router.POST("/api/v1/a2a", handler.Send)

	card := httptest.NewRecorder()
	router.ServeHTTP(card, httptest.NewRequest(http.MethodGet, "/.well-known/agent-card.json", nil))
	if card.Code != http.StatusOK || !strings.Contains(card.Body.String(), "hai_controlled_planning") || !strings.Contains(card.Body.String(), "http://127.0.0.1/api/v1/a2a") {
		t.Fatalf("agent card = %d %s", card.Code, card.Body.String())
	}

	body := `{"jsonrpc":"2.0","id":1,"method":"tasks/send","params":{"message":{"role":"user","parts":[{"kind":"text","text":"Plan a source-backed response"}]}}}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/a2a", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testBridgeToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "hai-controlled-planning-proposal") || strings.Contains(response.Body.String(), "sourceContext") {
		t.Fatalf("task response = %d %s", response.Code, response.Body.String())
	}

	denied := httptest.NewRecorder()
	deniedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/a2a", strings.NewReader(body))
	deniedRequest.Header.Set("Authorization", "Bearer not-the-token")
	router.ServeHTTP(denied, deniedRequest)
	if denied.Code != http.StatusNotFound {
		t.Fatalf("invalid token status = %d", denied.Code)
	}
}

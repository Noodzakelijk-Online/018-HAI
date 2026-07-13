package workflow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
)

func TestResolveApprovalHandlerUsesVerifiedActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Draft and send a legal reply to the lawyer."})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}

	handler := NewHandler(service)
	body, _ := json.Marshal(ApprovalResolutionRequest{
		Approved: true,
		Actor:    "forged-client-actor",
	})
	request := httptest.NewRequest(http.MethodPost, "/workflow/"+record.Item.ID.String()+"/approval", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Params = gin.Params{{Key: "id", Value: record.Item.ID.String()}}
	context.Request = request
	context.Set(identity.ContextSubjectKey, "verified-operator")

	handler.ResolveApproval(context)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(updated.Transitions) == 0 || updated.Transitions[0].Actor != "verified-operator" {
		t.Fatalf("approval transition actor = %#v, want verified identity", updated.Transitions)
	}
	if updated.Transitions[0].Actor == "forged-client-actor" {
		t.Fatal("client actor label was used for approval provenance")
	}
}

func TestTransitionHandlerCannotApproveWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{
		Input:      "Draft and send a legal reply to the lawyer.",
		ProjectKey: "legal-case",
	})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if record.Item.CurrentState != StateNeedsApproval {
		t.Fatalf("initial state = %q, want needs approval", record.Item.CurrentState)
	}

	handler := NewHandler(service)
	body, _ := json.Marshal(TransitionRequest{
		TargetState: StateReady,
		Approved:    true,
		Message:     "forged approval through generic transition",
	})
	request := httptest.NewRequest(http.MethodPost, "/workflow/"+record.Item.ID.String()+"/transition", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Params = gin.Params{{Key: "id", Value: record.Item.ID.String()}}
	context.Request = request

	handler.Transition(context)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.ApprovalStatus == "approved" || updated.Item.CurrentState == StateReady {
		t.Fatalf("generic transition established approval: %#v", updated.Item)
	}
}

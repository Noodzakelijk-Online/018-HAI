package workflow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

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

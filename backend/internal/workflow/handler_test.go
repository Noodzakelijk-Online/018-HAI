package workflow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"automation-hub-backend/internal/identity"
	"automation-hub-backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

func TestWorkflowHandlerRejectsCrossOwnerReadAndApproval(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{
		OwnerIdentity: "bob",
		Input:         "Draft and send a legal reply to Bob's lawyer.",
	})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	handler := NewHandler(service)

	getRequest := httptest.NewRequest(http.MethodGet, "/workflow/"+record.Item.ID.String(), nil)
	getResponse := httptest.NewRecorder()
	getContext, _ := gin.CreateTestContext(getResponse)
	getContext.Params = gin.Params{{Key: "id", Value: record.Item.ID.String()}}
	getContext.Request = getRequest
	getContext.Set(identity.ContextSubjectKey, "alice")
	handler.Get(getContext)
	if getResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-owner get status = %d, want 404: %s", getResponse.Code, getResponse.Body.String())
	}

	body, _ := json.Marshal(ApprovalResolutionRequest{Approved: true})
	approvalRequest := httptest.NewRequest(http.MethodPost, "/workflow/"+record.Item.ID.String()+"/approval", bytes.NewReader(body))
	approvalRequest.Header.Set("Content-Type", "application/json")
	approvalResponse := httptest.NewRecorder()
	approvalContext, _ := gin.CreateTestContext(approvalResponse)
	approvalContext.Params = gin.Params{{Key: "id", Value: record.Item.ID.String()}}
	approvalContext.Request = approvalRequest
	approvalContext.Set(identity.ContextSubjectKey, "alice")
	handler.ResolveApproval(approvalContext)
	if approvalResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-owner approval status = %d, want 404: %s", approvalResponse.Code, approvalResponse.Body.String())
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.ApprovalStatus == "approved" {
		t.Fatalf("cross-owner approval mutated workflow: %#v", updated.Item)
	}
}

func TestRunDueHandlerRunsOnlyVerifiedOwnerWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeWorkflowRepo()
	runner := &fakeTaskRunner{result: &TaskRunResult{PlanID: "handler-owner-plan", CompletionStatus: "validated", VerificationStatus: "verified", Passed: true}}
	service := NewServiceWithTaskRunner(repo, runner)
	if _, err := service.Intake(IntakeRequest{OwnerIdentity: "alice", Input: "Create Alice's low-risk admin checklist."}); err != nil {
		t.Fatalf("alice Intake: %v", err)
	}
	bob, err := service.Intake(IntakeRequest{OwnerIdentity: "bob", Input: "Create Bob's low-risk admin checklist."})
	if err != nil {
		t.Fatalf("bob Intake: %v", err)
	}
	handler := NewHandler(service)
	request := httptest.NewRequest(http.MethodPost, "/workflow/run-due", bytes.NewBufferString(`{"limit":5}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	context.Set(identity.ContextSubjectKey, "alice")

	handler.RunDue(context)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if len(runner.requests) != 1 || runner.requests[0].OwnerIdentity != "alice" {
		t.Fatalf("handler executed wrong workflows: %#v", runner.requests)
	}
	if repo.items[bob.Item.ID].CurrentState != StateReady {
		t.Fatalf("handler executed Bob workflow for Alice: %#v", repo.items[bob.Item.ID])
	}
}

func TestIntakeHandlerRoutesLegacyRequestThroughPursuitGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	router := &capturingPursuitIntakeRouter{record: &WorkflowRecord{Item: models.WorkflowItem{ID: uuid.New(), Title: "Governed intake"}}}
	handler := NewHandlerWithPursuitIntakeRouter(service, router)
	body, _ := json.Marshal(IntakeRequest{Input: "Prepare the evidence bundle", ProjectKey: "vivare"})
	request := httptest.NewRequest(http.MethodPost, "/workflow/intake", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	context.Set(identity.ContextSubjectKey, "verified-operator")

	handler.Intake(context)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", response.Code, response.Body.String())
	}
	if router.calls != 1 || router.request.Actor != "verified-operator" {
		t.Fatalf("pursuit intake request = %#v", router.request)
	}
	if router.request.SourceType != "workflow_api" || router.request.SourceID == "" || router.request.SourceURI == "" {
		t.Fatalf("legacy request was not normalized with deterministic provenance: %#v", router.request)
	}
}

type capturingPursuitIntakeRouter struct {
	calls   int
	request IntakeRequest
	record  *WorkflowRecord
}

func (r *capturingPursuitIntakeRouter) RouteWorkflowIntake(request IntakeRequest) (*WorkflowRecord, error) {
	r.calls++
	r.request = request
	return r.record, nil
}

package task

import (
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/automation"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

func TestAutomationToolExecutorMapsControlledLaunchResult(t *testing.T) {
	id := uuid.New()
	launchEventID := uuid.New()
	launchedAt := time.Now().UTC()
	launcher := &fakeAutomationLauncher{
		result: &automation.LaunchResult{
			AutomationID:  id,
			LaunchEventID: launchEventID,
			RuntimeType:   "script",
			LaunchType:    "script",
			Target:        "verify-project.sh",
			Status:        "completed",
			Message:       "script completed",
			Output:        "tests passed",
			RuntimeRouteTrace: &models.AutomationRuntimeRouteTrace{
				RuntimeID:         "openclaw",
				Intent:            "code_review",
				ExecutionMode:     "read_only",
				RiskLevel:         "medium",
				RecommendedSkills: []string{"autoreview", "gitcrawl"},
				BlockedSurfaces:   []string{"external_message_sending"},
			},
			ExitCode:    0,
			DurationMs:  42,
			AuditEvents: []string{"script executed without shell"},
			LaunchedAt:  launchedAt,
		},
	}
	executor := NewAutomationToolExecutor(launcher)
	result, err := executor.Execute(ToolExecutionRequest{OwnerIdentity: "alice", AutomationID: id.String(), Task: "Run tests"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "completed" || result.Output != "tests passed" {
		t.Fatalf("result = %#v, want completed runtime evidence", result)
	}
	if result.AutomationID != id.String() || result.LaunchEventID != launchEventID.String() || !result.ExecutedAt.Equal(launchedAt) {
		t.Fatalf("runtime identity was not preserved: %#v", result)
	}
	if result.RuntimeRouteTrace == nil || result.RuntimeRouteTrace.RuntimeID != "openclaw" || len(result.RuntimeRouteTrace.RecommendedSkills) != 2 {
		t.Fatalf("runtime route trace was not preserved: %#v", result.RuntimeRouteTrace)
	}
	if launcher.request.OwnerIdentity != "alice" {
		t.Fatalf("launch request owner = %q, want alice", launcher.request.OwnerIdentity)
	}
	if launcher.request.ApprovalProof != nil || launcher.issueCalls != 0 {
		t.Fatalf("unapproved request received an approval proof: request=%#v issues=%d", launcher.request, launcher.issueCalls)
	}
}

func TestAutomationToolExecutorIssuesActionBoundProofForRecordedReview(t *testing.T) {
	id := uuid.New()
	sourceID := "task-review:" + uuid.NewString()
	issuedAt := time.Now().UTC()
	proof := &automation.ApprovalProof{
		ID:               "proof-1",
		OwnerIdentity:    "alice",
		AutomationID:     id,
		ActionDigest:     strings.Repeat("a", 64),
		Scope:            automation.ApprovalScopeScript,
		ApprovalSourceID: sourceID,
		IssuedAt:         issuedAt,
		ExpiresAt:        issuedAt.Add(time.Minute),
		Nonce:            "proof-nonce",
		Signature:        "proof-signature",
	}
	launcher := &fakeAutomationLauncher{
		proof: proof,
		result: &automation.LaunchResult{
			AutomationID: id,
			Status:       "completed",
			LaunchedAt:   time.Now().UTC(),
		},
	}
	executor := NewAutomationToolExecutor(launcher)
	result, err := executor.Execute(ToolExecutionRequest{
		OwnerIdentity:    "alice",
		AutomationID:     id.String(),
		Task:             "Run the exact reviewed action.",
		ProjectKey:       "018-hai",
		ApprovalSourceID: sourceID,
		approvalDecision: &automation.TaskApprovalDecisionRequest{
			OwnerIdentity:    "alice",
			Task:             "Run the exact reviewed action.",
			ProjectKey:       "018-hai",
			ApprovalSourceID: sourceID,
			ApprovedAt:       issuedAt.Add(-time.Second),
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || result.Status != "completed" {
		t.Fatalf("result = %#v, want completed", result)
	}
	if launcher.issueCalls != 1 || launcher.issueID != id {
		t.Fatalf("proof issuance = calls %d id %s, want one for %s", launcher.issueCalls, launcher.issueID, id)
	}
	if launcher.issueRequest.OwnerIdentity != "alice" ||
		launcher.issueRequest.Task != "Run the exact reviewed action." ||
		launcher.issueRequest.ProjectKey != "018-hai" ||
		launcher.issueRequest.ApprovalSourceID != sourceID {
		t.Fatalf("proof issue request was not action-bound: %#v", launcher.issueRequest)
	}
	if launcher.recordCalls != 1 || launcher.recordRequest.ApprovalSourceID != sourceID {
		t.Fatalf("approval decision was not registered exactly once: %#v", launcher.recordRequest)
	}
	if launcher.request.ApprovalProof != proof || launcher.request.ApprovalSourceID != sourceID {
		t.Fatalf("launch request did not carry issued proof: %#v", launcher.request)
	}
}

func TestAutomationToolExecutorFailsClosedWhenProofIssuerIsUnavailable(t *testing.T) {
	launcher := &launchOnlyAutomationLauncher{}
	executor := NewAutomationToolExecutor(launcher)
	sourceID := "task-review:" + uuid.NewString()
	_, err := executor.Execute(ToolExecutionRequest{
		OwnerIdentity:    "alice",
		AutomationID:     uuid.NewString(),
		Task:             "Run reviewed action.",
		ApprovalSourceID: sourceID,
		approvalDecision: &automation.TaskApprovalDecisionRequest{
			OwnerIdentity:    "alice",
			Task:             "Run reviewed action.",
			ApprovalSourceID: sourceID,
			ApprovedAt:       time.Now().UTC(),
		},
	})
	if err == nil || launcher.launchCalls != 0 {
		t.Fatalf("missing proof issuer did not fail closed: err=%v launches=%d", err, launcher.launchCalls)
	}
}

func TestAutomationToolExecutorRejectsInvalidAutomationID(t *testing.T) {
	executor := NewAutomationToolExecutor(&fakeAutomationLauncher{})
	if _, err := executor.Execute(ToolExecutionRequest{AutomationID: "not-a-uuid"}); err == nil {
		t.Fatalf("expected invalid automation ID to be rejected")
	}
}

type fakeAutomationLauncher struct {
	result        *automation.LaunchResult
	err           error
	request       automation.TaskLaunchRequest
	proof         *automation.ApprovalProof
	issueErr      error
	issueID       uuid.UUID
	issueRequest  automation.TaskApprovalProofRequest
	issueCalls    int
	recordRequest automation.TaskApprovalDecisionRequest
	recordCalls   int
}

func (f *fakeAutomationLauncher) Launch(id uuid.UUID) (*automation.LaunchResult, error) {
	return f.result, f.err
}

func (f *fakeAutomationLauncher) LaunchTask(id uuid.UUID, request automation.TaskLaunchRequest) (*automation.LaunchResult, error) {
	f.request = request
	return f.result, f.err
}

func (f *fakeAutomationLauncher) IssueApprovalProof(id uuid.UUID, request automation.TaskApprovalProofRequest) (*automation.ApprovalProof, error) {
	f.issueCalls++
	f.issueID = id
	f.issueRequest = request
	return f.proof, f.issueErr
}

func (f *fakeAutomationLauncher) RecordApprovalDecision(id uuid.UUID, request automation.TaskApprovalDecisionRequest) error {
	f.recordCalls++
	f.issueID = id
	f.recordRequest = request
	return nil
}

type launchOnlyAutomationLauncher struct {
	launchCalls int
}

func (f *launchOnlyAutomationLauncher) Launch(id uuid.UUID) (*automation.LaunchResult, error) {
	f.launchCalls++
	return &automation.LaunchResult{AutomationID: id}, nil
}

func (f *launchOnlyAutomationLauncher) LaunchTask(id uuid.UUID, request automation.TaskLaunchRequest) (*automation.LaunchResult, error) {
	f.launchCalls++
	return &automation.LaunchResult{AutomationID: id}, nil
}

func (f *launchOnlyAutomationLauncher) RecordApprovalDecision(uuid.UUID, automation.TaskApprovalDecisionRequest) error {
	return nil
}

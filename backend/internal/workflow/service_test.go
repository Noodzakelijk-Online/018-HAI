package workflow

import (
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestIntakeCreatesApprovalGatedLegalWorkflow(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)

	record, err := service.Intake(IntakeRequest{
		Input:      "Email from lawyer about Vivare legal hearing tomorrow. Draft formal reply.",
		ProjectKey: "Vivare dispute",
		SourceType: "email",
		SourceURI:  "mailto:lawyer@example.test",
		Trigger:    "email.sync",
	})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if record.Item.CurrentState != StateNeedsApproval {
		t.Fatalf("state = %q, want needs approval", record.Item.CurrentState)
	}
	if !record.Item.RequiresApproval {
		t.Fatalf("expected approval requirement")
	}
	if record.Item.PriorityScore < 80 {
		t.Fatalf("priority = %d, want high legal priority", record.Item.PriorityScore)
	}
	if len(record.Checklist) == 0 {
		t.Fatalf("expected generated checklist")
	}
	if !hasApprovalChecklist(record.Checklist) {
		t.Fatalf("expected approval-marked checklist item")
	}
	if len(record.Intake) != 1 {
		t.Fatalf("intake records = %d, want 1", len(record.Intake))
	}
	if len(record.Matches) != 1 {
		t.Fatalf("project matches = %d, want 1", len(record.Matches))
	}
	if len(record.OpenLoops) != 1 {
		t.Fatalf("open loops = %d, want 1 approval loop", len(record.OpenLoops))
	}
	if len(record.Proposals) != 1 {
		t.Fatalf("proposals = %d, want 1", len(record.Proposals))
	}
	if len(record.QualityGates) == 0 {
		t.Fatalf("expected quality gates")
	}
	if len(record.Evidence) == 0 {
		t.Fatalf("expected evidence claims from legal/hearing input")
	}
	if len(record.Events) < 2 {
		t.Fatalf("events = %d, want intake normalization and audit events", len(record.Events))
	}
	if len(record.SourceLinks) != 1 {
		t.Fatalf("source links = %d, want 1", len(record.SourceLinks))
	}
	if len(record.Decisions) < 3 {
		t.Fatalf("decisions = %d, want classification/priority/approval decisions", len(record.Decisions))
	}
	if len(record.Transitions) != 1 {
		t.Fatalf("transitions = %d, want intake transition", len(record.Transitions))
	}
}

func TestDashboardSurfacesQueuesAndRules(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	legal, err := service.Intake(IntakeRequest{Input: "Email from lawyer about Vivare hearing tomorrow. Draft reply only."})
	if err != nil {
		t.Fatalf("Intake legal: %v", err)
	}
	admin, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for low risk admin work"})
	if err != nil {
		t.Fatalf("Intake admin: %v", err)
	}
	blocked, err := service.Intake(IntakeRequest{Input: "Need access, missing login credentials for local folder source."})
	if err != nil {
		t.Fatalf("Intake blocked: %v", err)
	}

	dashboard, err := service.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if dashboard.Counts["approvals"] == 0 {
		t.Fatalf("expected approval count, got %#v", dashboard.Counts)
	}
	if dashboard.Counts["ready"] == 0 {
		t.Fatalf("expected ready count for %s, got %#v", admin.Item.ID, dashboard.Counts)
	}
	if dashboard.Counts["blocked"] == 0 {
		t.Fatalf("expected blocked count for %s, got %#v", blocked.Item.ID, dashboard.Counts)
	}
	if len(dashboard.Rules) < 10 {
		t.Fatalf("rules = %d, want default rulebook", len(dashboard.Rules))
	}
	if len(dashboard.HighRiskItems) == 0 || dashboard.HighRiskItems[0].ID != legal.Item.ID {
		t.Fatalf("expected high-risk legal item in dashboard")
	}
}

func TestTransitionRequiresApprovalFromNeedsApprovalToReady(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Publish public Medium article from Trello card"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}

	if _, err := service.Transition(record.Item.ID, TransitionRequest{TargetState: StateReady}); err == nil {
		t.Fatalf("expected transition without approval to fail")
	}
	approved, err := service.Transition(record.Item.ID, TransitionRequest{TargetState: StateReady, Approved: true, Message: "Robert approved draft-only workflow"})
	if err != nil {
		t.Fatalf("Transition approved: %v", err)
	}
	if approved.Item.CurrentState != StateReady {
		t.Fatalf("state = %q, want ready", approved.Item.CurrentState)
	}
	if approved.Item.ApprovalStatus != "approved" {
		t.Fatalf("approval status = %q, want approved", approved.Item.ApprovalStatus)
	}
	if len(approved.Events) < 2 {
		t.Fatalf("expected transition audit event")
	}
}

func TestTransitionRequiresApprovalForBlockedApprovalWorkflowToReady(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Email from lawyer about legal hearing. Draft formal reply."})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	item := record.Item
	item.CurrentState = StateBlocked
	item.RequiresApproval = true
	item.ApprovalStatus = "pending"
	if _, err := repo.UpdateItem(&item); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	if _, err := service.Transition(record.Item.ID, TransitionRequest{TargetState: StateReady}); err == nil {
		t.Fatalf("expected blocked approval-required workflow to require approval before ready")
	}
	approved, err := service.Transition(record.Item.ID, TransitionRequest{TargetState: StateReady, Approved: true, Message: "Robert approved controlled draft preparation"})
	if err != nil {
		t.Fatalf("Transition approved: %v", err)
	}
	if approved.Item.CurrentState != StateReady {
		t.Fatalf("state = %q, want ready", approved.Item.CurrentState)
	}
	if approved.Item.ApprovalStatus != "approved" {
		t.Fatalf("approval status = %q, want approved", approved.Item.ApprovalStatus)
	}
}

func TestTransitionRejectsManualCompletion(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for low risk admin work"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}

	if _, err := service.Transition(record.Item.ID, TransitionRequest{TargetState: StateCompleted, Message: "looks done"}); err == nil {
		t.Fatalf("expected manual completion to be rejected")
	}
}

func TestRunDueSkipsReadyApprovalWorkflowWithoutApproval(t *testing.T) {
	repo := newFakeWorkflowRepo()
	runner := &fakeTaskRunner{result: &TaskRunResult{
		PlanID:             "plan-approval",
		CompletionStatus:   "validated",
		VerificationStatus: "verified",
		Output:             "completed",
		Passed:             true,
	}}
	service := NewServiceWithTaskRunner(repo, runner)
	record, err := service.Intake(IntakeRequest{Input: "Email from lawyer about legal hearing. Draft formal reply."})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	item := record.Item
	item.CurrentState = StateReady
	item.RequiresApproval = true
	item.ApprovalStatus = "pending"
	if _, err := repo.UpdateItem(&item); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	summary, err := service.RunDue(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if summary.Checked != 0 {
		t.Fatalf("checked = %d, want 0 for unapproved ready workflow", summary.Checked)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("runner requests = %d, want 0", len(runner.requests))
	}
}

func TestResolveProposalApprovesWorkflow(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Publish public Medium article from Trello card"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	approved, err := service.ResolveProposal(record.Item.ID, record.Proposals[0].ID, ProposalResolutionRequest{
		Approved:       true,
		SelectedOption: "Approve draft-only workflow",
		Note:           "Robert approved draft preparation only.",
		Actor:          "Robert",
	})
	if err != nil {
		t.Fatalf("ResolveProposal: %v", err)
	}
	if approved.Item.CurrentState != StateReady {
		t.Fatalf("state = %q, want ready", approved.Item.CurrentState)
	}
	if approved.Item.ApprovalStatus != "approved" {
		t.Fatalf("approval status = %q, want approved", approved.Item.ApprovalStatus)
	}
	if approved.Proposals[0].Status != "approved" {
		t.Fatalf("proposal status = %q, want approved", approved.Proposals[0].Status)
	}
}

func TestResolveProposalDefaultsToChangesRequested(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for low risk admin work"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	updated, err := service.ResolveProposal(record.Item.ID, record.Proposals[0].ID, ProposalResolutionRequest{
		Note:  "Needs more detail before execution.",
		Actor: "Robert",
	})
	if err != nil {
		t.Fatalf("ResolveProposal: %v", err)
	}
	if updated.Proposals[0].Status != "changes_requested" {
		t.Fatalf("proposal status = %q, want changes_requested", updated.Proposals[0].Status)
	}
	if updated.Item.CurrentState != StateWaitingInput {
		t.Fatalf("state = %q, want waiting_external_input", updated.Item.CurrentState)
	}
}

func TestResolveProposalRejectsClosedWorkflow(t *testing.T) {
	repo := newFakeWorkflowRepo()
	runner := &fakeTaskRunner{result: &TaskRunResult{
		PlanID:             "plan-closed",
		CompletionStatus:   "validated",
		VerificationStatus: "verified",
		Output:             "completed",
		Passed:             true,
	}}
	service := NewServiceWithTaskRunner(repo, runner)
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for low risk admin work"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if _, err := service.RunDue(RunDueRequest{Limit: 5}); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if _, err := service.ResolveProposal(record.Item.ID, record.Proposals[0].ID, ProposalResolutionRequest{Approved: true}); err == nil {
		t.Fatalf("expected closed workflow proposal resolution to fail")
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.CurrentState != StateCompleted {
		t.Fatalf("state = %q, want completed", updated.Item.CurrentState)
	}
	if updated.Proposals[0].Status != "open" {
		t.Fatalf("proposal status = %q, want unchanged open", updated.Proposals[0].Status)
	}
}

func TestRunDueOpenLoopsCreatesFollowUpProposal(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Email from lawyer about Vivare hearing tomorrow. Draft reply only."})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	loops := repo.openLoops[record.Item.ID]
	if len(loops) == 0 {
		t.Fatalf("expected intake open loop")
	}
	loops[0].FollowUpAt = timePtr(time.Now().UTC().Add(-time.Hour))
	repo.openLoops[record.Item.ID] = loops

	summary, err := service.RunDueOpenLoops(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDueOpenLoops: %v", err)
	}
	if summary.Triggered != 1 {
		t.Fatalf("triggered = %d, want 1: %#v", summary.Triggered, summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(updated.Proposals) < 2 {
		t.Fatalf("proposals = %d, want follow-up proposal added", len(updated.Proposals))
	}
	if updated.OpenLoops[0].Status != "triggered" {
		t.Fatalf("open loop status = %q, want triggered", updated.OpenLoops[0].Status)
	}
	if updated.Item.CurrentState != StateNeedsApproval {
		t.Fatalf("state = %q, want needs approval for legal follow-up", updated.Item.CurrentState)
	}
}

func TestRunDueOpenLoopsSkipsWhenEmergencyStopActive(t *testing.T) {
	t.Setenv("HAI_EMERGENCY_STOP", "true")
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Email from lawyer about Vivare hearing tomorrow. Draft reply only."})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	loops := repo.openLoops[record.Item.ID]
	if len(loops) == 0 {
		t.Fatalf("expected intake open loop")
	}
	loops[0].FollowUpAt = timePtr(time.Now().UTC().Add(-time.Hour))
	repo.openLoops[record.Item.ID] = loops

	summary, err := service.RunDueOpenLoops(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDueOpenLoops: %v", err)
	}
	if summary.Skipped != 1 || summary.Triggered != 0 {
		t.Fatalf("summary = %#v, want one skipped and no triggered loops", summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.OpenLoops[0].Status != "open" {
		t.Fatalf("open loop status = %q, want open", updated.OpenLoops[0].Status)
	}
}

func TestRunDueOpenLoopsKeepsBlockedWorkflowBlocked(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Need access, missing login credentials for local folder source."})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	loops := repo.openLoops[record.Item.ID]
	if len(loops) == 0 {
		t.Fatalf("expected intake open loop")
	}
	loops[0].FollowUpAt = timePtr(time.Now().UTC().Add(-time.Hour))
	repo.openLoops[record.Item.ID] = loops

	summary, err := service.RunDueOpenLoops(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDueOpenLoops: %v", err)
	}
	if summary.Triggered != 1 {
		t.Fatalf("triggered = %d, want 1: %#v", summary.Triggered, summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.CurrentState != StateBlocked {
		t.Fatalf("state = %q, want blocked", updated.Item.CurrentState)
	}
	if updated.Item.BlockedReason != "missing information or access" {
		t.Fatalf("blocked reason = %q, want original blocker", updated.Item.BlockedReason)
	}
	if updated.OpenLoops[0].Status != "triggered" {
		t.Fatalf("open loop status = %q, want triggered", updated.OpenLoops[0].Status)
	}
}

func TestRunDueOpenLoopsSkipsOpenLoopClaimedByAnotherWorker(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{
		Input: "Waiting for a client response; follow up tomorrow.",
	})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if len(record.OpenLoops) == 0 {
		t.Fatalf("expected an open loop")
	}
	loops := repo.openLoops[record.Item.ID]
	loops[0].FollowUpAt = timePtr(time.Now().UTC().Add(-time.Hour))
	repo.openLoops[record.Item.ID] = loops
	claimedAt := time.Now().UTC()
	claimed, acquired, err := repo.ClaimDueOpenLoop(loops[0].ID, "test-claim", claimedAt, claimedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("ClaimDueOpenLoop: %v", err)
	}
	if !acquired || claimed == nil || claimed.Status != "processing" {
		t.Fatalf("claim = %#v, acquired = %t, want processing claim", claimed, acquired)
	}
	claimed.Status = "open"
	claimed.ClaimID = ""
	claimed.LeaseUntil = nil
	if _, err := repo.UpdateOpenLoop(claimed); err != nil {
		t.Fatalf("release test claim: %v", err)
	}
	repo.rejectOpenLoopClaims = true

	summary, err := service.RunDueOpenLoops(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDueOpenLoops: %v", err)
	}
	if summary.Skipped != 1 || summary.Triggered != 0 {
		t.Fatalf("summary = %#v, want one skipped and no triggered follow-up", summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(updated.Proposals) != len(record.Proposals) || len(updated.Checklist) != len(record.Checklist) {
		t.Fatalf("claim rejection created duplicate follow-up artifacts")
	}
}

func TestRunDueOpenLoopsReusesExistingFollowUpArtifacts(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Email from lawyer about Vivare hearing tomorrow. Draft reply only."})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	loops := repo.openLoops[record.Item.ID]
	if len(loops) == 0 {
		t.Fatalf("expected an open loop")
	}
	loops[0].FollowUpAt = timePtr(time.Now().UTC().Add(-time.Hour))
	repo.openLoops[record.Item.ID] = loops
	checklistLabel := "Resolve due open loop: " + compact(loops[0].WaitingFor, 160)
	recommendedAction := "Follow-up due: " + firstNonEmpty(loops[0].NextAction, loops[0].WaitingFor)
	if _, err := repo.CreateChecklistItem(&models.WorkflowChecklistItem{
		WorkflowID: record.Item.ID,
		Label:      checklistLabel,
		Status:     "open",
	}); err != nil {
		t.Fatalf("CreateChecklistItem: %v", err)
	}
	if _, err := repo.CreateProposal(&models.WorkflowProposal{
		WorkflowID:        record.Item.ID,
		RecommendedAction: recommendedAction,
		Status:            "open",
	}); err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	checklistCount := len(repo.checklist[record.Item.ID])
	proposalCount := len(repo.proposals[record.Item.ID])

	summary, err := service.RunDueOpenLoops(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDueOpenLoops: %v", err)
	}
	if summary.Triggered != 1 {
		t.Fatalf("summary = %#v, want one triggered follow-up", summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(updated.Checklist) != checklistCount || len(updated.Proposals) != proposalCount {
		t.Fatalf("existing follow-up artifacts were duplicated")
	}
}

func TestChecklistUpdateAuditsProgress(t *testing.T) {
	service := NewService(newFakeWorkflowRepo())
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for Docker build issue"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	updated, err := service.UpdateChecklistItem(record.Item.ID, record.Checklist[0].ID, ChecklistUpdateRequest{Status: "done"})
	if err != nil {
		t.Fatalf("UpdateChecklistItem: %v", err)
	}
	if updated.Checklist[0].Status != "done" {
		t.Fatalf("checklist status = %q, want done", updated.Checklist[0].Status)
	}
	if len(updated.Events) < 2 {
		t.Fatalf("expected checklist audit event")
	}
}

func TestRunDueConsumesApprovedWorkflowWithTaskRunner(t *testing.T) {
	repo := newFakeWorkflowRepo()
	runner := &fakeTaskRunner{result: &TaskRunResult{
		PlanID:             "plan-1",
		CompletionStatus:   "validated",
		VerificationStatus: "verified",
		Output:             "completed",
		Passed:             true,
	}}
	service := NewServiceWithTaskRunner(repo, runner)
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for low risk admin work"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if record.Item.CurrentState != StateReady {
		t.Fatalf("state = %q, want ready", record.Item.CurrentState)
	}
	summary, err := service.RunDue(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if summary.Completed != 1 {
		t.Fatalf("completed = %d, want 1: %#v", summary.Completed, summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.CurrentState != StateCompleted {
		t.Fatalf("state = %q, want completed", updated.Item.CurrentState)
	}
	if updated.Item.LastTaskPlanID != "plan-1" {
		t.Fatalf("plan id = %q, want plan-1", updated.Item.LastTaskPlanID)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("runner requests = %d, want 1", len(runner.requests))
	}
	if !hasGateStatus(updated.QualityGates, "verification before completion", "passed") {
		t.Fatalf("expected verification quality gate to pass")
	}
}

func TestRunDueSkipsWorkflowClaimedByAnotherWorker(t *testing.T) {
	repo := newFakeWorkflowRepo()
	runner := &fakeTaskRunner{result: &TaskRunResult{
		CompletionStatus:   "validated",
		VerificationStatus: "verified",
		Passed:             true,
	}}
	service := NewServiceWithTaskRunner(repo, runner)
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for low risk admin work"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	repo.rejectWorkflowClaims = true

	summary, err := service.RunDue(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if summary.Skipped != 1 || summary.Completed != 0 {
		t.Fatalf("summary = %#v, want one skipped and no completed workflow", summary)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("task runner executed an unclaimed workflow")
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.CurrentState != StateReady {
		t.Fatalf("state = %q, want ready after claim rejection", updated.Item.CurrentState)
	}
}

func TestRecoverStaleWorkflowClaimBlocksUnknownOutcome(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for low risk admin work"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	claimedAt := time.Now().UTC().Add(-2 * time.Minute)
	claimed, acquired, err := repo.ClaimRunnableItem(record.Item.ID, "expired-workflow-claim", claimedAt, claimedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("ClaimRunnableItem: %v", err)
	}
	if !acquired || claimed == nil {
		t.Fatalf("expected workflow claim")
	}

	summary, err := service.RecoverStaleClaims(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RecoverStaleClaims: %v", err)
	}
	if summary.WorkflowsBlocked != 1 {
		t.Fatalf("summary = %#v, want one blocked workflow", summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.CurrentState != StateBlocked {
		t.Fatalf("state = %q, want blocked", updated.Item.CurrentState)
	}
	if updated.Item.WorkerClaimID != "" || updated.Item.WorkerLeaseUntil != nil {
		t.Fatalf("expired claim was not cleared: %#v", updated.Item)
	}
	if updated.Item.RetryCount != 1 {
		t.Fatalf("retry count = %d, want interrupted attempt recorded", updated.Item.RetryCount)
	}
	if !strings.Contains(updated.Item.BlockedReason, "outcome is unknown") {
		t.Fatalf("blocked reason = %q, want unknown-outcome review", updated.Item.BlockedReason)
	}
	if updated.Item.RecoveryStatus != RecoveryNeedsReview {
		t.Fatalf("recovery status = %q, want needs_review", updated.Item.RecoveryStatus)
	}
	dashboard, err := service.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if dashboard.Counts["interruptedReview"] != 1 {
		t.Fatalf("interrupted review count = %d, want 1", dashboard.Counts["interruptedReview"])
	}
}

func TestInterruptedExecutionCannotBypassReview(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record := recoverInterruptedWorkflow(t, repo, service, "Create Trello checklist for low risk admin work")

	if _, err := service.Transition(record.Item.ID, TransitionRequest{TargetState: StateReady}); err == nil {
		t.Fatalf("expected generic transition to reject interrupted workflow")
	}
	if _, err := service.ResolveApproval(record.Item.ID, ApprovalResolutionRequest{Approved: true, Note: "approve"}); err == nil {
		t.Fatalf("expected approval resolution to reject interrupted workflow")
	}
	if _, err := service.ResolveProposal(record.Item.ID, record.Proposals[0].ID, ProposalResolutionRequest{Approved: true}); err == nil {
		t.Fatalf("expected proposal resolution to reject interrupted workflow")
	}
}

func TestResolveInterruptedExecutionRetryMakesLowRiskWorkflowReady(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record := recoverInterruptedWorkflow(t, repo, service, "Create Trello checklist for low risk admin work")

	updated, err := service.ResolveInterruptedExecution(record.Item.ID, InterruptedExecutionResolutionRequest{
		Decision: "retry",
		Note:     "Checked Trello and no checklist was created by the interrupted attempt.",
		Actor:    "Robert",
	})
	if err != nil {
		t.Fatalf("ResolveInterruptedExecution: %v", err)
	}
	if updated.Item.CurrentState != StateReady {
		t.Fatalf("state = %q, want ready", updated.Item.CurrentState)
	}
	if updated.Item.RecoveryStatus != RecoveryRetryConfirmed {
		t.Fatalf("recovery status = %q, want retry_confirmed", updated.Item.RecoveryStatus)
	}
	if updated.Item.BlockedReason != "" || updated.Item.LastWorkerError != "" {
		t.Fatalf("resolved retry retained active error: %#v", updated.Item)
	}
	if !hasDecision(updated.Decisions, "interrupted_execution", "retry") {
		t.Fatalf("expected interrupted-execution retry decision")
	}
}

func TestResolveInterruptedExecutionRetryRequiresFreshApprovalForHighRiskWorkflow(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record := recoverInterruptedWorkflow(t, repo, service, "Email lawyer about legal hearing and draft formal reply.")

	updated, err := service.ResolveInterruptedExecution(record.Item.ID, InterruptedExecutionResolutionRequest{
		Decision: "retry",
		Note:     "Checked sent mail and confirmed that no message was sent.",
		Actor:    "Robert",
	})
	if err != nil {
		t.Fatalf("ResolveInterruptedExecution: %v", err)
	}
	if updated.Item.CurrentState != StateNeedsApproval {
		t.Fatalf("state = %q, want needs_approval", updated.Item.CurrentState)
	}
	if updated.Item.ApprovalStatus != "pending" {
		t.Fatalf("approval status = %q, want pending", updated.Item.ApprovalStatus)
	}
	if updated.Item.MaxRetries <= updated.Item.RetryCount {
		t.Fatalf("operator-approved retry has no remaining attempt: %d/%d", updated.Item.RetryCount, updated.Item.MaxRetries)
	}
}

func TestInterruptedExecutionRetryCompletesThroughWorker(t *testing.T) {
	repo := newFakeWorkflowRepo()
	runner := &fakeTaskRunner{result: &TaskRunResult{
		PlanID:             "recovery-plan",
		CompletionStatus:   "validated",
		VerificationStatus: "verified",
		Output:             "verified completion after controlled retry",
		Passed:             true,
	}}
	service := NewServiceWithTaskRunner(repo, runner)
	record := recoverInterruptedWorkflow(t, repo, service, "Create Trello checklist for low risk admin work")
	retried, err := service.ResolveInterruptedExecution(record.Item.ID, InterruptedExecutionResolutionRequest{
		Decision: "retry",
		Note:     "Checked Trello and confirmed the interrupted attempt created no checklist.",
		Actor:    "Robert",
	})
	if err != nil {
		t.Fatalf("ResolveInterruptedExecution: %v", err)
	}

	summary, err := service.RunDue(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if summary.Completed != 1 {
		t.Fatalf("summary = %#v, want controlled retry completion", summary)
	}
	completed, err := service.Get(retried.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if completed.Item.RecoveryStatus != RecoveryCompletedAfterRetry {
		t.Fatalf("recovery status = %q, want completed_after_retry", completed.Item.RecoveryStatus)
	}
}

func TestResolveInterruptedExecutionCompletionRequiresEvidence(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record := recoverInterruptedWorkflow(t, repo, service, "Create Trello checklist for low risk admin work")

	if _, err := service.ResolveInterruptedExecution(record.Item.ID, InterruptedExecutionResolutionRequest{
		Decision: "confirm_completed",
		Note:     "The checklist exists.",
	}); err == nil {
		t.Fatalf("expected completion without evidence URI to fail")
	}
}

func TestResolveInterruptedExecutionCompletesWithEvidence(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record := recoverInterruptedWorkflow(t, repo, service, "Create Trello checklist for low risk admin work")

	updated, err := service.ResolveInterruptedExecution(record.Item.ID, InterruptedExecutionResolutionRequest{
		Decision:      "confirm_completed",
		Note:          "Verified the expected checklist exists and contains the requested items.",
		EvidenceURI:   "https://trello.example/card/123",
		EvidenceLabel: "Trello checklist",
		Actor:         "Robert",
	})
	if err != nil {
		t.Fatalf("ResolveInterruptedExecution: %v", err)
	}
	if updated.Item.CurrentState != StateCompleted || updated.Item.CompletedAt == nil {
		t.Fatalf("workflow was not completed: %#v", updated.Item)
	}
	if updated.Item.RecoveryStatus != RecoveryCompletionConfirmed || updated.Item.VerificationStatus != "human_approved" {
		t.Fatalf("recovery verification not recorded: %#v", updated.Item)
	}
	if !hasSourceRelationship(updated.SourceLinks, "completion_evidence") {
		t.Fatalf("expected completion evidence source link")
	}
	if !hasEvidenceStatus(updated.Evidence, "human_approved") {
		t.Fatalf("expected human-approved evidence claim")
	}
	if !hasGateStatus(updated.QualityGates, "verification before completion", "passed") {
		t.Fatalf("expected passed completion verification gate")
	}
}

func TestResolveInterruptedExecutionKeepBlockedRecordsReview(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record := recoverInterruptedWorkflow(t, repo, service, "Create Trello checklist for low risk admin work")

	updated, err := service.ResolveInterruptedExecution(record.Item.ID, InterruptedExecutionResolutionRequest{
		Decision: "keep_blocked",
		Note:     "The external system is unavailable, so side effects cannot be confirmed yet.",
		Actor:    "Robert",
	})
	if err != nil {
		t.Fatalf("ResolveInterruptedExecution: %v", err)
	}
	if updated.Item.CurrentState != StateBlocked || updated.Item.RecoveryStatus != RecoveryNeedsReview {
		t.Fatalf("workflow should remain in interrupted review: %#v", updated.Item)
	}
	if updated.Item.RecoveryNote == "" {
		t.Fatalf("expected operator review note")
	}
}

func TestRecoverStaleClaimsLeavesActiveWorkflowLeaseOwned(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for low risk admin work"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	claimedAt := time.Now().UTC()
	claimed, acquired, err := repo.ClaimRunnableItem(record.Item.ID, "active-workflow-claim", claimedAt, claimedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("ClaimRunnableItem: %v", err)
	}
	if !acquired || claimed == nil {
		t.Fatalf("expected workflow claim")
	}

	summary, err := service.RecoverStaleClaims(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RecoverStaleClaims: %v", err)
	}
	if summary.Checked != 0 || summary.WorkflowsBlocked != 0 {
		t.Fatalf("summary = %#v, active lease must not be recovered", summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.CurrentState != StateInProgress || updated.Item.WorkerClaimID != "active-workflow-claim" {
		t.Fatalf("active claim changed: %#v", updated.Item)
	}
}

func TestRecoverStaleOpenLoopClaimReopensIdempotentFollowUp(t *testing.T) {
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Waiting for a client response; follow up tomorrow."})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	loops := repo.openLoops[record.Item.ID]
	if len(loops) == 0 {
		t.Fatalf("expected an open loop")
	}
	loops[0].FollowUpAt = timePtr(time.Now().UTC().Add(-time.Hour))
	repo.openLoops[record.Item.ID] = loops
	claimedAt := time.Now().UTC().Add(-2 * time.Minute)
	claimed, acquired, err := repo.ClaimDueOpenLoop(loops[0].ID, "expired-open-loop-claim", claimedAt, claimedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("ClaimDueOpenLoop: %v", err)
	}
	if !acquired || claimed == nil {
		t.Fatalf("expected open-loop claim")
	}

	summary, err := service.RecoverStaleClaims(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RecoverStaleClaims: %v", err)
	}
	if summary.OpenLoopsReopened != 1 {
		t.Fatalf("summary = %#v, want one reopened open loop", summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.OpenLoops[0].Status != "open" || updated.OpenLoops[0].ClaimID != "" || updated.OpenLoops[0].LeaseUntil != nil {
		t.Fatalf("open-loop claim was not safely reopened: %#v", updated.OpenLoops[0])
	}
}

func TestRecoverStaleClaimsMigratesLegacyUnownedRows(t *testing.T) {
	t.Setenv("WORKFLOW_CLAIM_LEASE_SECONDS", "60")
	repo := newFakeWorkflowRepo()
	service := NewService(repo)
	record, err := service.Intake(IntakeRequest{Input: "Waiting for a client response; follow up tomorrow."})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	legacyTime := time.Now().UTC().Add(-2 * time.Minute)
	item := repo.items[record.Item.ID]
	item.CurrentState = StateInProgress
	item.LastRunAt = timePtr(legacyTime)
	item.WorkerClaimID = ""
	item.WorkerLeaseUntil = nil
	loops := repo.openLoops[record.Item.ID]
	if len(loops) == 0 {
		t.Fatalf("expected an open loop")
	}
	loops[0].Status = "processing"
	loops[0].ClaimID = ""
	loops[0].LeaseUntil = nil
	loops[0].UpdatedAt = legacyTime
	repo.openLoops[record.Item.ID] = loops

	summary, err := service.RecoverStaleClaims(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RecoverStaleClaims: %v", err)
	}
	if summary.WorkflowsBlocked != 1 || summary.OpenLoopsReopened != 1 {
		t.Fatalf("summary = %#v, want legacy workflow blocked and open loop reopened", summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.CurrentState != StateBlocked || updated.OpenLoops[0].Status != "open" {
		t.Fatalf("legacy rows were not recovered: item=%#v loop=%#v", updated.Item, updated.OpenLoops[0])
	}
}

func TestRunDueRecoversTaskRunnerPanicIntoRetry(t *testing.T) {
	repo := newFakeWorkflowRepo()
	runner := &fakeTaskRunner{panicValue: "provider adapter crashed"}
	service := NewServiceWithTaskRunner(repo, runner)
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for low risk admin work"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}

	summary, err := service.RunDue(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if summary.Retried != 1 {
		t.Fatalf("summary = %#v, want one scheduled retry", summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.CurrentState != StateReady {
		t.Fatalf("state = %q, want ready for retry", updated.Item.CurrentState)
	}
	if !strings.Contains(updated.Item.LastWorkerError, "panic recovered") {
		t.Fatalf("last worker error = %q, want recovered panic", updated.Item.LastWorkerError)
	}
}

func TestRunDueCannotPersistResultAfterClaimOwnershipChanges(t *testing.T) {
	repo := newFakeWorkflowRepo()
	var workflowID uuid.UUID
	runner := &fakeTaskRunner{
		result: &TaskRunResult{
			CompletionStatus:   "validated",
			VerificationStatus: "verified",
			Passed:             true,
		},
		onRun: func(request TaskRunRequest) {
			repo.items[workflowID].WorkerClaimID = "replacement-worker"
		},
	}
	service := NewServiceWithTaskRunner(repo, runner)
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for low risk admin work"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	workflowID = record.Item.ID

	summary, err := service.RunDue(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if summary.Blocked != 1 || summary.Completed != 0 {
		t.Fatalf("summary = %#v, lost claim must not complete workflow", summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.CurrentState != StateInProgress || updated.Item.WorkerClaimID != "replacement-worker" {
		t.Fatalf("stale worker overwrote replacement claim: %#v", updated.Item)
	}
}

func TestRunDueBlocksWorkflowWhenEmergencyStopActive(t *testing.T) {
	t.Setenv("HAI_EMERGENCY_STOP", "true")
	repo := newFakeWorkflowRepo()
	runner := &fakeTaskRunner{result: &TaskRunResult{Passed: true}}
	service := NewServiceWithTaskRunner(repo, runner)
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for low risk admin work"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}

	summary, err := service.RunDue(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if summary.Blocked != 1 {
		t.Fatalf("blocked = %d, want 1: %#v", summary.Blocked, summary)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("task runner was called while emergency stop was active")
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.CurrentState != StateReady {
		t.Fatalf("state = %q, want ready because emergency stop should not consume the item", updated.Item.CurrentState)
	}
}

func TestRunDueBlocksTechnicalWorkflowWhenQualityEvidenceMissing(t *testing.T) {
	repo := newFakeWorkflowRepo()
	runner := &fakeTaskRunner{result: &TaskRunResult{
		PlanID:             "plan-tech",
		CompletionStatus:   "validated",
		VerificationStatus: "verified",
		Output:             "completed",
		Passed:             true,
	}}
	service := NewServiceWithTaskRunner(repo, runner)
	record, err := service.Intake(IntakeRequest{Input: "Review GitHub developer claim that code feature is done"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	summary, err := service.RunDue(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if summary.Retried != 1 {
		t.Fatalf("retried = %d, want 1: %#v", summary.Retried, summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.CurrentState != StateReady {
		t.Fatalf("state = %q, want ready after scheduled retry", updated.Item.CurrentState)
	}
	if updated.Item.LastWorkerError == "" {
		t.Fatalf("expected quality gate failure reason")
	}
	if !hasGateStatus(updated.QualityGates, "tests or build evidence", "needs_review") {
		t.Fatalf("expected tests/build gate to need review")
	}
}

func TestDefaultRulesPreserveCreatedAtAcrossUpserts(t *testing.T) {
	service := NewService(newFakeWorkflowRepo())
	first := service.Overview()
	firstRule, ok := findRule(first.Rules, "approval.legal_external")
	if !ok {
		t.Fatalf("expected default rule")
	}
	second := service.Overview()
	secondRule, ok := findRule(second.Rules, "approval.legal_external")
	if !ok {
		t.Fatalf("expected default rule on second overview")
	}
	if !firstRule.CreatedAt.Equal(secondRule.CreatedAt) {
		t.Fatalf("created at changed from %s to %s", firstRule.CreatedAt, secondRule.CreatedAt)
	}
	if firstRule.ID != secondRule.ID {
		t.Fatalf("rule ID changed from %s to %s", firstRule.ID, secondRule.ID)
	}
}

func TestRunDueRetriesAndBlocksAfterLimit(t *testing.T) {
	repo := newFakeWorkflowRepo()
	runner := &fakeTaskRunner{result: &TaskRunResult{
		CompletionStatus:   "review_required",
		VerificationStatus: "needs_review",
		FailureReason:      "unsupported claims",
		Passed:             false,
		ReviewRequired:     true,
	}}
	service := NewServiceWithTaskRunner(repo, runner)
	record, err := service.Intake(IntakeRequest{Input: "Create Trello checklist for low risk admin work"})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	item := record.Item
	item.MaxRetries = 1
	if _, err := repo.UpdateItem(&item); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
	summary, err := service.RunDue(RunDueRequest{Limit: 5})
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if summary.Blocked != 1 {
		t.Fatalf("blocked = %d, want 1: %#v", summary.Blocked, summary)
	}
	updated, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.Item.CurrentState != StateBlocked {
		t.Fatalf("state = %q, want blocked", updated.Item.CurrentState)
	}
	if updated.Item.RetryCount != 1 {
		t.Fatalf("retry count = %d, want 1", updated.Item.RetryCount)
	}
}

func TestIntakeDeduplicatesBySourceURI(t *testing.T) {
	service := NewService(newFakeWorkflowRepo())
	first, err := service.Intake(IntakeRequest{
		Input:     "Follow up: request missing evidence bundle from legal contact.",
		SourceURI: "local://source/item/123",
		Trigger:   "source.extraction",
	})
	if err != nil {
		t.Fatalf("Intake first: %v", err)
	}
	second, err := service.Intake(IntakeRequest{
		Input:     "Follow up: request missing evidence bundle from legal contact.",
		SourceURI: "local://source/item/123",
		Trigger:   "source.extraction",
	})
	if err != nil {
		t.Fatalf("Intake second: %v", err)
	}
	if first.Item.ID != second.Item.ID {
		t.Fatalf("deduped ID = %s, want %s", second.Item.ID, first.Item.ID)
	}
	if len(second.Events) < 2 {
		t.Fatalf("expected dedupe audit event")
	}
}

func hasApprovalChecklist(items []models.WorkflowChecklistItem) bool {
	for _, item := range items {
		if item.RequiresApproval {
			return true
		}
	}
	return false
}

func hasGateStatus(gates []models.WorkflowQualityGate, gateName, status string) bool {
	for _, gate := range gates {
		if gate.Gate == gateName && gate.Status == status {
			return true
		}
	}
	return false
}

func hasDecision(decisions []models.WorkflowDecision, decisionType, decision string) bool {
	for _, item := range decisions {
		if item.DecisionType == decisionType && item.Decision == decision {
			return true
		}
	}
	return false
}

func hasSourceRelationship(links []models.WorkflowSourceLink, relationship string) bool {
	for _, link := range links {
		if link.Relationship == relationship {
			return true
		}
	}
	return false
}

func hasEvidenceStatus(evidence []models.WorkflowEvidenceClaim, status string) bool {
	for _, claim := range evidence {
		if claim.Status == status {
			return true
		}
	}
	return false
}

func recoverInterruptedWorkflow(t *testing.T, repo *fakeWorkflowRepo, service Service, input string) *WorkflowRecord {
	t.Helper()
	record, err := service.Intake(IntakeRequest{Input: input})
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if record.Item.RequiresApproval {
		record, err = service.ResolveApproval(record.Item.ID, ApprovalResolutionRequest{
			Approved: true,
			Note:     "Initial controlled execution approved for test.",
			Actor:    "Robert",
		})
		if err != nil {
			t.Fatalf("ResolveApproval: %v", err)
		}
	}
	claimedAt := time.Now().UTC().Add(-2 * time.Minute)
	claimed, acquired, err := repo.ClaimRunnableItem(record.Item.ID, "expired-workflow-claim", claimedAt, claimedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("ClaimRunnableItem: %v", err)
	}
	if !acquired || claimed == nil {
		t.Fatalf("expected workflow claim")
	}
	if _, err := service.RecoverStaleClaims(RunDueRequest{Limit: 5}); err != nil {
		t.Fatalf("RecoverStaleClaims: %v", err)
	}
	recovered, err := service.Get(record.Item.ID)
	if err != nil {
		t.Fatalf("Get recovered workflow: %v", err)
	}
	return recovered
}

func findRule(rules []models.WorkflowRule, ruleKey string) (models.WorkflowRule, bool) {
	for _, rule := range rules {
		if rule.RuleKey == ruleKey {
			return rule, true
		}
	}
	return models.WorkflowRule{}, false
}

func timePtr(value time.Time) *time.Time {
	return &value
}

type fakeWorkflowRepo struct {
	items                map[uuid.UUID]*models.WorkflowItem
	checklist            map[uuid.UUID][]models.WorkflowChecklistItem
	intake               map[uuid.UUID][]models.WorkflowIntakeRecord
	matches              map[uuid.UUID][]models.WorkflowProjectMatch
	evidence             map[uuid.UUID][]models.WorkflowEvidenceClaim
	openLoops            map[uuid.UUID][]models.WorkflowOpenLoop
	proposals            map[uuid.UUID][]models.WorkflowProposal
	qualityGate          map[uuid.UUID][]models.WorkflowQualityGate
	rules                map[string]models.WorkflowRule
	transitions          map[uuid.UUID][]models.WorkflowTransition
	sourceLinks          map[uuid.UUID][]models.WorkflowSourceLink
	decisions            map[uuid.UUID][]models.WorkflowDecision
	events               map[uuid.UUID][]models.WorkflowEvent
	rejectWorkflowClaims bool
	rejectOpenLoopClaims bool
}

func newFakeWorkflowRepo() *fakeWorkflowRepo {
	return &fakeWorkflowRepo{
		items:       map[uuid.UUID]*models.WorkflowItem{},
		checklist:   map[uuid.UUID][]models.WorkflowChecklistItem{},
		intake:      map[uuid.UUID][]models.WorkflowIntakeRecord{},
		matches:     map[uuid.UUID][]models.WorkflowProjectMatch{},
		evidence:    map[uuid.UUID][]models.WorkflowEvidenceClaim{},
		openLoops:   map[uuid.UUID][]models.WorkflowOpenLoop{},
		proposals:   map[uuid.UUID][]models.WorkflowProposal{},
		qualityGate: map[uuid.UUID][]models.WorkflowQualityGate{},
		rules:       map[string]models.WorkflowRule{},
		transitions: map[uuid.UUID][]models.WorkflowTransition{},
		sourceLinks: map[uuid.UUID][]models.WorkflowSourceLink{},
		decisions:   map[uuid.UUID][]models.WorkflowDecision{},
		events:      map[uuid.UUID][]models.WorkflowEvent{},
	}
}

func (r *fakeWorkflowRepo) CreateItem(item *models.WorkflowItem) (*models.WorkflowItem, error) {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now
	r.items[item.ID] = item
	return item, nil
}

func (r *fakeWorkflowRepo) UpdateItem(item *models.WorkflowItem) (*models.WorkflowItem, error) {
	item.UpdatedAt = time.Now().UTC()
	r.items[item.ID] = item
	return item, nil
}

func (r *fakeWorkflowRepo) FindItem(id uuid.UUID) (*models.WorkflowItem, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copied := *item
	return &copied, nil
}

func (r *fakeWorkflowRepo) FindActiveItemBySourceURI(sourceURI string) (*models.WorkflowItem, error) {
	for _, item := range r.items {
		if item.SourceURI == sourceURI && !item.Archived {
			copied := *item
			return &copied, nil
		}
	}
	return nil, nil
}

func (r *fakeWorkflowRepo) FindItems(includeArchived bool) ([]models.WorkflowItem, error) {
	result := []models.WorkflowItem{}
	for _, item := range r.items {
		if includeArchived || !item.Archived {
			result = append(result, *item)
		}
	}
	return result, nil
}

func (r *fakeWorkflowRepo) FindApprovalItems() ([]models.WorkflowItem, error) {
	result := []models.WorkflowItem{}
	for _, item := range r.items {
		if item.CurrentState == StateNeedsApproval && !item.Archived {
			result = append(result, *item)
		}
	}
	return result, nil
}

func (r *fakeWorkflowRepo) FindRunnableItems(now time.Time, limit int) ([]models.WorkflowItem, error) {
	result := []models.WorkflowItem{}
	for _, item := range r.items {
		if item.CurrentState != StateReady || item.Archived || item.RetryCount >= item.MaxRetries {
			continue
		}
		if item.RequiresApproval && item.ApprovalStatus != "approved" {
			continue
		}
		if item.NextRunAt != nil && item.NextRunAt.After(now) {
			continue
		}
		result = append(result, *item)
	}
	if limit > 0 && len(result) > limit {
		return result[:limit], nil
	}
	return result, nil
}

func (r *fakeWorkflowRepo) ClaimRunnableItem(id uuid.UUID, claimID string, now time.Time, leaseUntil time.Time) (*models.WorkflowItem, bool, error) {
	if r.rejectWorkflowClaims {
		return nil, false, nil
	}
	item, ok := r.items[id]
	if !ok || item.Archived || item.CurrentState != StateReady || item.RetryCount >= item.MaxRetries {
		return nil, false, nil
	}
	if item.RequiresApproval && item.ApprovalStatus != "approved" {
		return nil, false, nil
	}
	if item.NextRunAt != nil && item.NextRunAt.After(now) {
		return nil, false, nil
	}
	item.CurrentState = StateInProgress
	item.LastRunAt = timePtr(now)
	item.NextAction = "task engine is executing claimed workflow item"
	item.LastWorkerError = ""
	item.WorkerClaimID = claimID
	item.WorkerLeaseUntil = timePtr(leaseUntil)
	item.UpdatedAt = now
	copied := *item
	return &copied, true, nil
}

func (r *fakeWorkflowRepo) RenewRunnableItemClaim(id uuid.UUID, claimID string, leaseUntil time.Time) (bool, error) {
	item, ok := r.items[id]
	if !ok || item.CurrentState != StateInProgress || item.WorkerClaimID != claimID {
		return false, nil
	}
	item.WorkerLeaseUntil = timePtr(leaseUntil)
	item.UpdatedAt = time.Now().UTC()
	return true, nil
}

func (r *fakeWorkflowRepo) UpdateClaimedItem(item *models.WorkflowItem, claimID string) (*models.WorkflowItem, bool, error) {
	stored, ok := r.items[item.ID]
	if !ok || stored.CurrentState != StateInProgress || stored.WorkerClaimID != claimID {
		return nil, false, nil
	}
	copied := *item
	copied.WorkerClaimID = ""
	copied.WorkerLeaseUntil = nil
	copied.UpdatedAt = time.Now().UTC()
	r.items[item.ID] = &copied
	result := copied
	return &result, true, nil
}

func (r *fakeWorkflowRepo) FindExpiredWorkflowClaims(now time.Time, limit int) ([]models.WorkflowItem, error) {
	result := []models.WorkflowItem{}
	for _, item := range r.items {
		if item.CurrentState != StateInProgress {
			continue
		}
		expiredLease := item.WorkerClaimID != "" && item.WorkerLeaseUntil != nil && !item.WorkerLeaseUntil.After(now)
		expiredLegacyClaim := item.WorkerClaimID == "" && item.WorkerLeaseUntil == nil && item.LastRunAt != nil && !item.LastRunAt.After(now.Add(-claimLeaseDuration()))
		if !expiredLease && !expiredLegacyClaim {
			continue
		}
		result = append(result, *item)
	}
	if limit > 0 && len(result) > limit {
		return result[:limit], nil
	}
	return result, nil
}

func (r *fakeWorkflowRepo) RecoverExpiredWorkflowClaim(item models.WorkflowItem, now time.Time) (*models.WorkflowItem, bool, error) {
	stored, ok := r.items[item.ID]
	if !ok || stored.CurrentState != StateInProgress || stored.WorkerClaimID != item.WorkerClaimID {
		return nil, false, nil
	}
	expiredLease := stored.WorkerClaimID != "" && stored.WorkerLeaseUntil != nil && !stored.WorkerLeaseUntil.After(now)
	expiredLegacyClaim := stored.WorkerClaimID == "" && stored.WorkerLeaseUntil == nil && stored.LastRunAt != nil && !stored.LastRunAt.After(now.Add(-claimLeaseDuration()))
	if !expiredLease && !expiredLegacyClaim {
		return nil, false, nil
	}
	stored.CurrentState = StateBlocked
	stored.BlockedReason = "worker lease expired; execution outcome is unknown and requires human review"
	stored.NextAction = "review external side effects before retrying interrupted workflow"
	stored.LastWorkerError = stored.BlockedReason
	stored.RecoveryStatus = RecoveryNeedsReview
	stored.RecoveryNote = ""
	stored.RetryCount++
	stored.NextRunAt = nil
	stored.WorkerClaimID = ""
	stored.WorkerLeaseUntil = nil
	stored.UpdatedAt = now
	copied := *stored
	return &copied, true, nil
}

func (r *fakeWorkflowRepo) CreateChecklistItem(item *models.WorkflowChecklistItem) (*models.WorkflowChecklistItem, error) {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now
	r.checklist[item.WorkflowID] = append(r.checklist[item.WorkflowID], *item)
	return item, nil
}

func (r *fakeWorkflowRepo) UpdateChecklistItem(item *models.WorkflowChecklistItem) (*models.WorkflowChecklistItem, error) {
	item.UpdatedAt = time.Now().UTC()
	items := r.checklist[item.WorkflowID]
	for index := range items {
		if items[index].ID == item.ID {
			items[index] = *item
			r.checklist[item.WorkflowID] = items
			return item, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeWorkflowRepo) FindChecklist(workflowID uuid.UUID) ([]models.WorkflowChecklistItem, error) {
	return append([]models.WorkflowChecklistItem{}, r.checklist[workflowID]...), nil
}

func (r *fakeWorkflowRepo) SaveIntakeRecord(record *models.WorkflowIntakeRecord) (*models.WorkflowIntakeRecord, error) {
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	record.CreatedAt = time.Now().UTC()
	r.intake[record.WorkflowID] = append([]models.WorkflowIntakeRecord{*record}, r.intake[record.WorkflowID]...)
	return record, nil
}

func (r *fakeWorkflowRepo) FindIntakeRecords(workflowID uuid.UUID) ([]models.WorkflowIntakeRecord, error) {
	return append([]models.WorkflowIntakeRecord{}, r.intake[workflowID]...), nil
}

func (r *fakeWorkflowRepo) CreateProjectMatch(match *models.WorkflowProjectMatch) (*models.WorkflowProjectMatch, error) {
	if match.ID == uuid.Nil {
		match.ID = uuid.New()
	}
	match.CreatedAt = time.Now().UTC()
	r.matches[match.WorkflowID] = append([]models.WorkflowProjectMatch{*match}, r.matches[match.WorkflowID]...)
	return match, nil
}

func (r *fakeWorkflowRepo) FindProjectMatches(workflowID uuid.UUID) ([]models.WorkflowProjectMatch, error) {
	return append([]models.WorkflowProjectMatch{}, r.matches[workflowID]...), nil
}

func (r *fakeWorkflowRepo) CreateEvidenceClaim(claim *models.WorkflowEvidenceClaim) (*models.WorkflowEvidenceClaim, error) {
	if claim.ID == uuid.Nil {
		claim.ID = uuid.New()
	}
	claim.CreatedAt = time.Now().UTC()
	r.evidence[claim.WorkflowID] = append([]models.WorkflowEvidenceClaim{*claim}, r.evidence[claim.WorkflowID]...)
	return claim, nil
}

func (r *fakeWorkflowRepo) FindEvidenceClaims(workflowID uuid.UUID) ([]models.WorkflowEvidenceClaim, error) {
	return append([]models.WorkflowEvidenceClaim{}, r.evidence[workflowID]...), nil
}

func (r *fakeWorkflowRepo) CreateOpenLoop(loop *models.WorkflowOpenLoop) (*models.WorkflowOpenLoop, error) {
	if loop.ID == uuid.Nil {
		loop.ID = uuid.New()
	}
	now := time.Now().UTC()
	loop.CreatedAt = now
	loop.UpdatedAt = now
	r.openLoops[loop.WorkflowID] = append([]models.WorkflowOpenLoop{*loop}, r.openLoops[loop.WorkflowID]...)
	return loop, nil
}

func (r *fakeWorkflowRepo) UpdateOpenLoop(loop *models.WorkflowOpenLoop) (*models.WorkflowOpenLoop, error) {
	loop.UpdatedAt = time.Now().UTC()
	loops := r.openLoops[loop.WorkflowID]
	for index := range loops {
		if loops[index].ID == loop.ID {
			loops[index] = *loop
			r.openLoops[loop.WorkflowID] = loops
			return loop, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeWorkflowRepo) FindOpenLoops(workflowID uuid.UUID) ([]models.WorkflowOpenLoop, error) {
	return append([]models.WorkflowOpenLoop{}, r.openLoops[workflowID]...), nil
}

func (r *fakeWorkflowRepo) FindDashboardOpenLoops(now time.Time) ([]models.WorkflowOpenLoop, error) {
	result := []models.WorkflowOpenLoop{}
	for _, loops := range r.openLoops {
		for _, loop := range loops {
			if loop.Status != "open" {
				continue
			}
			if loop.FollowUpAt == nil || !loop.FollowUpAt.After(now) {
				result = append(result, loop)
			}
		}
	}
	return result, nil
}

func (r *fakeWorkflowRepo) ClaimDueOpenLoop(id uuid.UUID, claimID string, now time.Time, leaseUntil time.Time) (*models.WorkflowOpenLoop, bool, error) {
	if r.rejectOpenLoopClaims {
		return nil, false, nil
	}
	for workflowID, loops := range r.openLoops {
		for index := range loops {
			if loops[index].ID != id || loops[index].Status != "open" {
				continue
			}
			if loops[index].FollowUpAt != nil && loops[index].FollowUpAt.After(now) {
				return nil, false, nil
			}
			loops[index].Status = "processing"
			loops[index].ClaimID = claimID
			loops[index].LeaseUntil = timePtr(leaseUntil)
			loops[index].UpdatedAt = now
			r.openLoops[workflowID] = loops
			copied := loops[index]
			return &copied, true, nil
		}
	}
	return nil, false, nil
}

func (r *fakeWorkflowRepo) RenewOpenLoopClaim(id uuid.UUID, claimID string, leaseUntil time.Time) (bool, error) {
	for workflowID, loops := range r.openLoops {
		for index := range loops {
			if loops[index].ID != id || loops[index].Status != "processing" || loops[index].ClaimID != claimID {
				continue
			}
			loops[index].LeaseUntil = timePtr(leaseUntil)
			loops[index].UpdatedAt = time.Now().UTC()
			r.openLoops[workflowID] = loops
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeWorkflowRepo) UpdateClaimedOpenLoop(loop *models.WorkflowOpenLoop, claimID string) (*models.WorkflowOpenLoop, bool, error) {
	for workflowID, loops := range r.openLoops {
		for index := range loops {
			stored := &loops[index]
			if stored.ID != loop.ID || stored.Status != "processing" || stored.ClaimID != claimID {
				continue
			}
			stored.Status = loop.Status
			stored.ClaimID = ""
			stored.LeaseUntil = nil
			stored.UpdatedAt = time.Now().UTC()
			r.openLoops[workflowID] = loops
			copied := *stored
			return &copied, true, nil
		}
	}
	return nil, false, nil
}

func (r *fakeWorkflowRepo) FindExpiredOpenLoopClaims(now time.Time, limit int) ([]models.WorkflowOpenLoop, error) {
	result := []models.WorkflowOpenLoop{}
	for _, loops := range r.openLoops {
		for _, loop := range loops {
			if loop.Status != "processing" {
				continue
			}
			expiredLease := loop.ClaimID != "" && loop.LeaseUntil != nil && !loop.LeaseUntil.After(now)
			expiredLegacyClaim := loop.ClaimID == "" && loop.LeaseUntil == nil && !loop.UpdatedAt.After(now.Add(-claimLeaseDuration()))
			if !expiredLease && !expiredLegacyClaim {
				continue
			}
			result = append(result, loop)
		}
	}
	if limit > 0 && len(result) > limit {
		return result[:limit], nil
	}
	return result, nil
}

func (r *fakeWorkflowRepo) RecoverExpiredOpenLoopClaim(loop models.WorkflowOpenLoop, now time.Time) (*models.WorkflowOpenLoop, bool, error) {
	for workflowID, loops := range r.openLoops {
		for index := range loops {
			stored := &loops[index]
			if stored.ID != loop.ID || stored.Status != "processing" || stored.ClaimID != loop.ClaimID {
				continue
			}
			expiredLease := stored.ClaimID != "" && stored.LeaseUntil != nil && !stored.LeaseUntil.After(now)
			expiredLegacyClaim := stored.ClaimID == "" && stored.LeaseUntil == nil && !stored.UpdatedAt.After(now.Add(-claimLeaseDuration()))
			if !expiredLease && !expiredLegacyClaim {
				continue
			}
			stored.Status = "open"
			stored.ClaimID = ""
			stored.LeaseUntil = nil
			stored.UpdatedAt = now
			r.openLoops[workflowID] = loops
			copied := *stored
			return &copied, true, nil
		}
	}
	return nil, false, nil
}

func (r *fakeWorkflowRepo) CreateProposal(proposal *models.WorkflowProposal) (*models.WorkflowProposal, error) {
	if proposal.ID == uuid.Nil {
		proposal.ID = uuid.New()
	}
	now := time.Now().UTC()
	proposal.CreatedAt = now
	proposal.UpdatedAt = now
	r.proposals[proposal.WorkflowID] = append([]models.WorkflowProposal{*proposal}, r.proposals[proposal.WorkflowID]...)
	return proposal, nil
}

func (r *fakeWorkflowRepo) UpdateProposal(proposal *models.WorkflowProposal) (*models.WorkflowProposal, error) {
	proposal.UpdatedAt = time.Now().UTC()
	proposals := r.proposals[proposal.WorkflowID]
	for index := range proposals {
		if proposals[index].ID == proposal.ID {
			proposals[index] = *proposal
			r.proposals[proposal.WorkflowID] = proposals
			return proposal, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeWorkflowRepo) FindProposals(workflowID uuid.UUID) ([]models.WorkflowProposal, error) {
	return append([]models.WorkflowProposal{}, r.proposals[workflowID]...), nil
}

func (r *fakeWorkflowRepo) CreateQualityGate(gate *models.WorkflowQualityGate) (*models.WorkflowQualityGate, error) {
	if gate.ID == uuid.Nil {
		gate.ID = uuid.New()
	}
	now := time.Now().UTC()
	gate.CreatedAt = now
	gate.UpdatedAt = now
	r.qualityGate[gate.WorkflowID] = append([]models.WorkflowQualityGate{*gate}, r.qualityGate[gate.WorkflowID]...)
	return gate, nil
}

func (r *fakeWorkflowRepo) UpdateQualityGate(gate *models.WorkflowQualityGate) (*models.WorkflowQualityGate, error) {
	gate.UpdatedAt = time.Now().UTC()
	gates := r.qualityGate[gate.WorkflowID]
	for index := range gates {
		if gates[index].ID == gate.ID {
			gates[index] = *gate
			r.qualityGate[gate.WorkflowID] = gates
			return gate, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeWorkflowRepo) FindQualityGates(workflowID uuid.UUID) ([]models.WorkflowQualityGate, error) {
	return append([]models.WorkflowQualityGate{}, r.qualityGate[workflowID]...), nil
}

func (r *fakeWorkflowRepo) SaveRule(rule *models.WorkflowRule) (*models.WorkflowRule, error) {
	existing, exists := r.rules[rule.RuleKey]
	if rule.ID == uuid.Nil {
		if exists {
			rule.ID = existing.ID
		} else {
			rule.ID = uuid.New()
		}
	}
	now := time.Now().UTC()
	if rule.CreatedAt.IsZero() {
		if exists {
			rule.CreatedAt = existing.CreatedAt
		} else {
			rule.CreatedAt = now
		}
	}
	rule.UpdatedAt = now
	r.rules[rule.RuleKey] = *rule
	return rule, nil
}

func (r *fakeWorkflowRepo) FindRules() ([]models.WorkflowRule, error) {
	result := []models.WorkflowRule{}
	for _, rule := range r.rules {
		result = append(result, rule)
	}
	return result, nil
}

func (r *fakeWorkflowRepo) CreateTransition(transition *models.WorkflowTransition) (*models.WorkflowTransition, error) {
	if transition.ID == uuid.Nil {
		transition.ID = uuid.New()
	}
	transition.CreatedAt = time.Now().UTC()
	r.transitions[transition.WorkflowID] = append([]models.WorkflowTransition{*transition}, r.transitions[transition.WorkflowID]...)
	return transition, nil
}

func (r *fakeWorkflowRepo) FindTransitions(workflowID uuid.UUID) ([]models.WorkflowTransition, error) {
	return append([]models.WorkflowTransition{}, r.transitions[workflowID]...), nil
}

func (r *fakeWorkflowRepo) CreateSourceLink(link *models.WorkflowSourceLink) (*models.WorkflowSourceLink, error) {
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	link.CreatedAt = time.Now().UTC()
	r.sourceLinks[link.WorkflowID] = append([]models.WorkflowSourceLink{*link}, r.sourceLinks[link.WorkflowID]...)
	return link, nil
}

func (r *fakeWorkflowRepo) FindSourceLinks(workflowID uuid.UUID) ([]models.WorkflowSourceLink, error) {
	return append([]models.WorkflowSourceLink{}, r.sourceLinks[workflowID]...), nil
}

func (r *fakeWorkflowRepo) CreateDecision(decision *models.WorkflowDecision) (*models.WorkflowDecision, error) {
	if decision.ID == uuid.Nil {
		decision.ID = uuid.New()
	}
	decision.CreatedAt = time.Now().UTC()
	r.decisions[decision.WorkflowID] = append([]models.WorkflowDecision{*decision}, r.decisions[decision.WorkflowID]...)
	return decision, nil
}

func (r *fakeWorkflowRepo) FindDecisions(workflowID uuid.UUID) ([]models.WorkflowDecision, error) {
	return append([]models.WorkflowDecision{}, r.decisions[workflowID]...), nil
}

func (r *fakeWorkflowRepo) CreateEvent(event *models.WorkflowEvent) (*models.WorkflowEvent, error) {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	event.CreatedAt = time.Now().UTC()
	r.events[event.WorkflowID] = append([]models.WorkflowEvent{*event}, r.events[event.WorkflowID]...)
	return event, nil
}

func (r *fakeWorkflowRepo) FindEvents(workflowID uuid.UUID) ([]models.WorkflowEvent, error) {
	return append([]models.WorkflowEvent{}, r.events[workflowID]...), nil
}

type fakeTaskRunner struct {
	result     *TaskRunResult
	err        error
	requests   []TaskRunRequest
	panicValue interface{}
	onRun      func(TaskRunRequest)
}

func (r *fakeTaskRunner) RunWorkflowTask(request TaskRunRequest) (*TaskRunResult, error) {
	r.requests = append(r.requests, request)
	if r.onRun != nil {
		r.onRun(request)
	}
	if r.panicValue != nil {
		panic(r.panicValue)
	}
	return r.result, r.err
}

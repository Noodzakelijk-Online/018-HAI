package task

import (
	"context"
	"errors"
	"testing"
	"time"

	"automation-hub-backend/internal/controlledlearning"
)

type recordingControlledLearning struct {
	requests []controlledlearning.RecordOutcomeRequest
	err      error
}

func (recorder *recordingControlledLearning) RecordOutcome(
	_ context.Context,
	request controlledlearning.RecordOutcomeRequest,
) (controlledlearning.OutcomeRecord, error) {
	recorder.requests = append(recorder.requests, request)
	if recorder.err != nil {
		return controlledlearning.OutcomeRecord{}, recorder.err
	}
	return controlledlearning.OutcomeRecord{
		ID:            "learning-outcome-1",
		OwnerIdentity: request.OwnerIdentity,
	}, nil
}

func TestVerifiedTaskOutcomeEntersControlledLearningLedger(t *testing.T) {
	completedAt := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	recorder := &recordingControlledLearning{}
	implementation := &service{controlledLearning: recorder}
	plan := &CompletionPlan{
		ID:               "plan-1",
		OwnerIdentity:    "alice",
		ProjectKey:       "018-hai",
		CompletionStatus: "validated",
		Intake: IntakeAnalysis{
			TaskType: "software",
		},
		ValidationResult: ValidationResult{
			Passed: true,
			Status: "passed",
			Criteria: []ValidationCriterionResult{{
				Criterion: "tests pass",
				Status:    validationCriterionPassed,
			}},
		},
		ExecutionResult: &ExecutionResult{
			CompletedAt:        completedAt,
			VerificationStatus: "test_passed",
		},
	}

	implementation.recordVerifiedLearningOutcome(plan)

	if len(recorder.requests) != 1 {
		t.Fatalf("learning requests = %d, want 1", len(recorder.requests))
	}
	request := recorder.requests[0]
	if request.OwnerIdentity != "alice" ||
		request.OperationID != "plan-1" ||
		request.Basis != controlledlearning.EvidenceVerifiedOutcome ||
		request.Status != controlledlearning.OutcomeSucceeded ||
		request.Verification != controlledlearning.VerificationTestPassed ||
		len(request.Sources) != 1 ||
		request.Sources[0].URI != "task-plan://plan-1" ||
		len(request.Criteria) != 1 ||
		!request.Criteria[0].Passed {
		t.Fatalf("unexpected controlled-learning request: %#v", request)
	}
	if len(plan.Events) != 1 ||
		plan.Events[0].Stage != "learning" {
		t.Fatalf("learning event not appended: %#v", plan.Events)
	}
}

func TestUnsupportedTaskOutputCannotBecomeLearningEvidence(t *testing.T) {
	recorder := &recordingControlledLearning{}
	implementation := &service{controlledLearning: recorder}
	plan := &CompletionPlan{
		ID:            "plan-unsupported",
		OwnerIdentity: "alice",
		ExecutionResult: &ExecutionResult{
			CompletedAt:        time.Now().UTC(),
			VerificationStatus: "unsupported",
		},
	}

	implementation.recordVerifiedLearningOutcome(plan)

	if len(recorder.requests) != 0 || len(plan.Events) != 0 {
		t.Fatalf(
			"unsupported output entered learning: requests=%#v events=%#v",
			recorder.requests,
			plan.Events,
		)
	}
}

func TestControlledLearningFailureDoesNotRewriteTaskCompletion(t *testing.T) {
	recorder := &recordingControlledLearning{err: errors.New("ledger unavailable")}
	implementation := &service{controlledLearning: recorder}
	plan := &CompletionPlan{
		ID:               "plan-failure",
		OwnerIdentity:    "alice",
		CompletionStatus: "validated",
		ValidationResult: ValidationResult{Passed: true, Status: "passed"},
		ExecutionResult: &ExecutionResult{
			CompletedAt:        time.Now().UTC(),
			VerificationStatus: "verified",
		},
	}

	implementation.recordVerifiedLearningOutcome(plan)

	if plan.CompletionStatus != "validated" ||
		len(plan.Events) != 1 ||
		plan.Events[0].Message !=
			"verified outcome could not be added to the controlled-learning ledger" {
		t.Fatalf("learning failure changed task truth: %#v", plan)
	}
}

func TestExplicitRejectedReviewRecordsHumanCorrection(t *testing.T) {
	recorder := &recordingControlledLearning{}
	implementation := &service{controlledLearning: recorder}
	resolvedAt := time.Date(2026, 7, 31, 12, 30, 0, 0, time.UTC)

	outcomeID := implementation.recordHumanCorrection(
		"alice",
		ReviewQueueItem{
			ID:     "216967e4-d62e-4a73-ae3f-c62efcbf78f5",
			TaskID: "plan-1",
			Request: IntakeRequest{
				ProjectKey: "018-hai",
			},
		},
		ReviewDecisionRecord{
			ID:             "316967e4-d62e-4a73-ae3f-c62efcbf78f5",
			Decision:       "rejected",
			ResolutionNote: "Do not send; create a formal draft for review.",
			ResolvedBy:     "alice",
			RequestDigest:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ResolvedAt:     resolvedAt,
		},
	)

	if outcomeID != "learning-outcome-1" || len(recorder.requests) != 1 {
		t.Fatalf("correction outcome = %q requests=%#v", outcomeID, recorder.requests)
	}
	request := recorder.requests[0]
	if request.Basis != controlledlearning.EvidenceHumanCorrection ||
		request.Status != controlledlearning.OutcomeCorrected ||
		!request.HumanConfirmed ||
		request.ActorIdentity != "alice" ||
		request.Correction != "Do not send; create a formal draft for review." ||
		request.Verification != controlledlearning.VerificationHumanApproved {
		t.Fatalf("unexpected correction record: %#v", request)
	}
}

func TestReviewWithoutCorrectionTextIsNotLearned(t *testing.T) {
	recorder := &recordingControlledLearning{}
	implementation := &service{controlledLearning: recorder}

	if outcomeID := implementation.recordHumanCorrection(
		"alice",
		ReviewQueueItem{ID: "review-1"},
		ReviewDecisionRecord{Decision: "rejected"},
	); outcomeID != "" || len(recorder.requests) != 0 {
		t.Fatalf("empty review became correction: %q %#v", outcomeID, recorder.requests)
	}
}

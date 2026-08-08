package workflow

import (
	"context"
	"errors"
	"testing"

	"automation-hub-backend/internal/controlledlearning"
	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

type workflowLearningRecorder struct {
	requests []controlledlearning.RecordOutcomeRequest
	err      error
}

func (recorder *workflowLearningRecorder) RecordOutcome(
	_ context.Context,
	request controlledlearning.RecordOutcomeRequest,
) (controlledlearning.OutcomeRecord, error) {
	recorder.requests = append(recorder.requests, request)
	if recorder.err != nil {
		return controlledlearning.OutcomeRecord{}, recorder.err
	}
	return controlledlearning.OutcomeRecord{
		ID:            "workflow-learning-outcome",
		OwnerIdentity: request.OwnerIdentity,
	}, nil
}

func TestWorkflowCorrectionRecorderFailureDoesNotReturnPromotableOutcome(t *testing.T) {
	recorder := &workflowLearningRecorder{err: errors.New("ledger unavailable")}
	implementation := &service{
		repo:               newFakeWorkflowRepo(),
		controlledLearning: recorder,
	}
	item := &models.WorkflowItem{
		ID:            uuid.New(),
		OwnerIdentity: "alice",
		ProjectKey:    "018-hai",
		CurrentState:  StateBlocked,
		Title:         "Review external message",
	}

	outcomeID := implementation.recordControlledCorrection(
		item,
		"approval_rejected",
		"Never send this category automatically; prepare a draft.",
		"alice",
	)

	if outcomeID != "" || len(recorder.requests) != 1 {
		t.Fatalf("failed recorder produced promotable outcome: %q %#v", outcomeID, recorder.requests)
	}
}

func TestAuthenticatedWorkflowCorrectionEntersControlledLearning(t *testing.T) {
	recorder := &workflowLearningRecorder{}
	implementation := &service{
		repo:               newFakeWorkflowRepo(),
		controlledLearning: recorder,
	}
	item := &models.WorkflowItem{
		ID:            uuid.New(),
		OwnerIdentity: "alice",
		ProjectKey:    "018-hai",
		CurrentState:  StateBlocked,
		Title:         "Review external message",
	}

	outcomeID := implementation.recordControlledCorrection(
		item,
		"approval_rejected",
		"Never send this category automatically; prepare a draft.",
		"alice",
	)

	if outcomeID != "workflow-learning-outcome" ||
		len(recorder.requests) != 1 {
		t.Fatalf("outcome=%q requests=%#v", outcomeID, recorder.requests)
	}
	request := recorder.requests[0]
	if request.OwnerIdentity != "alice" ||
		request.OperationID != item.ID.String() ||
		request.Basis != controlledlearning.EvidenceHumanCorrection ||
		request.Status != controlledlearning.OutcomeCorrected ||
		!request.HumanConfirmed ||
		request.ActorIdentity != "alice" ||
		request.Verification != controlledlearning.VerificationHumanApproved ||
		len(request.Sources) != 1 ||
		request.Sources[0].URI != "workflow://"+item.ID.String() {
		t.Fatalf("unexpected controlled correction: %#v", request)
	}
}

func TestWorkflowCorrectionRejectsUnverifiedActor(t *testing.T) {
	recorder := &workflowLearningRecorder{}
	implementation := &service{controlledLearning: recorder}
	item := &models.WorkflowItem{
		ID:            uuid.New(),
		OwnerIdentity: "alice",
	}

	outcomeID := implementation.recordControlledCorrection(
		item,
		"proposal_rejected",
		"Use a different source next time.",
		"workflow-worker",
	)

	if outcomeID != "" || len(recorder.requests) != 0 {
		t.Fatalf("unverified actor became learning evidence: %q %#v", outcomeID, recorder.requests)
	}
}

func TestWorkflowCorrectionRejectsGenericFeedback(t *testing.T) {
	recorder := &workflowLearningRecorder{}
	implementation := &service{controlledLearning: recorder}
	item := &models.WorkflowItem{
		ID:            uuid.New(),
		OwnerIdentity: "alice",
	}

	outcomeID := implementation.recordControlledCorrection(
		item,
		"proposal_rejected",
		"rejected",
		"alice",
	)

	if outcomeID != "" || len(recorder.requests) != 0 {
		t.Fatalf("generic feedback became learning evidence: %q %#v", outcomeID, recorder.requests)
	}
}

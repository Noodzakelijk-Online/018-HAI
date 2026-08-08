package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"automation-hub-backend/internal/controlledlearning"
	"automation-hub-backend/internal/models"
)

const controlledLearningTimeout = 5 * time.Second

type ControlledLearningRecorder interface {
	RecordOutcome(
		context.Context,
		controlledlearning.RecordOutcomeRequest,
	) (controlledlearning.OutcomeRecord, error)
}

// WithControlledLearning attaches the governed learning ledger without
// widening the workflow Service interface or changing protocol test doubles.
func WithControlledLearning(
	base Service,
	recorder ControlledLearningRecorder,
) Service {
	implementation, ok := base.(*service)
	if !ok || recorder == nil {
		return base
	}
	implementation.controlledLearning = recorder
	return implementation
}

func (s *service) recordControlledCorrection(
	item *models.WorkflowItem,
	signal string,
	note string,
	actor string,
) string {
	if s == nil || s.controlledLearning == nil || item == nil ||
		strings.TrimSpace(item.OwnerIdentity) == "" ||
		strings.TrimSpace(actor) != strings.TrimSpace(item.OwnerIdentity) ||
		!humanCorrectionSignal(signal) ||
		!feedbackNoteUseful(signal, note) {
		return ""
	}
	note = strings.TrimSpace(note)
	signal = strings.TrimSpace(signal)
	now := time.Now().UTC()
	sum := sha256.Sum256([]byte(strings.Join([]string{
		item.ID.String(),
		signal,
		note,
		item.CurrentState,
	}, "\x00")))
	digest := hex.EncodeToString(sum[:])
	ctx, cancel := context.WithTimeout(
		context.Background(),
		controlledLearningTimeout,
	)
	defer cancel()
	outcome, err := s.controlledLearning.RecordOutcome(
		ctx,
		controlledlearning.RecordOutcomeRequest{
			OwnerIdentity: strings.TrimSpace(item.OwnerIdentity),
			IdempotencyKey: "workflow-correction:" +
				item.ID.String() + ":" + digest[:16],
			OperationID:    item.ID.String(),
			ProjectKey:     strings.TrimSpace(item.ProjectKey),
			Basis:          controlledlearning.EvidenceHumanCorrection,
			Status:         controlledlearning.OutcomeCorrected,
			Summary:        "Human workflow review supplied a correction for future governed decisions.",
			ActorIdentity:  strings.TrimSpace(actor),
			HumanConfirmed: true,
			Correction:     note,
			Verification:   controlledlearning.VerificationHumanApproved,
			Sources: []controlledlearning.SourceReference{{
				ID:          "workflow:" + item.ID.String(),
				Kind:        "workflow_review",
				URI:         "workflow://" + item.ID.String(),
				RetrievedAt: now,
				ContentHash: digest,
			}},
			Tags:       feedbackLessonTags(*item, signal),
			OccurredAt: now,
		},
	)
	if err != nil {
		s.audit(
			item.ID,
			"workflow.controlled_learning_failed",
			item.CurrentState,
			item.CurrentState,
			"authenticated correction could not be added to the controlled-learning ledger",
			signal,
			"controlled learning",
			"workflow://"+item.ID.String(),
			actor,
		)
		return ""
	}
	s.audit(
		item.ID,
		"workflow.controlled_learning",
		item.CurrentState,
		item.CurrentState,
		"authenticated correction recorded for governed learning review",
		signal,
		"controlled learning",
		"controlled-learning://outcomes/"+outcome.ID,
		actor,
	)
	return outcome.ID
}

func humanCorrectionSignal(signal string) bool {
	switch strings.TrimSpace(signal) {
	case "approval_approved",
		"approval_rejected",
		"proposal_changes_requested",
		"proposal_rejected",
		"interruption_retry",
		"interruption_keep_blocked",
		"interruption_confirm_completed",
		"checklist_open",
		"checklist_done",
		"checklist_blocked":
		return true
	default:
		return false
	}
}

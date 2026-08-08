package task

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/frameworkevidence"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/verification"
)

const frameworkEvidenceRepositoryTimeout = 5 * time.Second

// WithFrameworkEvidenceRepository installs the durable, owner-scoped preflight
// ledger shared with execution authorization. Production composition roots
// must provide the PostgreSQL repository; constructors keep an in-memory
// implementation for isolated tests and side-effect-free previews.
func WithFrameworkEvidenceRepository(
	base Service,
	repository frameworkevidence.Repository,
) (Service, error) {
	implementation, ok := base.(*service)
	if !ok {
		return nil, fmt.Errorf("framework evidence persistence requires the built-in task service")
	}
	if repository == nil {
		return nil, fmt.Errorf("framework evidence repository is required")
	}
	implementation.frameworkEvidence = repository
	return implementation, nil
}

func (s *service) persistFrameworkEvidencePreflight(plan *CompletionPlan) error {
	if s == nil || s.frameworkEvidence == nil {
		return fmt.Errorf("durable framework evidence repository is unavailable")
	}
	if plan == nil || plan.FrameworkDecision == nil || plan.FrameworkEvidencePreflight == nil {
		return fmt.Errorf("framework evidence preflight record is incomplete")
	}
	preflight := plan.FrameworkEvidencePreflight
	if !preflight.Passed || !strings.EqualFold(strings.TrimSpace(preflight.Status), "passed") {
		return fmt.Errorf("only a passing framework evidence preflight can be persisted")
	}
	assertions, err := json.Marshal(preflight.Assertions)
	if err != nil {
		return fmt.Errorf("encode framework evidence assertions: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), frameworkEvidenceRepositoryTimeout)
	defer cancel()
	return s.frameworkEvidence.Store(ctx, frameworkevidence.Record{
		OwnerIdentity:        strings.TrimSpace(plan.OwnerIdentity),
		TaskPlanID:           strings.TrimSpace(plan.ID),
		FrameworkSelectionID: strings.TrimSpace(plan.FrameworkDecision.ID),
		PreflightDigest:      strings.TrimSpace(preflight.Digest),
		Status:               frameworkevidence.StatusPassed,
		AssertionsJSON:       assertions,
		EvaluatedAt:          preflight.EvaluatedAt.UTC(),
	})
}

func frameworkEvidenceDurabilityRequired(plan *CompletionPlan) bool {
	return plan != nil && (plan.Intake.NeedsTools || plan.Intake.NeedsLocalExecution)
}

func frameworkEvidencePersistenceBlockedExecution(
	plan *CompletionPlan,
	err error,
) *ExecutionResult {
	now := time.Now().UTC()
	reason := "framework evidence preflight could not be persisted before execution"
	if err != nil {
		reason += ": " + strings.TrimSpace(err.Error())
	}
	plan.Events = append(plan.Events, event("framework-evidence-persistence", reason))
	return &ExecutionResult{
		StartedAt: now, CompletedAt: now, Mode: "blocked",
		Output:             "Execution was blocked before any external effect.",
		VerificationStatus: verification.StatusNeedsReview,
		Claims:             []models.VerificationClaim{},
		Actions: []ExecutedAction{executedAction(
			"governance.framework_evidence_persistence",
			"blocked",
			plan.Request,
			reason,
			now,
		)},
		BlockedReason: reason,
	}
}

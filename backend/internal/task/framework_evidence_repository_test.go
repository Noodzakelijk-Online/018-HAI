package task

import (
	"context"
	"errors"
	"strings"
	"testing"

	"automation-hub-backend/internal/frameworkevidence"
	"automation-hub-backend/internal/frameworkregistry"
)

type failingFrameworkEvidenceRepository struct {
	storeCalls int
}

func (r *failingFrameworkEvidenceRepository) Store(
	context.Context,
	frameworkevidence.Record,
) error {
	r.storeCalls++
	return errors.New("preflight ledger unavailable")
}

func (r *failingFrameworkEvidenceRepository) Resolve(
	context.Context,
	string,
	string,
	string,
	string,
) (frameworkevidence.Record, error) {
	return frameworkevidence.Record{}, frameworkevidence.ErrNotFound
}

func TestFrameworkEvidencePersistenceFailureBlocksBeforeExecutor(t *testing.T) {
	executor := &fakeToolExecutor{result: completedToolResult()}
	base := frameworkEvidenceRunService(t, executor, frameworkregistry.SelectionDecision{
		ID:                   "owner-identity-selection",
		MaximumAutonomyLevel: 10,
		Selected: []frameworkregistry.SelectedFramework{{
			ID:                   "human-sovereignty",
			Version:              "1.0.0",
			EvidenceRequirements: []string{"verified operator identity"},
		}},
		EvidenceRequirements: []string{"verified operator identity"},
		ConstitutionVersion:  1,
	})
	repository := &failingFrameworkEvidenceRepository{}
	configured, err := WithFrameworkEvidenceRepository(base, repository)
	if err != nil {
		t.Fatalf("WithFrameworkEvidenceRepository: %v", err)
	}

	plan, err := configured.Run(IntakeRequest{
		OwnerIdentity:  "alice",
		Request:        "Run local script tests for the project",
		ProjectKey:     "018-HAI",
		AutomationID:   executor.result.AutomationID,
		ExecuteAllowed: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if repository.storeCalls != 1 {
		t.Fatalf("preflight store calls = %d, want 1", repository.storeCalls)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want zero after preflight persistence failure", executor.calls)
	}
	if plan.FrameworkEvidencePreflight == nil || !plan.FrameworkEvidencePreflight.Passed {
		t.Fatalf("preflight should have passed before persistence failed: %#v", plan.FrameworkEvidencePreflight)
	}
	if plan.ExecutionResult == nil || !strings.Contains(
		plan.ExecutionResult.BlockedReason,
		"preflight ledger unavailable",
	) {
		t.Fatalf("persistence failure is not visible in blocked result: %#v", plan.ExecutionResult)
	}
}

func TestPassingFrameworkEvidenceIsResolvableBeforeSingleExecutorCall(t *testing.T) {
	executor := &fakeToolExecutor{result: completedToolResult()}
	base := frameworkEvidenceRunService(t, executor, frameworkregistry.SelectionDecision{
		ID:                   "durable-selection",
		MaximumAutonomyLevel: 10,
		Selected: []frameworkregistry.SelectedFramework{{
			ID:                   "human-sovereignty",
			Version:              "1.0.0",
			EvidenceRequirements: []string{"verified operator identity"},
		}},
		EvidenceRequirements: []string{"verified operator identity"},
		ConstitutionVersion:  1,
	})
	repository := frameworkevidence.NewMemoryRepository()
	configured, err := WithFrameworkEvidenceRepository(base, repository)
	if err != nil {
		t.Fatalf("WithFrameworkEvidenceRepository: %v", err)
	}

	plan, err := configured.Run(IntakeRequest{
		OwnerIdentity:  "alice",
		Request:        "Run local script tests for the project",
		ProjectKey:     "018-HAI",
		AutomationID:   executor.result.AutomationID,
		ExecuteAllowed: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want exactly one", executor.calls)
	}
	if plan.FrameworkEvidencePreflight == nil || !plan.FrameworkEvidencePreflight.Passed {
		t.Fatalf("preflight did not pass: %#v", plan.FrameworkEvidencePreflight)
	}
	resolved, err := repository.Resolve(
		context.Background(),
		plan.OwnerIdentity,
		plan.ID,
		plan.FrameworkDecision.ID,
		plan.FrameworkEvidencePreflight.Digest,
	)
	if err != nil {
		t.Fatalf("resolve persisted preflight: %v", err)
	}
	if resolved.Status != frameworkevidence.StatusPassed ||
		resolved.PreflightDigest != plan.FrameworkEvidencePreflight.Digest {
		t.Fatalf("resolved preflight = %#v", resolved)
	}
}

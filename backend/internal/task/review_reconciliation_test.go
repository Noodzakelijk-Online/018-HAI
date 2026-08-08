package task

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestApprovedReviewReconciliationUsesDurableEvidenceWithoutRetry(t *testing.T) {
	repository := NewMemoryTaskStateRepository()
	service := NewServiceWithDependencies(nil, nil, nil, nil, nil, nil, nil, repository).(*service)
	owner := "alice"
	now := time.Now().UTC().Truncate(time.Microsecond)

	completedReview := createApprovedReconciliationReview(t, repository, owner, now.Add(-3*time.Hour))
	indeterminateReview := createApprovedReconciliationReview(t, repository, owner, now.Add(-2*time.Hour))
	recentReview := createApprovedReconciliationReview(t, repository, owner, now.Add(-2*time.Minute))

	if err := repository.AppendCompletionPlan(owner, CompletionPlan{
		ID:               "completed-attempt",
		OwnerIdentity:    owner,
		ReviewItemID:     completedReview.ID,
		CreatedAt:        now.Add(-2 * time.Hour),
		Request:          "perform the approved read-only check",
		CompletionStatus: "validated",
		ValidationResult: ValidationResult{Passed: true, Status: "test_passed"},
		ExecutionResult:  &ExecutionResult{VerificationStatus: "test_passed"},
	}); err != nil {
		t.Fatalf("append completion evidence: %v", err)
	}

	preview, err := service.ReconcileApprovedReviewsForOwner(owner, ApprovedReviewReconciliationRequest{
		OlderThanMinutes: 30,
		Limit:            20,
	})
	if err != nil {
		t.Fatalf("preview reconciliation: %v", err)
	}
	if !preview.DryRun || preview.ApprovedFound != 3 || preview.Eligible != 2 ||
		preview.Completed != 1 || preview.ReturnedToReview != 1 || len(preview.Items) != 2 {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	for _, review := range []ReviewQueueItem{completedReview, indeterminateReview, recentReview} {
		stored, findErr := repository.FindReviewItem(owner, review.ID)
		if findErr != nil || stored.Status != "approved" {
			t.Fatalf("dry run mutated review %s: %#v, %v", review.ID, stored, findErr)
		}
	}

	applied, err := service.ReconcileApprovedReviewsForOwner(owner, ApprovedReviewReconciliationRequest{
		Apply:            true,
		Confirmation:     approvedReviewReconciliationConfirm,
		OlderThanMinutes: 30,
		Limit:            20,
	})
	if err != nil {
		t.Fatalf("apply reconciliation: %v", err)
	}
	if applied.DryRun || applied.Completed != 1 || applied.ReturnedToReview != 1 || applied.Conflicts != 0 {
		t.Fatalf("unexpected applied result: %#v", applied)
	}
	completed, _ := repository.FindReviewItem(owner, completedReview.ID)
	if completed.Status != "completed" || completed.TaskID != "completed-attempt" {
		t.Fatalf("verified review not completed from evidence: %#v", completed)
	}
	indeterminate, _ := repository.FindReviewItem(owner, indeterminateReview.ID)
	if indeterminate.Status != "needs_review" || indeterminate.Decision != "" || indeterminate.ResolvedAt != nil {
		t.Fatalf("indeterminate review did not return to review: %#v", indeterminate)
	}
	recent, _ := repository.FindReviewItem(owner, recentReview.ID)
	if recent.Status != "approved" {
		t.Fatalf("recent approval was reconciled before cutoff: %#v", recent)
	}
}

func TestApprovedReviewReconciliationRequiresExactConfirmationAndBoundedAge(t *testing.T) {
	service := NewServiceWithDependencies(nil, nil, nil, nil, nil, nil, nil, NewMemoryTaskStateRepository()).(*service)
	if _, err := service.ReconcileApprovedReviewsForOwner("alice", ApprovedReviewReconciliationRequest{Apply: true}); err == nil {
		t.Fatal("apply reconciliation accepted without exact confirmation")
	}
	if _, err := service.ReconcileApprovedReviewsForOwner("alice", ApprovedReviewReconciliationRequest{OlderThanMinutes: 1}); err == nil {
		t.Fatal("reconciliation accepted an unsafe age threshold")
	}
}

func TestCompletionPlanPersistsReviewReconciliationLink(t *testing.T) {
	repository := NewMemoryTaskStateRepository()
	reviewID := uuid.NewString()
	plan := CompletionPlan{
		ID: "linked-plan", OwnerIdentity: "alice", ReviewItemID: reviewID,
		CreatedAt: time.Now().UTC(), CompletionStatus: "planned",
	}
	if err := repository.AppendCompletionPlan("alice", plan); err != nil {
		t.Fatalf("append linked plan: %v", err)
	}
	stored, err := repository.FindCompletionPlan("alice", plan.ID)
	if err != nil || stored.ReviewItemID != reviewID {
		t.Fatalf("review link did not round-trip: %#v, %v", stored, err)
	}
	plan.ReviewItemID = "not-a-uuid"
	plan.ID = "invalid-link"
	if err := repository.AppendCompletionPlan("alice", plan); err == nil {
		t.Fatal("invalid review link was persisted")
	}
}

func createApprovedReconciliationReview(t *testing.T, repository TaskStateRepository, owner string, createdAt time.Time) ReviewQueueItem {
	t.Helper()
	createdAt = createdAt.UTC().Truncate(time.Microsecond)
	item := ReviewQueueItem{
		ID: uuid.NewString(), TaskID: "original-" + uuid.NewString(),
		Request: IntakeRequest{OwnerIdentity: owner, Request: "approved action"},
		Reason:  "approval required", Priority: "normal", Status: "open", CreatedAt: createdAt,
	}
	created, err := repository.CreateReviewItem(owner, item)
	if err != nil {
		t.Fatalf("create review: %v", err)
	}
	resolved, err := repository.ResolveReviewItem(owner, created.ID, ReviewResolution{
		Decision: "approved", Note: "approved for bounded execution", ResolvedAt: createdAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("approve review: %v", err)
	}
	return resolved.Item
}

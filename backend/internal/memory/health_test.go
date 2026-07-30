package memory

import (
	"testing"
	"time"

	"automation-hub-backend/internal/models"
)

func TestMemoryHealthIsOwnerScopedReadOnlyAndFindsReviewCandidates(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo)
	first, err := repo.Create(&models.ContextMemory{
		OwnerIdentity: "robert", ProjectKey: "vivare", Kind: "preference", Confidence: 0.9,
		Content: "Use formal Dutch legal tone for Vivare correspondence and attach evidence.",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := repo.Create(&models.ContextMemory{
		OwnerIdentity: "robert", ProjectKey: "vivare", Kind: "preference", Confidence: 0.8,
		Content: "Use formal Dutch legal tone for Vivare correspondence and attach source evidence.",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if _, err := repo.Create(&models.ContextMemory{
		OwnerIdentity: "other", ProjectKey: "vivare", Kind: "preference", Content: "Private other-owner memory.",
	}); err != nil {
		t.Fatalf("create other: %v", err)
	}
	stale := time.Now().UTC().Add(-91 * 24 * time.Hour)
	stored := repo.memories[first.ID]
	stored.CreatedAt, stored.UpdatedAt = stale, stale
	repo.memories[first.ID] = stored

	report, err := HealthForOwner(service, "robert", "vivare")
	if err != nil {
		t.Fatalf("HealthForOwner: %v", err)
	}
	if report.Active != 2 || report.PossibleDuplicatePairs != 1 || len(report.ConsolidationCandidates) != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	candidate := report.ConsolidationCandidates[0]
	correctPair := (candidate.FirstID == first.ID && candidate.SecondID == second.ID) || (candidate.FirstID == second.ID && candidate.SecondID == first.ID)
	if !correctPair || report.NeedsSourceReview != 2 || report.HighConfidenceUngrounded != 2 || report.Stale != 1 {
		t.Fatalf("unexpected scoped health result: %#v", report)
	}
	if len(repo.memories) != 3 {
		t.Fatal("health review mutated memory")
	}
}

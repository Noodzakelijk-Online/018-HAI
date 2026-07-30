package task

import (
	"testing"
	"time"

	"automation-hub-backend/internal/frameworkregistry"
	"automation-hub-backend/internal/lifeops"

	"github.com/google/uuid"
)

func TestLifeOpsContextProviderReturnsLatestNonExpiredNeedPerDomain(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	repository := lifeops.NewMemoryRepository()
	service := lifeops.NewService(repository, lifeops.WithClock(func() time.Time { return now }))
	expiredAt := now.Add(-time.Minute)
	for _, observation := range []lifeops.NeedObservation{
		{
			ID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), OwnerIdentity: "alice",
			DomainID: lifeops.DomainFinancial, NeedLevel: "stability", State: "active",
			Priority: 80, Confidence: .9, Evidence: []string{"current budget"},
			SourceLabel: "operator", SourceURI: "memory://budget/current",
			ObservedAt: now, CreatedAt: now,
		},
		{
			ID: uuid.MustParse("00000000-0000-0000-0000-000000000002"), OwnerIdentity: "alice",
			DomainID: lifeops.DomainFinancial, NeedLevel: "stability", State: "old",
			Priority: 10, Confidence: .2, ObservedAt: now.Add(-time.Hour),
			CreatedAt: now.Add(-time.Hour),
		},
		{
			ID: uuid.MustParse("00000000-0000-0000-0000-000000000003"), OwnerIdentity: "alice",
			DomainID: lifeops.DomainHealthWellbeing, NeedLevel: "wellbeing", State: "expired",
			Priority: 100, Confidence: 1, ObservedAt: now.Add(-2 * time.Hour),
			ExpiresAt: &expiredAt, CreatedAt: now.Add(-2 * time.Hour),
		},
	} {
		if err := repository.SaveNeedObservation(observation); err != nil {
			t.Fatalf("SaveNeedObservation: %v", err)
		}
	}

	provider := NewLifeOpsContextProvider(service)
	needs, err := provider.LatestNeeds("alice", now)
	if err != nil {
		t.Fatalf("LatestNeeds: %v", err)
	}
	if len(needs) != 1 {
		t.Fatalf("needs = %#v, want one current domain observation", needs)
	}
	if needs[0].DomainID != string(lifeops.DomainFinancial) || needs[0].State != "active" {
		t.Fatalf("need = %#v", needs[0])
	}
	if len(needs[0].Evidence) != 2 || needs[0].Evidence[1] != "memory://budget/current" {
		t.Fatalf("evidence provenance = %#v", needs[0].Evidence)
	}
}

func TestLifeOpsContextProviderReturnsExplicitUnknownCapacity(t *testing.T) {
	provider := NewLifeOpsContextProvider(lifeops.NewService(nil))
	capacity, err := provider.LatestCapacity("alice", time.Now().UTC())
	if err != nil {
		t.Fatalf("LatestCapacity: %v", err)
	}
	if capacity.Status != lifeops.CapacityUnknown || !capacity.NeedsReview || capacity.Fresh {
		t.Fatalf("capacity = %#v", capacity)
	}
	if capacity.PlanningStepLimit != 1 || len(capacity.Constraints) == 0 {
		t.Fatalf("unknown capacity is not fail-closed: %#v", capacity)
	}
}

func TestLifeOpsContextProviderMarksAgedCapacityStaleAtReadTime(t *testing.T) {
	capturedAt := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	repository := lifeops.NewMemoryRepository()
	service := lifeops.NewService(repository)
	if err := repository.SaveCapacitySnapshot(lifeops.CapacitySnapshot{
		ID: uuid.MustParse("00000000-0000-0000-0000-000000000004"), OwnerIdentity: "alice",
		Status: lifeops.CapacityAvailable, Signals: lifeops.CapacitySignals{Energy: 80},
		PlanningStepLimit: 5, SourceLabel: "operator", CapturedAt: capturedAt,
		Confidence: .9, Fresh: true, CreatedAt: capturedAt,
	}); err != nil {
		t.Fatalf("SaveCapacitySnapshot: %v", err)
	}
	provider := NewLifeOpsContextProvider(service)
	capacity, err := provider.LatestCapacity("alice", capturedAt.Add(25*time.Hour))
	if err != nil {
		t.Fatalf("LatestCapacity: %v", err)
	}
	if capacity.Fresh || !capacity.NeedsReview {
		t.Fatalf("aged capacity = %#v", capacity)
	}
}

func TestLifeOpsContextProviderRecordsTaskDomainProvenance(t *testing.T) {
	repository := lifeops.NewMemoryRepository()
	service := lifeops.NewService(repository)
	provider := NewLifeOpsContextProvider(service)
	err := provider.RecordTaskDomains(
		"alice",
		"plan-1",
		[]frameworkregistry.LifeDomainAssignment{{
			ID: string(lifeops.DomainLegalGovernment), Need: "legal obligation",
			Confidence: .9, Signals: []string{"hearing"}, Primary: true,
		}},
		"selection-1",
	)
	if err != nil {
		t.Fatalf("RecordTaskDomains: %v", err)
	}
	links, err := service.EntityDomains("alice", "task_plan", "plan-1")
	if err != nil {
		t.Fatalf("EntityDomains: %v", err)
	}
	if len(links) != 1 || links[0].DomainID != lifeops.DomainLegalGovernment || !links[0].Primary {
		t.Fatalf("links = %#v", links)
	}
	if links[0].SourceURI != "framework-selection://selection-1" ||
		links[0].VerificationStatus != "needs_review" {
		t.Fatalf("provenance = %#v", links[0])
	}
}

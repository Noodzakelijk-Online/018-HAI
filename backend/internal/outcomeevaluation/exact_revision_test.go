package outcomeevaluation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestServiceResolvesExactHistoricalOutcomeRevision(t *testing.T) {
	now := time.Date(2026, time.February, 2, 12, 0, 0, 0, time.UTC)
	repository, err := NewMemoryRepositoryWithLimits(HistoryLimits{
		OutcomeRevisions: 1,
		Evaluations:      1,
		Corrections:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := newService(repository, func() time.Time { return now })
	outcome := validRequest().Outcome

	first, _, err := service.StoreOutcome(context.Background(), "owner-1", "workspace-1", "outcome-1", StoreOutcomeRequest{
		IdempotencyKey: "exact-revision-1", ExpectedRevision: 0, Outcome: outcome,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome.Statement = "Second immutable outcome definition."
	second, _, err := service.StoreOutcome(context.Background(), "owner-1", "workspace-1", "outcome-1", StoreOutcomeRequest{
		IdempotencyKey: "exact-revision-2", ExpectedRevision: 1, Outcome: outcome,
	})
	if err != nil {
		t.Fatal(err)
	}

	history, err := service.OutcomeHistory(context.Background(), "owner-1", "workspace-1", "outcome-1")
	if err != nil || len(history) != 1 || history[0].Revision != 2 {
		t.Fatalf("bounded history = %#v, err %v", history, err)
	}
	resolved, err := service.ResolveOutcomeRevision(context.Background(), "owner-1", "workspace-1", "outcome-1", first.Revision, first.AuditDigest)
	if err != nil {
		t.Fatalf("ResolveOutcomeRevision(first) error = %v", err)
	}
	if resolved.Revision != first.Revision || resolved.AuditDigest != first.AuditDigest || resolved.Outcome.Statement != first.Outcome.Statement {
		t.Fatalf("resolved first revision = %#v, want %#v", resolved, first)
	}
	if err := VerifyOutcomeRevisionDigest(resolved); err != nil {
		t.Fatalf("resolved revision digest = %v", err)
	}

	selectors := []struct {
		name        string
		ownerID     string
		workspaceID string
		outcomeID   string
		revision    int64
		digest      string
	}{
		{name: "wrong digest", ownerID: "owner-1", workspaceID: "workspace-1", outcomeID: "outcome-1", revision: first.Revision, digest: second.AuditDigest},
		{name: "missing revision does not fall back", ownerID: "owner-1", workspaceID: "workspace-1", outcomeID: "outcome-1", revision: 99, digest: second.AuditDigest},
		{name: "wrong owner", ownerID: "other-owner", workspaceID: "workspace-1", outcomeID: "outcome-1", revision: first.Revision, digest: first.AuditDigest},
		{name: "wrong workspace", ownerID: "owner-1", workspaceID: "other-workspace", outcomeID: "outcome-1", revision: first.Revision, digest: first.AuditDigest},
		{name: "wrong outcome", ownerID: "owner-1", workspaceID: "workspace-1", outcomeID: "other-outcome", revision: first.Revision, digest: first.AuditDigest},
	}
	for _, selector := range selectors {
		t.Run(selector.name, func(t *testing.T) {
			_, resolveErr := service.ResolveOutcomeRevision(context.Background(), selector.ownerID, selector.workspaceID, selector.outcomeID, selector.revision, selector.digest)
			if !errors.Is(resolveErr, ErrNotFound) {
				t.Fatalf("ResolveOutcomeRevision() error = %v, want ErrNotFound", resolveErr)
			}
		})
	}
}

func TestResolveOutcomeRevisionRejectsInvalidSelectorAndCorruption(t *testing.T) {
	now := time.Date(2026, time.February, 2, 12, 0, 0, 0, time.UTC)
	repository := NewMemoryRepository()
	service := newService(repository, func() time.Time { return now })
	stored, _, err := service.StoreOutcome(context.Background(), "owner-1", "workspace-1", "outcome-1", StoreOutcomeRequest{
		IdempotencyKey: "exact-selector", ExpectedRevision: 0, Outcome: validRequest().Outcome,
	})
	if err != nil {
		t.Fatal(err)
	}

	invalidSelectors := []struct {
		name     string
		revision int64
		digest   string
	}{
		{name: "zero revision", revision: 0, digest: stored.AuditDigest},
		{name: "negative revision", revision: -1, digest: stored.AuditDigest},
		{name: "short digest", revision: 1, digest: "abc"},
		{name: "uppercase digest", revision: 1, digest: strings.ToUpper(stored.AuditDigest)},
		{name: "padded digest", revision: 1, digest: " " + stored.AuditDigest},
	}
	for _, selector := range invalidSelectors {
		t.Run(selector.name, func(t *testing.T) {
			_, resolveErr := service.ResolveOutcomeRevision(context.Background(), "owner-1", "workspace-1", "outcome-1", selector.revision, selector.digest)
			if !errors.Is(resolveErr, ErrInvalidInput) {
				t.Fatalf("ResolveOutcomeRevision() error = %v, want ErrInvalidInput", resolveErr)
			}
		})
	}

	key := repositoryKey{ownerID: "owner-1", workspaceID: "workspace-1", outcomeID: "outcome-1"}
	repository.mu.Lock()
	tampered := repository.data[key].exactRevisions[stored.Revision]
	tampered.Outcome.Statement = "tampered historical definition"
	repository.data[key].exactRevisions[stored.Revision] = tampered
	repository.mu.Unlock()
	if _, err := service.ResolveOutcomeRevision(context.Background(), "owner-1", "workspace-1", "outcome-1", stored.Revision, stored.AuditDigest); !errors.Is(err, ErrIntegrityViolation) {
		t.Fatalf("tampered exact revision error = %v, want ErrIntegrityViolation", err)
	}
}

func TestResolveOutcomeRevisionRequiresRepositoryCapability(t *testing.T) {
	repository := NewMemoryRepository()
	service := newService(&currentOutcomeOnlyRepository{Repository: repository}, time.Now)
	_, err := service.ResolveOutcomeRevision(context.Background(), "owner-1", "workspace-1", "outcome-1", 1, strings.Repeat("a", 64))
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("missing exact resolver capability error = %v", err)
	}
}

type currentOutcomeOnlyRepository struct {
	Repository
}

package executionauth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"automation-hub-backend/internal/frameworkevidence"
)

func TestFrameworkEvidencePreflightRepositoryAdapterResolvesExactRecord(t *testing.T) {
	repository := frameworkevidence.NewMemoryRepository()
	record := frameworkevidence.Record{
		ContractVersion:      frameworkevidence.ContractVersion,
		OwnerIdentity:        "alice",
		TaskPlanID:           "plan-1",
		FrameworkSelectionID: "selection-1",
		Status:               frameworkevidence.StatusPassed,
		AssertionsJSON:       json.RawMessage(`[{"requirementId":"requirement-1","frameworkId":"framework-1","phase":"pre_authorization","validator":"deterministic_check","status":"verified","evidence":["source_supported"]}]`),
		EvaluatedAt:          time.Date(2026, 8, 4, 8, 0, 0, 0, time.UTC),
	}
	digest, err := frameworkevidence.PreflightDigest(
		record.OwnerIdentity,
		record.TaskPlanID,
		record.FrameworkSelectionID,
		record.EvaluatedAt,
		record.AssertionsJSON,
	)
	if err != nil {
		t.Fatalf("PreflightDigest: %v", err)
	}
	record.PreflightDigest = digest
	if err := repository.Store(context.Background(), record); err != nil {
		t.Fatalf("Store: %v", err)
	}
	resolver, err := NewFrameworkEvidencePreflightResolver(repository)
	if err != nil {
		t.Fatalf("NewFrameworkEvidencePreflightResolver: %v", err)
	}

	snapshot, err := resolver.ResolveFrameworkEvidencePreflight(
		context.Background(),
		"alice",
		"plan-1",
		"selection-1",
		digest,
	)
	if err != nil {
		t.Fatalf("ResolveFrameworkEvidencePreflight: %v", err)
	}
	if snapshot.OwnerIdentity != record.OwnerIdentity ||
		snapshot.TaskPlanID != record.TaskPlanID ||
		snapshot.FrameworkSelectionID != record.FrameworkSelectionID ||
		snapshot.PreflightDigest != record.PreflightDigest ||
		snapshot.Status != frameworkevidence.StatusPassed {
		t.Fatalf("snapshot = %#v", snapshot)
	}

	_, err = resolver.ResolveFrameworkEvidencePreflight(
		context.Background(),
		"bob",
		"plan-1",
		"selection-1",
		digest,
	)
	if !errors.Is(err, frameworkevidence.ErrNotFound) {
		t.Fatalf("foreign-owner resolution error = %v, want ErrNotFound", err)
	}
}

func TestNewFrameworkEvidencePreflightResolverRejectsNilRepository(t *testing.T) {
	if _, err := NewFrameworkEvidencePreflightResolver(nil); err == nil {
		t.Fatal("NewFrameworkEvidencePreflightResolver accepted nil")
	}
}

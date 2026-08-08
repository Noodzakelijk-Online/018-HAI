package modelintelligence

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// memTelemetryRepo is an in-memory TelemetryRepository for durability tests.
type memTelemetryRepo struct{ rows []ModelRunTelemetry }

func (m *memTelemetryRepo) Save(t ModelRunTelemetry) error { m.rows = append(m.rows, t); return nil }
func (m *memTelemetryRepo) LoadAll() ([]ModelRunTelemetry, error) {
	out := make([]ModelRunTelemetry, len(m.rows))
	copy(out, m.rows)
	return out, nil
}
func (m *memTelemetryRepo) UpdateValidation(id string, status ValidationStatus, method string) error {
	for index := range m.rows {
		if m.rows[index].ID == id {
			m.rows[index].ValidationStatus = status
			m.rows[index].ValidationMethod = method
			return nil
		}
	}
	return fmt.Errorf("telemetry %s not found", id)
}

func TestTelemetryIsDurableAcrossRestart(t *testing.T) {
	repo := &memTelemetryRepo{}

	// First "process": a service persisting to the repo records a triage call.
	s1 := NewService(NewRegistryFromEnv()).WithTelemetryRepository(repo)
	s1.now = func() time.Time { return time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC) }
	if _, err := s1.Triage(context.Background(), "review_invoice", "Pay invoice", "pay the rent invoice", true, false, "op-1"); err != nil {
		t.Fatalf("triage: %v", err)
	}
	if len(s1.Telemetry()) == 0 {
		t.Fatalf("first service must record telemetry")
	}
	if len(repo.rows) == 0 {
		t.Fatalf("telemetry must be persisted to the durable repository")
	}
	if repo.rows[0].ValidationStatus != ValidationSchemaValidated || repo.rows[0].ValidationMethod != "triage_schema_v1" {
		t.Fatalf("triage validation evidence must be durable: %#v", repo.rows[0])
	}

	// Second "process" (restart): a fresh service seeded from the same repo must
	// already have the prior telemetry — proving durability.
	s2 := NewService(NewRegistryFromEnv()).WithTelemetryRepository(repo)
	if len(s2.Telemetry()) != len(repo.rows) {
		t.Fatalf("telemetry must survive restart: got %d, persisted %d", len(s2.Telemetry()), len(repo.rows))
	}
	if len(s2.LaneWinners()) == 0 {
		t.Fatalf("seeded telemetry must produce lane winners after restart")
	}
	profiles := s2.Profiles()
	foundObserved := false
	for _, profile := range profiles {
		if profile.ProviderID == ProviderTestFastTriage && profile.ObservedRuns > 0 {
			foundObserved = true
		}
	}
	if !foundObserved {
		t.Fatal("durable telemetry must rebuild observed profile metrics after restart")
	}
}

func TestCalibrationRefreshesRowsWrittenByAnotherEngine(t *testing.T) {
	repo := &memTelemetryRepo{}
	service := NewService(NewRegistryFromEnv()).WithTelemetryRepository(repo)
	repo.rows = append(repo.rows, ModelRunTelemetry{
		ID: "llm-generation:external", ProviderID: ProviderTestFastTriage,
		ModelID: "triage-rules-v1", Lane: LaneFastTriage, OK: true,
		ValidationStatus: ValidationUnvalidated, CreatedAt: time.Now().UTC(),
	})
	if got := service.Calibration().UnvalidatedRuns; got != 1 {
		t.Fatalf("calibration did not refresh external durable row: got %d", got)
	}
	if err := repo.UpdateValidation("llm-generation:external", ValidationSourceSupported, "task_success_criteria_v1"); err != nil {
		t.Fatal(err)
	}
	summary := service.Calibration()
	if summary.AcceptedOutputs != 1 || summary.UnvalidatedRuns != 0 {
		t.Fatalf("calibration did not refresh external validation: %#v", summary)
	}
}

func TestTelemetryStorePreservesCallerAssignedID(t *testing.T) {
	store := NewTelemetryStore()
	recorded := store.Record(ModelRunTelemetry{ID: "external-id", ValidationStatus: ValidationUnvalidated})
	if recorded.ID != "external-id" {
		t.Fatalf("record id = %q, want caller-assigned id", recorded.ID)
	}
}

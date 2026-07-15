package modelintelligence

import (
	"context"
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

	// Second "process" (restart): a fresh service seeded from the same repo must
	// already have the prior telemetry — proving durability.
	s2 := NewService(NewRegistryFromEnv()).WithTelemetryRepository(repo)
	if len(s2.Telemetry()) != len(repo.rows) {
		t.Fatalf("telemetry must survive restart: got %d, persisted %d", len(s2.Telemetry()), len(repo.rows))
	}
	if len(s2.LaneWinners()) == 0 {
		t.Fatalf("seeded telemetry must produce lane winners after restart")
	}
}

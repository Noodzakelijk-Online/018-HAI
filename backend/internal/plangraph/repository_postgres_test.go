package plangraph

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPostgresRowRoundTripVerifiesCanonicalDigest(t *testing.T) {
	service, _ := newTestService()
	plan, err := service.Preview(t.Context(), "owner-a", previewRequest("row-round-trip"))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	row, err := planToRow(*plan)
	if err != nil {
		t.Fatalf("planToRow: %v", err)
	}
	// PostgreSQL stores timestamptz at microsecond precision.
	row.CreatedAt = row.CreatedAt.Truncate(1000)
	roundTrip, err := rowToPlan(row)
	if err != nil {
		t.Fatalf("rowToPlan: %v", err)
	}
	if roundTrip.OwnerIdentity != plan.OwnerIdentity || roundTrip.Digest != plan.Digest || roundTrip.Revision != plan.Revision {
		t.Fatalf("round trip mismatch: got %+v want %+v", roundTrip, plan)
	}

	var payload map[string]any
	if err := json.Unmarshal(row.PayloadJSON, &payload); err != nil {
		t.Fatalf("decode test payload: %v", err)
	}
	payload["title"] = "tampered"
	row.PayloadJSON, err = json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode test payload: %v", err)
	}
	if _, err := rowToPlan(row); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
}

func TestRepositoriesRejectInvalidOrTamperedRevisions(t *testing.T) {
	service, _ := newTestService()
	plan, err := service.Preview(t.Context(), "owner-a", previewRequest("tamper-source"))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	tampered := clonePlan(*plan)
	tampered.Title = "tampered without rehash"
	if err := NewMemoryRepository().CreateRevision(t.Context(), tampered, 0); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("memory repository accepted tampered plan: %v", err)
	}
	if _, err := planToRow(tampered); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("PostgreSQL conversion accepted tampered plan: %v", err)
	}
}

func TestPostgresRowRejectsMetadataAndAuthorityTampering(t *testing.T) {
	service, _ := newTestService()
	plan, err := service.Preview(t.Context(), "owner-a", previewRequest("row-metadata-tamper"))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	row, err := planToRow(*plan)
	if err != nil {
		t.Fatalf("planToRow: %v", err)
	}
	row.CreatedBy = "different-actor"
	if _, err := rowToPlan(row); err == nil || !strings.Contains(err.Error(), "metadata mismatch") {
		t.Fatalf("expected metadata mismatch, got %v", err)
	}

	row, err = planToRow(*plan)
	if err != nil {
		t.Fatalf("planToRow: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(row.PayloadJSON, &payload); err != nil {
		t.Fatalf("decode test payload: %v", err)
	}
	payload["canExecute"] = true
	row.PayloadJSON, err = json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode test payload: %v", err)
	}
	if _, err := rowToPlan(row); err == nil || !strings.Contains(err.Error(), "cannot grant execution authority") {
		t.Fatalf("expected execution-authority rejection, got %v", err)
	}
}

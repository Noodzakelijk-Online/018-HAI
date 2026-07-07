package reconcile

import (
	"testing"

	"automation-hub-backend/internal/models"
	"github.com/google/uuid"
)

func TestScanCleanDataHasNoFindings(t *testing.T) {
	memories := []models.ContextMemory{
		{ID: uuid.New(), Content: "ok", Kind: "preference", Confidence: 0.8},
	}
	r := ScanMemories(memories)
	if !r.Clean() || r.Scanned != 1 {
		t.Fatalf("clean scan expected: %+v", r)
	}
}

func TestScanFlagsAndClassifiesRepairs(t *testing.T) {
	memories := []models.ContextMemory{
		{ID: uuid.New(), Content: "ok", Kind: "preference", Confidence: 2.0}, // repairable (confidence)
		{ID: uuid.New(), Content: "", Kind: "", Confidence: 0.5},             // needs manual input
	}
	r := ScanMemories(memories)
	if len(r.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(r.Findings))
	}

	var repairable, manual int
	for _, f := range r.Findings {
		if f.Repairable {
			repairable++
		} else {
			manual++
		}
	}
	if repairable != 1 || manual != 1 {
		t.Fatalf("classification wrong: %d repairable, %d manual", repairable, manual)
	}
}

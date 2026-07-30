package domainpack

import (
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/models"

	"github.com/google/uuid"
)

func TestPreferencePersistenceConversionRoundTrip(t *testing.T) {
	now := time.Date(2026, time.July, 30, 16, 0, 0, 0, time.UTC)
	enabled := false
	value := PackPreference{
		OwnerIdentity:       "alice",
		PackID:              PackIdentityRoles,
		CatalogVersion:      CatalogVersion,
		Revision:            4,
		Status:              PreferenceStatusActive,
		Enabled:             &enabled,
		ClassificationBoost: -7,
		ForceLocalOnly:      true,
		Adaptation: PackAdaptation{
			Notes: "reviewed",
			AdditionalClassificationSignals: []ClassificationSignal{{
				Phrase: "board role", Strength: SignalStrong, Reason: "owner role phrase",
			}},
			AdditionalAgentCapabilities: []string{"role-review"},
		},
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}
	row, err := preferenceToModel(value)
	if err != nil {
		t.Fatalf("preferenceToModel: %v", err)
	}
	roundTrip, err := preferenceFromModel(row)
	if err != nil {
		t.Fatalf("preferenceFromModel: %v", err)
	}
	if roundTrip.OwnerIdentity != value.OwnerIdentity ||
		roundTrip.PackID != value.PackID ||
		roundTrip.Revision != value.Revision ||
		roundTrip.Enabled == nil || *roundTrip.Enabled ||
		roundTrip.Adaptation.Notes != "reviewed" ||
		len(roundTrip.Adaptation.AdditionalClassificationSignals) != 1 {
		t.Fatalf("round trip = %#v, want %#v", roundTrip, value)
	}
}

func TestPreferencePersistenceRejectsUnknownJSONAndStatus(t *testing.T) {
	row := models.DomainPackPreference{
		ID: uuid.New(), OwnerIdentity: "alice", PackID: string(PackWorkVenture),
		CatalogVersion: CatalogVersion, Revision: 1, Status: string(PreferenceStatusActive),
		AdaptationsJSON: `{"unknown":true}`, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := preferenceFromModel(row); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown JSON error = %v", err)
	}
	row.AdaptationsJSON = `{}`
	row.Status = "invalid"
	if _, err := preferenceFromModel(row); err == nil ||
		!strings.Contains(err.Error(), "invalid status") {
		t.Fatalf("invalid status error = %v", err)
	}
}

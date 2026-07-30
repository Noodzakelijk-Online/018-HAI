//go:build integration

package domainpack

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/migrations"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGormPreferenceRepositoryOwnerIsolationAndRoundTrip(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("HAI_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open Postgres: %v", err)
	}
	if _, err := infra.ApplyMigrations(db, migrations.Files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}

	suffix := uuid.NewString()
	alice := "domainpack-alice-" + suffix
	bob := "domainpack-bob-" + suffix
	t.Cleanup(func() {
		_ = db.Where("owner_identity IN ?", []string{alice, bob}).
			Delete(&models.DomainPackPreference{}).Error
	})
	now := time.Date(2026, time.July, 30, 17, 0, 0, 0, time.UTC)
	repository := NewGormPreferenceRepository(db, func() time.Time { return now })
	disabled := false
	first, err := repository.Upsert(PackPreference{
		OwnerIdentity:       alice,
		PackID:              PackWorkVenture,
		Status:              PreferenceStatusActive,
		Enabled:             &disabled,
		ClassificationBoost: -8,
		ForceLocalOnly:      true,
		Adaptation: PackAdaptation{
			Notes: "postgres round trip",
			AdditionalClassificationSignals: []ClassificationSignal{{
				Phrase:   "client north star",
				Strength: SignalStrong,
				Reason:   "owner-specific phrase",
			}},
		},
	})
	if err != nil {
		t.Fatalf("Upsert alice: %v", err)
	}
	if first.Revision != 1 || first.CatalogVersion != CatalogVersion {
		t.Fatalf("first metadata = %#v", first)
	}
	if _, err := repository.Upsert(PackPreference{
		OwnerIdentity: bob,
		PackID:        PackWorkVenture,
		Status:        PreferenceStatusActive,
	}); err != nil {
		t.Fatalf("Upsert bob: %v", err)
	}

	gotAlice, ok, err := repository.Get(alice, PackWorkVenture)
	if err != nil || !ok || gotAlice.Enabled == nil || *gotAlice.Enabled ||
		gotAlice.Adaptation.Notes != "postgres round trip" {
		t.Fatalf("Get alice = %#v, ok=%v, err=%v", gotAlice, ok, err)
	}
	gotBob, ok, err := repository.Get(bob, PackWorkVenture)
	if err != nil || !ok || gotBob.Enabled != nil ||
		gotBob.Adaptation.Notes != "" {
		t.Fatalf("Get bob = %#v, ok=%v, err=%v", gotBob, ok, err)
	}
	aliceList, err := repository.List(alice)
	if err != nil || len(aliceList) != 1 ||
		aliceList[0].OwnerIdentity != alice {
		t.Fatalf("List alice = %#v, err=%v", aliceList, err)
	}

	gotAlice.Status = PreferenceStatusArchived
	gotAlice.UpdatedAt = time.Time{}
	now = now.Add(time.Minute)
	second, err := repository.Upsert(gotAlice)
	if err != nil {
		t.Fatalf("archive alice: %v", err)
	}
	if second.Revision != 2 || second.Status != PreferenceStatusArchived ||
		!second.CreatedAt.Equal(first.CreatedAt) ||
		!second.UpdatedAt.Equal(now) {
		t.Fatalf("second metadata = %#v, first=%#v", second, first)
	}
	stale := gotAlice
	if _, err := repository.Upsert(stale); !errors.Is(err, ErrPreferenceConflict) {
		t.Fatalf("stale update error = %v, want ErrPreferenceConflict", err)
	}
}

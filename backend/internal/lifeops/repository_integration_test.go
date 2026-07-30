//go:build integration

package lifeops

import (
	"os"
	"strings"
	"testing"
	"time"

	"automation-hub-backend/migrations"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openLifeOpsIntegrationRepository(t *testing.T) (*GormRepository, *gorm.DB) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("HAI_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres integration test")
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("HAI_ALLOW_DESTRUCTIVE_DATABASE_TESTS")), "true") {
		t.Skip("HAI_ALLOW_DESTRUCTIVE_DATABASE_TESTS=true is required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	var databaseName string
	if err := db.Raw("SELECT current_database()").Scan(&databaseName).Error; err != nil {
		t.Fatalf("read current database: %v", err)
	}
	if !strings.Contains(strings.ToLower(databaseName), "test") {
		t.Fatalf("refusing destructive integration setup for database %q", databaseName)
	}
	if err := db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;").Error; err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	for _, path := range []string{"pre/0001_extensions.up.sql", "pre/0007_lifeops.up.sql"} {
		sql, err := migrations.Files.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		if err := db.Exec(string(sql)).Error; err != nil {
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}
	return NewGormRepository(db), db
}

func TestLifeOpsPostgresRoundTripOwnerIsolationAndAppendOnlyHistory(t *testing.T) {
	repository, db := openLifeOpsIntegrationRepository(t)
	now := time.Date(2026, 7, 30, 16, 0, 0, 0, time.UTC)
	service := NewService(repository, WithClock(func() time.Time { return now }))

	link, err := service.LinkEntity(LinkEntityRequest{
		OwnerIdentity: "alice", EntityType: "pursuit", EntityID: "p-1",
		DomainID: DomainWorkVenture, Confidence: .9, SourceLabel: "operator",
		Evidence: []string{"source://p-1"},
	})
	if err != nil {
		t.Fatalf("LinkEntity: %v", err)
	}
	if !link.Primary {
		t.Fatal("first domain link was not made primary")
	}
	if links, err := service.EntityDomains("bob", "pursuit", "p-1"); err != nil || len(links) != 0 {
		t.Fatalf("cross-owner links = %#v, %v", links, err)
	}

	need, err := service.RecordNeed(RecordNeedRequest{
		OwnerIdentity: "alice", DomainID: DomainHealthWellbeing,
		NeedLevel: "wellbeing", State: "attention_required",
		CurrentLevel: 30, TargetLevel: 80, Priority: 90, Confidence: .8,
		Evidence: []string{"daily check-in"}, SourceLabel: "operator", ObservedAt: now,
	})
	if err != nil {
		t.Fatalf("RecordNeed: %v", err)
	}
	if err := db.Exec("UPDATE life_need_observations SET state = ? WHERE id = ?", "tampered", need.ID).Error; err == nil {
		t.Fatal("Postgres allowed append-only need mutation")
	}
	if err := db.Exec("DELETE FROM life_need_observations WHERE id = ?", need.ID).Error; err == nil {
		t.Fatal("Postgres allowed append-only need deletion")
	}

	capacity, err := service.RecordCapacity(RecordCapacityRequest{
		OwnerIdentity: "alice", Status: CapacityConstrained,
		Signals:              CapacitySignals{Energy: 40, AttentionQuality: 35},
		TimeAvailableMinutes: 45, ConcurrentWorkLimit: 1, CurrentLoad: 70,
		SourceLabel: "operator", CapturedAt: now, Confidence: .9,
	})
	if err != nil {
		t.Fatalf("RecordCapacity: %v", err)
	}
	latest, err := service.LatestCapacity("alice")
	if err != nil || latest.ID != capacity.ID || latest.Signals.Energy != 40 {
		t.Fatalf("LatestCapacity = %#v, %v", latest, err)
	}

	goal, err := service.CreateGoal(CreateGoalRequest{
		OwnerIdentity: "alice", Level: GoalLevelPursuit,
		DomainIDs: []DomainID{DomainWorkVenture}, Title: "Deliver governed HAI",
		SuccessCriteria: []string{"all required evidence passes"},
		StopConditions:  []string{"owner withdraws consent"},
		Confidence:      .9, SourceLabel: "operator",
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	if _, err := service.Goal("bob", goal.ID); err != ErrNotFound {
		t.Fatalf("cross-owner goal error = %v, want ErrNotFound", err)
	}
}

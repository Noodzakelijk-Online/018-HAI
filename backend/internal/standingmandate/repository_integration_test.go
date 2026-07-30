//go:build integration

package standingmandate

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"automation-hub-backend/migrations"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func standingMandateIntegrationRepository(t *testing.T) (*GormRepository, *gorm.DB) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("HAI_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres integration test")
	}
	if !strings.EqualFold(
		strings.TrimSpace(os.Getenv("HAI_ALLOW_DESTRUCTIVE_DATABASE_TESTS")),
		"true",
	) {
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
	for _, path := range []string{
		"pre/0001_extensions.up.sql",
		"pre/0008_standing_mandates.up.sql",
	} {
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

func TestStandingMandatePostgresLifecycleOwnerIsolationAndDecisionDurability(t *testing.T) {
	repository, db := standingMandateIntegrationRepository(t)
	now := time.Date(2026, time.July, 30, 15, 0, 0, 0, time.UTC)
	service, err := NewService(repository, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	mandate := createTestMandate(t, service, now)
	if _, err := service.Get(
		context.Background(),
		"other-owner",
		mandate.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner mandate read error = %v, want ErrNotFound", err)
	}
	active, err := service.Activate(
		context.Background(),
		"robert",
		mandate.ID,
		mandate.Revision,
	)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if active.Status != StatusActive || active.Revision != 2 {
		t.Fatalf("active mandate = %#v", active)
	}
	if _, err := service.Activate(
		context.Background(),
		"robert",
		mandate.ID,
		mandate.Revision,
	); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale activation error = %v, want ErrRevisionConflict", err)
	}

	decision, err := service.Authorize(
		context.Background(),
		active.ID,
		validAction(now),
	)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	stored, err := service.GetDecision(
		context.Background(),
		"robert",
		decision.ID,
	)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if stored.Evidence.DecisionDigest != decision.Evidence.DecisionDigest {
		t.Fatalf("stored decision digest = %q, want %q",
			stored.Evidence.DecisionDigest,
			decision.Evidence.DecisionDigest,
		)
	}
	if _, err := service.GetDecision(
		context.Background(),
		"other-owner",
		decision.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner decision read error = %v, want ErrNotFound", err)
	}
	if err := db.Exec(
		"UPDATE standing_mandate_authorization_decisions SET reason = ? WHERE id = ?",
		"tampered",
		decision.ID,
	).Error; err == nil {
		t.Fatal("Postgres allowed immutable decision mutation")
	}
	if err := db.Exec(
		"DELETE FROM standing_mandate_authorization_decisions WHERE id = ?",
		decision.ID,
	).Error; err == nil {
		t.Fatal("Postgres allowed immutable decision deletion")
	}

	now = now.Add(time.Minute)
	revoked, err := service.Revoke(
		context.Background(),
		"robert",
		active.ID,
		active.Revision,
		"robert",
		"mandate retired",
	)
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if revoked.Status != StatusRevoked ||
		revoked.Revision != 3 ||
		revoked.RevokedAt == nil {
		t.Fatalf("revoked mandate = %#v", revoked)
	}
	if err := db.Exec(
		"DELETE FROM standing_mandates WHERE id = ?",
		revoked.ID,
	).Error; err == nil {
		t.Fatal("Postgres allowed durable mandate deletion")
	}
}

func TestStandingMandatePostgresConcurrentLifecycleUsesOptimisticRevision(t *testing.T) {
	repository, _ := standingMandateIntegrationRepository(t)
	now := time.Date(2026, time.July, 30, 16, 0, 0, 0, time.UTC)
	service, err := NewService(repository, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	mandate := createTestMandate(t, service, now)

	results := make(chan error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, activateErr := service.Activate(
				context.Background(),
				"robert",
				mandate.ID,
				mandate.Revision,
			)
			results <- activateErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent activation error: %v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf(
			"concurrent activations: successes=%d conflicts=%d, want 1 and 1",
			successes,
			conflicts,
		)
	}

	active, err := service.Get(context.Background(), "robert", mandate.ID)
	if err != nil {
		t.Fatalf("Get active mandate: %v", err)
	}
	now = now.Add(time.Minute)
	results = make(chan error, 2)
	start = make(chan struct{})
	wait = sync.WaitGroup{}
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, revokeErr := service.Revoke(
				context.Background(),
				"robert",
				active.ID,
				active.Revision,
				"robert",
				"concurrent retirement test",
			)
			results <- revokeErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	successes = 0
	conflicts = 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent revocation error: %v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf(
			"concurrent revocations: successes=%d conflicts=%d, want 1 and 1",
			successes,
			conflicts,
		)
	}
}

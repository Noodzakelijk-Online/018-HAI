//go:build integration

package migrations_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"automation-hub-backend/internal/infra"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestPursuitResourceReservationsConcurrencyAndImmutabilityInPostgres(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	files := migrationFilesThrough(t, "pre/0035_pursuit_resource_reservations")
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply pursuit resource reservation migration: %v", err)
	}

	t.Run("schema and append-only protections", func(t *testing.T) {
		pursuitID := insertReservationTestPursuit(t, db, "append-only", 60)
		reservationID := insertReservation(t, db, pursuitID, "append-only", "immutable", 10)
		settlementID := insertRelease(t, db, reservationID, pursuitID, "append-only")

		assertMutationRejected(t, db, `UPDATE pursuit_resource_reservations SET reason = 'changed' WHERE id = ?`, reservationID)
		assertMutationRejected(t, db, `DELETE FROM pursuit_resource_reservations WHERE id = ?`, reservationID)
		assertMutationRejected(t, db, `UPDATE pursuit_resource_reservation_settlements SET evidence_uri = 'changed' WHERE id = ?`, settlementID)
		assertMutationRejected(t, db, `DELETE FROM pursuit_resource_reservation_settlements WHERE id = ?`, settlementID)
		assertMutationRejected(t, db, `TRUNCATE pursuit_resource_reservation_settlements`)
	})

	t.Run("two concurrent holds compete atomically", func(t *testing.T) {
		pursuitID := insertReservationTestPursuit(t, db, "concurrent", 60)
		start := make(chan struct{})
		results := make(chan error, 2)
		var workers sync.WaitGroup

		for index := 0; index < 2; index++ {
			workers.Add(1)
			go func(index int) {
				defer workers.Done()
				<-start
				results <- db.Exec(`
					INSERT INTO pursuit_resource_reservations (
						pursuit_id, owner_identity, operation_id, estimated_effort_minutes,
						actor, record_digest, reserved_at
					) VALUES (?, 'concurrent', ?, 40, 'concurrent', ?, now())`,
					pursuitID,
					fmt.Sprintf("concurrent-%d", index),
					strings.Repeat(fmt.Sprintf("%x", index+1), 64),
				).Error
			}(index)
		}
		close(start)
		workers.Wait()
		close(results)

		succeeded := 0
		failedForCeiling := 0
		for err := range results {
			if err == nil {
				succeeded++
				continue
			}
			if strings.Contains(err.Error(), "pursuit effort reservation exceeds remaining ceiling") {
				failedForCeiling++
				continue
			}
			t.Fatalf("unexpected concurrent reservation error: %v", err)
		}
		if succeeded != 1 || failedForCeiling != 1 {
			t.Fatalf("concurrent reservations: succeeded=%d ceiling_failures=%d, want 1 and 1", succeeded, failedForCeiling)
		}
	})

	t.Run("recorded usage and active hold share the ceiling", func(t *testing.T) {
		pursuitID := insertReservationTestPursuit(t, db, "combined", 60)
		if err := db.Exec(`
			INSERT INTO pursuit_resource_events (
				pursuit_id, owner_identity, kind, effort_minutes, actor,
				idempotency_key, record_digest, occurred_at
			) VALUES (?, 'combined', 'effort_recorded', 30, 'combined', 'recorded-30', ?, now())`,
			pursuitID, strings.Repeat("a", 64)).Error; err != nil {
			t.Fatalf("insert recorded effort: %v", err)
		}
		insertReservation(t, db, pursuitID, "combined", "hold-20", 20)
		if err := reservationInsertError(db, pursuitID, "combined", "hold-11", 11, "b"); err == nil ||
			!strings.Contains(err.Error(), "pursuit effort reservation exceeds remaining ceiling") {
			t.Fatalf("recorded plus held ceiling error = %v, want effort ceiling rejection", err)
		}
	})

	t.Run("settlement releases held capacity", func(t *testing.T) {
		pursuitID := insertReservationTestPursuit(t, db, "settlement", 60)
		reservationID := insertReservation(t, db, pursuitID, "settlement", "hold-50", 50)
		if err := reservationInsertError(db, pursuitID, "settlement", "blocked-20", 20, "c"); err == nil ||
			!strings.Contains(err.Error(), "pursuit effort reservation exceeds remaining ceiling") {
			t.Fatalf("active hold ceiling error = %v, want effort ceiling rejection", err)
		}

		insertRelease(t, db, reservationID, pursuitID, "settlement")
		if err := reservationInsertError(db, pursuitID, "settlement", "released-20", 20, "d"); err != nil {
			t.Fatalf("reserve after release: %v", err)
		}
	})

	if err := infra.RollbackMigration(db, files, "pre", "pre/0035_pursuit_resource_reservations"); err == nil ||
		!strings.Contains(err.Error(), "refusing to remove non-empty pursuit resource reservation ledger") {
		t.Fatalf("non-empty rollback error = %v, want reservation ledger refusal", err)
	}
}

func TestPursuitResourceReservationReconciliationReasonInPostgres(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	legacyFiles := migrationFilesThrough(t, "pre/0035_pursuit_resource_reservations")
	if _, err := infra.ApplyMigrations(db, legacyFiles, "pre"); err != nil {
		t.Fatalf("apply legacy reservation migration: %v", err)
	}
	legacyPursuitID := insertReservationTestPursuit(t, db, "legacy-reconciliation", 60)
	legacyReservationID := insertReservation(t, db, legacyPursuitID, "legacy-reconciliation", "legacy-release", 5)
	legacySettlementID := insertRelease(t, db, legacyReservationID, legacyPursuitID, "legacy-reconciliation")

	files := migrationFilesThrough(t, "pre/0036_pursuit_resource_reservation_reconciliation")
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply reconciliation migration: %v", err)
	}
	var legacyReason string
	if err := db.Raw(`SELECT reason FROM pursuit_resource_reservation_settlements WHERE id = ?`, legacySettlementID).Scan(&legacyReason).Error; err != nil {
		t.Fatalf("read legacy reconciliation reason: %v", err)
	}
	if legacyReason != "Legacy release recorded before reconciliation reason capture." {
		t.Fatalf("legacy reconciliation reason = %q", legacyReason)
	}
	pursuitID := insertReservationTestPursuit(t, db, "reconciliation", 60)
	reservationID := insertReservation(t, db, pursuitID, "reconciliation", "crashed-operation", 15)
	settlementID := uuid.New()
	if err := db.Exec(`
		INSERT INTO pursuit_resource_reservation_settlements (
			id, reservation_id, pursuit_id, owner_identity, disposition,
			reason, actor, record_digest, settled_at
		) VALUES (?, ?, ?, 'reconciliation', 'released', ?, 'operator', ?, now())`,
		settlementID, reservationID, pursuitID,
		"Worker crashed and no process remains.", strings.Repeat("f", 64),
	).Error; err != nil {
		t.Fatalf("insert reconciliation settlement: %v", err)
	}
	assertMutationRejected(t, db, `UPDATE pursuit_resource_reservation_settlements SET reason = 'rewritten reason' WHERE id = ?`, settlementID)

	secondReservationID := insertReservation(t, db, pursuitID, "reconciliation", "missing-reason", 15)
	if err := db.Exec(`
		INSERT INTO pursuit_resource_reservation_settlements (
			reservation_id, pursuit_id, owner_identity, disposition,
			reason, actor, record_digest, settled_at
		) VALUES (?, ?, 'reconciliation', 'released', '', 'operator', ?, now())`,
		secondReservationID, pursuitID, strings.Repeat("a", 64),
	).Error; err == nil || !strings.Contains(err.Error(), "pursuit_resource_settlement_reason_check") {
		t.Fatalf("missing release reason error = %v, want reason constraint", err)
	}
	if err := infra.RollbackMigration(db, files, "pre", "pre/0036_pursuit_resource_reservation_reconciliation"); err == nil ||
		!strings.Contains(err.Error(), "refusing to discard pursuit resource reservation reconciliation reasons") {
		t.Fatalf("reconciliation rollback error = %v, want evidence refusal", err)
	}
}

func insertReservationTestPursuit(t *testing.T, db *gorm.DB, owner string, ceilingMinutes int64) uuid.UUID {
	t.Helper()
	pursuitID := uuid.New()
	resourceLimits := fmt.Sprintf(`{"maxEffortHours":%g}`, float64(ceilingMinutes)/60)
	if err := db.Exec(`
		INSERT INTO public.pursuits (
			id, owner_identity, title, status, risk_level, autonomy_level,
			completion_state, resource_limits, archived, created_at, updated_at
		) VALUES (?, ?, ?, 'active', 'low', 'manual', 'open', ?::jsonb, false, now(), now())`,
		pursuitID, owner, "Reservation test "+owner, resourceLimits).Error; err != nil {
		t.Fatalf("insert pursuit: %v", err)
	}
	return pursuitID
}

func insertReservation(t *testing.T, db *gorm.DB, pursuitID uuid.UUID, owner, operationID string, effortMinutes int64) uuid.UUID {
	t.Helper()
	reservationID := uuid.New()
	if err := db.Exec(`
		INSERT INTO pursuit_resource_reservations (
			id, pursuit_id, owner_identity, operation_id, estimated_effort_minutes,
			actor, record_digest, reserved_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, now())`,
		reservationID, pursuitID, owner, operationID, effortMinutes, owner, strings.Repeat("e", 64)).Error; err != nil {
		t.Fatalf("insert reservation %s: %v", operationID, err)
	}
	return reservationID
}

func reservationInsertError(db *gorm.DB, pursuitID uuid.UUID, owner, operationID string, effortMinutes int64, digestCharacter string) error {
	return db.Exec(`
		INSERT INTO pursuit_resource_reservations (
			pursuit_id, owner_identity, operation_id, estimated_effort_minutes,
			actor, record_digest, reserved_at
		) VALUES (?, ?, ?, ?, ?, ?, now())`,
		pursuitID, owner, operationID, effortMinutes, owner, strings.Repeat(digestCharacter, 64)).Error
}

func insertRelease(t *testing.T, db *gorm.DB, reservationID, pursuitID uuid.UUID, owner string) uuid.UUID {
	t.Helper()
	settlementID := uuid.New()
	if err := db.Exec(`
		INSERT INTO pursuit_resource_reservation_settlements (
			id, reservation_id, pursuit_id, owner_identity, disposition,
			actor, record_digest, settled_at
		) VALUES (?, ?, ?, ?, 'released', ?, ?, now())`,
		settlementID, reservationID, pursuitID, owner, owner, strings.Repeat("f", 64)).Error; err != nil {
		t.Fatalf("release reservation: %v", err)
	}
	return settlementID
}

func assertMutationRejected(t *testing.T, db *gorm.DB, query string, args ...any) {
	t.Helper()
	err := db.Exec(query, args...).Error
	if err == nil || !strings.Contains(err.Error(), "pursuit resource reservations and settlements are append-only") {
		t.Fatalf("append-only mutation error = %v, want append-only rejection", err)
	}
}

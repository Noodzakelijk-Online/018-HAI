//go:build integration

package migrations_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/internal/task"
	"automation-hub-backend/migrations"
	"gorm.io/gorm"
)

func TestTaskOperationIdentityConcurrencyAndFencingInPostgres(t *testing.T) {
	db := openIsolatedMigrationDatabase(t)
	files := migrationFilesThrough(t, "pre/0037_task_operation_identity")
	if _, err := infra.ApplyMigrations(db, files, "pre"); err != nil {
		t.Fatalf("apply task operation identity migration: %v", err)
	}
	repository := task.NewPostgresTaskStateRepository(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	start := make(chan struct{})
	claims := make(chan task.TaskOperationClaim, 2)
	errorsFound := make(chan error, 2)
	var workers sync.WaitGroup
	for _, worker := range []string{"worker:one", "worker:two"} {
		workers.Add(1)
		go func(worker string) {
			defer workers.Done()
			<-start
			claim, err := repository.ClaimTaskOperation("alice", "event:postgres-1", digest, "run", worker, now, time.Minute)
			if err != nil {
				errorsFound <- err
				return
			}
			claims <- claim
		}(worker)
	}
	close(start)
	workers.Wait()
	close(claims)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent claim: %v", err)
	}
	counts := map[string]int{}
	var acquired task.TaskOperationClaim
	for claim := range claims {
		counts[claim.Disposition]++
		if claim.Disposition == task.TaskOperationAcquired {
			acquired = claim
		}
	}
	if counts[task.TaskOperationAcquired] != 1 || counts[task.TaskOperationInProgress] != 1 {
		t.Fatalf("claim dispositions = %#v", counts)
	}
	if _, err := repository.ClaimTaskOperation("alice", "event:postgres-1", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "run", "worker:three", now, time.Minute); !errors.Is(err, task.ErrTaskStateConflict) {
		t.Fatalf("changed request error = %v, want conflict", err)
	}

	completed, err := repository.CompleteTaskOperation(
		"alice", acquired.Operation.ID, acquired.Operation.LeaseOwner,
		acquired.Operation.LeaseGeneration, "plan-postgres-1", now.Add(time.Second),
	)
	if err != nil || !completed {
		t.Fatalf("complete operation = (%v, %v)", completed, err)
	}
	replay, err := repository.ClaimTaskOperation("alice", "event:postgres-1", digest, "run", "worker:four", now.Add(2*time.Second), time.Minute)
	if err != nil || replay.Disposition != task.TaskOperationReplay || replay.Operation.TaskPlanID != "plan-postgres-1" {
		t.Fatalf("replay = (%#v, %v)", replay, err)
	}

	assertTaskOperationMutationRejected(t, db, "UPDATE task_operations SET request_digest = repeat('c', 64) WHERE id = ?", acquired.Operation.ID)
	assertTaskOperationMutationRejected(t, db, "UPDATE task_operations SET status = 'needs_review', task_plan_id = '', completed_at = NULL, last_error = 'changed' WHERE id = ?", acquired.Operation.ID)
	assertTaskOperationMutationRejected(t, db, "DELETE FROM task_operations WHERE id = ?", acquired.Operation.ID)
	assertTaskOperationMutationRejected(t, db, "TRUNCATE task_operations")
	if err := infra.RollbackMigration(db, migrations.Files, "pre", "pre/0037_task_operation_identity"); err == nil {
		t.Fatal("rollback discarded non-empty task operation audit state")
	}
}

func assertTaskOperationMutationRejected(t *testing.T, db *gorm.DB, query string, args ...interface{}) {
	t.Helper()
	err := db.Exec(query, args...).Error
	if err == nil || (!strings.Contains(err.Error(), "task operation") && !strings.Contains(err.Error(), "terminal task operation")) {
		t.Fatalf("task operation mutation error = %v, want durable-state rejection", err)
	}
}

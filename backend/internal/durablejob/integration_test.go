//go:build integration

// Real-Postgres proof for the durable worker. Runs only under
// `-tags integration` with HAI_TEST_DATABASE_DSN set.
package durablejob

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"automation-hub-backend/internal/backoff"
	"automation-hub-backend/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func integrationRepo(t *testing.T) (Repository, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("HAI_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`).Error; err != nil {
		t.Fatalf("extension: %v", err)
	}
	_ = db.Migrator().DropTable(&models.DurableJob{})
	if err := db.AutoMigrate(&models.DurableJob{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return NewGormRepository(db), db
}

func TestDurableJobSurvivesProcessRestart(t *testing.T) {
	repo, _ := integrationRepo(t)
	now := time.Now().UTC()

	first := NewRunner(repo, Options{
		WorkerID: "w1",
		Policy:   backoff.Policy{Base: time.Millisecond, Factor: 1},
		Now:      func() time.Time { return now },
	})
	first.Register("job", func(ctx context.Context, j models.DurableJob) error { return errors.New("transient") })
	job, err := first.Enqueue("job", `{"n":1}`, now, 3)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := first.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	stored, err := repo.Find(job.ID)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if stored.Status != models.DurableJobPending || stored.Attempts != 1 {
		t.Fatalf("after failure status=%q attempts=%d, want pending/1", stored.Status, stored.Attempts)
	}

	// A completely new Runner (simulating a restarted process) must pick up the
	// persisted job and finish it — nothing lived in memory.
	later := now.Add(time.Minute)
	second := NewRunner(repo, Options{WorkerID: "w2", Now: func() time.Time { return later }})
	second.Register("job", func(ctx context.Context, j models.DurableJob) error { return nil })
	processed, err := second.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce (restarted): %v", err)
	}
	if processed != 1 {
		t.Fatalf("restarted worker processed=%d, want 1", processed)
	}
	stored, _ = repo.Find(job.ID)
	if stored.Status != models.DurableJobSucceeded {
		t.Fatalf("status=%q, want succeeded after restart", stored.Status)
	}
}

func TestConcurrentWorkersNeverDoubleClaim(t *testing.T) {
	repo, _ := integrationRepo(t)
	now := time.Now().UTC()
	const jobCount = 25

	seeder := NewRunner(repo, Options{WorkerID: "seed", Now: func() time.Time { return now }})
	for i := 0; i < jobCount; i++ {
		if _, err := seeder.Enqueue("shared", "{}", now, 3); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	var mu sync.Mutex
	executions := map[string]int{}
	makeRunner := func(id string) *Runner {
		r := NewRunner(repo, Options{WorkerID: id, Batch: 5, Now: func() time.Time { return now }})
		r.Register("shared", func(ctx context.Context, j models.DurableJob) error {
			mu.Lock()
			executions[j.ID.String()]++
			mu.Unlock()
			return nil
		})
		return r
	}

	// Two workers hammer the same queue simultaneously.
	var wg sync.WaitGroup
	for _, id := range []string{"wA", "wB"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			runner := makeRunner(id)
			for i := 0; i < 10; i++ {
				if _, err := runner.RunOnce(context.Background()); err != nil {
					t.Errorf("%s RunOnce: %v", id, err)
					return
				}
			}
		}(id)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(executions) != jobCount {
		t.Fatalf("executed %d distinct jobs, want %d", len(executions), jobCount)
	}
	for id, count := range executions {
		if count != 1 {
			t.Fatalf("job %s executed %d times; FOR UPDATE SKIP LOCKED must prevent double claiming", id, count)
		}
	}
}

func TestExpiredLeaseIsReclaimed(t *testing.T) {
	repo, db := integrationRepo(t)
	now := time.Now().UTC()
	lease := 30 * time.Second

	seeder := NewRunner(repo, Options{WorkerID: "seed", Now: func() time.Time { return now }})
	job, err := seeder.Enqueue("orphan", "{}", now, 3)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// Claim it, then abandon it (as if the worker crashed).
	if _, err := repo.ClaimDue("dead-worker", now, 10); err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	// Backdate the lease so it is expired.
	if err := db.Model(&models.DurableJob{}).Where("id = ?", job.ID).
		Update("locked_at", now.Add(-2*lease)).Error; err != nil {
		t.Fatalf("backdate lease: %v", err)
	}

	survivor := NewRunner(repo, Options{WorkerID: "w2", Lease: lease, Now: func() time.Time { return now }})
	survivor.Register("orphan", func(ctx context.Context, j models.DurableJob) error { return nil })
	processed, err := survivor.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed=%d, want 1 (orphaned job reclaimed)", processed)
	}
	stored, _ := repo.Find(job.ID)
	if stored.Status != models.DurableJobSucceeded {
		t.Fatalf("status=%q, want succeeded after lease recovery", stored.Status)
	}
}

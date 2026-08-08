package outcomeevaluation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestPostgresRepositoryLifecycle is opt-in. Set
// HAI_OUTCOME_EVALUATION_TEST_DATABASE_DSN to a disposable database where
// migration 0023 has already been applied.
func TestPostgresRepositoryLifecycle(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("HAI_OUTCOME_EVALUATION_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("HAI_OUTCOME_EVALUATION_TEST_DATABASE_DSN not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open Postgres: %v", err)
	}
	// This switch exists only for disposable repository tests. Production and
	// normal integration runs must receive the schema from migration 0023.
	if strings.EqualFold(strings.TrimSpace(os.Getenv("HAI_OUTCOME_EVALUATION_TEST_BOOTSTRAP_SCHEMA")), "true") {
		if err := db.Exec(postgres0023SchemaContract).Error; err != nil {
			t.Fatalf("bootstrap disposable 0023 schema contract: %v", err)
		}
	}
	for _, table := range []string{
		"public.outcome_evaluation_outcome_revisions",
		"public.outcome_evaluation_evaluations",
		"public.outcome_evaluation_corrections",
	} {
		var relation *string
		if err := db.Raw(`SELECT to_regclass(?)::text`, table).Row().Scan(&relation); err != nil {
			t.Fatalf("check %s: %v", table, err)
		}
		if relation == nil || *relation == "" {
			t.Fatalf("required 0023 table %s is missing", table)
		}
	}

	repository, err := NewPostgresRepositoryWithLimits(db, HistoryLimits{
		OutcomeRevisions: 2,
		Evaluations:      10,
		Corrections:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	suffix := fmt.Sprintf("%d", now.UnixNano())
	owner := "outcome-postgres-owner-" + suffix
	workspace := "workspace-" + suffix
	outcomeID := "outcome-" + suffix
	scope := Scope{OwnerID: owner, WorkspaceID: workspace}
	outcomeDefinition := validRequest().Outcome
	outcomeDefinition.ID = outcomeID
	outcomeDefinition.Scope = scope
	for index := range outcomeDefinition.Indicators {
		outcomeDefinition.Indicators[index].Baseline.Scope = scope
	}
	service := newService(repository, func() time.Time { return now })

	stored, created, err := service.StoreOutcome(context.Background(), owner, workspace, outcomeID, StoreOutcomeRequest{
		IdempotencyKey: "create-" + suffix, ExpectedRevision: 0, Outcome: outcomeDefinition,
	})
	if err != nil || !created || stored.Revision != 1 {
		t.Fatalf("StoreOutcome = (%d, %t, %v)", stored.Revision, created, err)
	}
	retry, created, err := service.StoreOutcome(context.Background(), owner, workspace, outcomeID, StoreOutcomeRequest{
		IdempotencyKey: "create-" + suffix, ExpectedRevision: 0, Outcome: outcomeDefinition,
	})
	if err != nil || created || retry.AuditDigest != stored.AuditDigest {
		t.Fatalf("idempotent StoreOutcome = (%t, %v)", created, err)
	}
	if _, err := repository.GetOutcome(context.Background(), "other-"+owner, workspace, outcomeID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner read = %v, want ErrNotFound", err)
	}

	for revision := int64(1); revision < 3; revision++ {
		outcomeDefinition.Statement = fmt.Sprintf("Outcome revision %d", revision+1)
		if _, _, err := service.StoreOutcome(context.Background(), owner, workspace, outcomeID, StoreOutcomeRequest{
			IdempotencyKey:   fmt.Sprintf("revision-%d-%s", revision+1, suffix),
			ExpectedRevision: revision, Outcome: outcomeDefinition,
		}); err != nil {
			t.Fatalf("store revision %d: %v", revision+1, err)
		}
	}
	if _, _, err := service.StoreOutcome(context.Background(), owner, workspace, outcomeID, StoreOutcomeRequest{
		IdempotencyKey: "stale-" + suffix, ExpectedRevision: 1, Outcome: outcomeDefinition,
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revision error = %v", err)
	}
	history, err := repository.ListOutcomeHistory(context.Background(), owner, workspace, outcomeID)
	if err != nil || len(history) != 2 || history[0].Revision != 2 || history[1].Revision != 3 {
		t.Fatalf("bounded outcome history = %#v, err %v", history, err)
	}
	exact, err := service.ResolveOutcomeRevision(context.Background(), owner, workspace, outcomeID, stored.Revision, stored.AuditDigest)
	if err != nil || exact.Revision != stored.Revision || exact.AuditDigest != stored.AuditDigest {
		t.Fatalf("exact historical outcome = %#v, err %v", exact, err)
	}
	if _, err := service.ResolveOutcomeRevision(context.Background(), owner, workspace, outcomeID, stored.Revision, history[1].AuditDigest); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mismatched exact digest error = %v, want ErrNotFound", err)
	}
	if _, err := service.ResolveOutcomeRevision(context.Background(), "other-"+owner, workspace, outcomeID, stored.Revision, stored.AuditDigest); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner exact read error = %v, want ErrNotFound", err)
	}

	makeObservation := func(id string, value float64, observedAt time.Time) Observation {
		return Observation{
			ID: id, Scope: scope, IndicatorID: "indicator-1", Value: value,
			ObservedAt: observedAt, RecordedAt: observedAt.Add(time.Minute),
			Verification: VerificationVerified,
			Sources: []SourceReference{{
				ID: id + "-source", URI: "https://evidence.example/" + id,
				ContentDigest: strings.Repeat("a", 64), RetrievedAt: observedAt.Add(time.Minute), Status: SourceVerified,
			}},
			Attribution: Attribution{Method: AttributionControlledStudy, Confidence: 0.8, Rationale: "Postgres integration observation."},
		}
	}
	historicalEvaluation, created, err := service.CreateEvaluation(context.Background(), owner, workspace, outcomeID, CreateEvaluationRequest{
		IdempotencyKey:     "historical-evaluation-" + suffix,
		OutcomeRevision:    stored.Revision,
		OutcomeAuditDigest: stored.AuditDigest,
		Observations: []Observation{
			makeObservation("historical-a-"+suffix, 10, testStart.Add(5*24*time.Hour)),
			makeObservation("historical-b-"+suffix, 12, testStart.Add(15*24*time.Hour)),
		},
		AsOf: testAsOf,
	})
	if err != nil || !created || historicalEvaluation.OutcomeRevision != stored.Revision {
		t.Fatalf("historical CreateEvaluation = (%+v, %t, %v)", historicalEvaluation, created, err)
	}
	if _, _, err := service.CreateEvaluation(context.Background(), owner, workspace, outcomeID, CreateEvaluationRequest{
		IdempotencyKey:     "forged-historical-evaluation-" + suffix,
		OutcomeRevision:    stored.Revision,
		OutcomeAuditDigest: history[1].AuditDigest,
		Observations:       []Observation{makeObservation("forged-historical-"+suffix, 10, testStart.Add(5*24*time.Hour))},
		AsOf:               testAsOf,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("forged historical evaluation error = %v, want ErrNotFound", err)
	}
	evaluationRequest := CreateEvaluationRequest{
		IdempotencyKey: "evaluation-" + suffix, OutcomeRevision: 3,
		Observations: []Observation{
			makeObservation("observation-a-"+suffix, 12, testStart.Add(5*24*time.Hour)),
			makeObservation("observation-b-"+suffix, 16, testStart.Add(15*24*time.Hour)),
		},
		AsOf: testAsOf,
	}
	start := make(chan struct{})
	results := make(chan struct {
		created bool
		err     error
	}, 12)
	var wait sync.WaitGroup
	for range 12 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, wasCreated, createErr := service.CreateEvaluation(context.Background(), owner, workspace, outcomeID, evaluationRequest)
			results <- struct {
				created bool
				err     error
			}{wasCreated, createErr}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	createdCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent evaluation: %v", result.err)
		}
		if result.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("concurrent evaluation created count = %d, want 1", createdCount)
	}

	original := makeObservation("corrected-"+suffix, 9, testStart.Add(20*24*time.Hour))
	correctionRequest := StoreCorrectionRequest{
		IdempotencyKey: "correction-" + suffix, OutcomeRevision: 3,
		Observation: original,
		Correction: UserCorrection{
			ID: "correction-" + suffix, Scope: scope, ObservationID: original.ID,
			ActorID: owner, UserConfirmed: true, CorrectedValue: 11,
			CorrectedVerification: VerificationUserConfirmed,
			Reason:                "Owner-confirmed Postgres integration correction.", CorrectedAt: original.RecordedAt.Add(time.Hour),
		},
		AsOf: testAsOf,
	}
	correction, created, err := service.StoreCorrection(context.Background(), owner, workspace, outcomeID, correctionRequest)
	if err != nil || !created {
		t.Fatalf("StoreCorrection = (%t, %v)", created, err)
	}
	retriedCorrection, created, err := service.StoreCorrection(context.Background(), owner, workspace, outcomeID, correctionRequest)
	if err != nil || created || retriedCorrection.AuditDigest != correction.AuditDigest {
		t.Fatalf("idempotent StoreCorrection = (%t, %v)", created, err)
	}
}

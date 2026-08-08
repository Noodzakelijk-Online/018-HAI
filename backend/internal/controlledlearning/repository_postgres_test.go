package controlledlearning

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"automation-hub-backend/internal/infra"
	"automation-hub-backend/migrations"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPostgresRepositoryFailsClosedWithoutDatabase(t *testing.T) {
	repository := NewPostgresRepository(nil)
	record := postgresTestOutcome(t, "owner@example.com", "missing-db")
	proposal := postgresTestProposal(t, record, "missing-db")

	if _, err := repository.CreateOutcome(context.Background(), record); err == nil {
		t.Fatal("CreateOutcome with nil database succeeded")
	}
	if _, err := repository.GetOutcome(
		context.Background(),
		record.OwnerIdentity,
		record.ID,
	); err == nil {
		t.Fatal("GetOutcome with nil database succeeded")
	}
	if _, err := repository.CreateProposal(
		context.Background(),
		proposal,
	); err == nil {
		t.Fatal("CreateProposal with nil database succeeded")
	}
}

func TestPostgresRepositoryRejectsInvalidRecordsBeforeSQL(t *testing.T) {
	repository := NewPostgresRepository(&gorm.DB{})
	record := postgresTestOutcome(t, "owner@example.com", "invalid")
	record.EvidenceDigest = "sha256:" + strings.Repeat("0", 64)
	if _, err := repository.CreateOutcome(
		context.Background(),
		record,
	); !errors.Is(err, ErrIntegrityViolation) {
		t.Fatalf("invalid outcome error = %v, want ErrIntegrityViolation", err)
	}

	proposal := postgresTestProposal(
		t,
		postgresTestOutcome(t, "owner@example.com", "evidence"),
		"invalid",
	)
	proposal.ID = "not-a-uuid"
	if _, err := repository.CreateProposal(
		context.Background(),
		proposal,
	); err == nil {
		t.Fatal("CreateProposal accepted a non-UUID id")
	}
}

func TestStrictDecodersDetectTamperingAndUnknownFields(t *testing.T) {
	record := postgresTestOutcome(t, "owner@example.com", "tamper")
	record.Summary = "payload changed after its digest was signed"
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal tampered outcome: %v", err)
	}
	if _, err := decodeOutcome(payload); !errors.Is(err, ErrIntegrityViolation) {
		t.Fatalf("tampered outcome error = %v, want ErrIntegrityViolation", err)
	}

	valid := postgresTestOutcome(t, "owner@example.com", "unknown")
	payload, err = json.Marshal(valid)
	if err != nil {
		t.Fatalf("marshal valid outcome: %v", err)
	}
	payload = append(payload[:len(payload)-1], []byte(`,"unknownField":true}`)...)
	if _, err := decodeOutcome(payload); err == nil {
		t.Fatal("outcome decoder accepted an unknown field")
	}
}

func TestPostgresRepositoryRoundTripReplayIsolationAndTamperDetection(t *testing.T) {
	repository, db := controlledLearningPostgresRepository(t)
	ctx := context.Background()
	owner := "learning-" + uuid.NewString() + "@example.com"
	record := postgresTestOutcome(t, owner, "round-trip")

	created, err := repository.CreateOutcome(ctx, record)
	if err != nil {
		t.Fatalf("CreateOutcome: %v", err)
	}
	if !reflect.DeepEqual(created, record) {
		t.Fatalf("created outcome differs:\ngot  %#v\nwant %#v", created, record)
	}

	replay := record
	replay.ID = uuid.NewString()
	replayed, err := repository.CreateOutcome(ctx, replay)
	if err != nil {
		t.Fatalf("replay CreateOutcome: %v", err)
	}
	if replayed.ID != record.ID || replayed.EvidenceDigest != record.EvidenceDigest {
		t.Fatalf("exact replay returned %#v, want existing record", replayed)
	}

	conflict := replay
	conflict.Summary = "different evidence under the same key"
	conflict.EvidenceDigest, err = outcomeDigest(conflict)
	if err != nil {
		t.Fatalf("digest conflicting outcome: %v", err)
	}
	if _, err := repository.CreateOutcome(
		ctx,
		conflict,
	); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting replay error = %v, want ErrIdempotencyConflict", err)
	}

	got, err := repository.GetOutcome(ctx, owner, record.ID)
	if err != nil {
		t.Fatalf("GetOutcome: %v", err)
	}
	if !reflect.DeepEqual(got, record) {
		t.Fatalf("round-trip outcome differs:\ngot  %#v\nwant %#v", got, record)
	}
	if _, err := repository.GetOutcome(
		ctx,
		"other-"+owner,
		record.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner GetOutcome error = %v, want ErrNotFound", err)
	}

	second := postgresTestOutcome(t, owner, "second")
	second.RecordedAt = record.RecordedAt.Add(time.Second)
	second.OccurredAt = record.OccurredAt.Add(time.Second)
	second.EvidenceDigest, err = outcomeDigest(second)
	if err != nil {
		t.Fatalf("digest second outcome: %v", err)
	}
	if _, err := repository.CreateOutcome(ctx, second); err != nil {
		t.Fatalf("create second outcome: %v", err)
	}
	list, err := repository.ListOutcomes(ctx, OutcomeQuery{
		OwnerIdentity: owner,
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("ListOutcomes: %v", err)
	}
	if len(list) != 1 || list[0].ID != second.ID {
		t.Fatalf("ListOutcomes = %#v, want newest outcome", list)
	}
	filtered, err := repository.ListOutcomes(ctx, OutcomeQuery{
		OwnerIdentity: owner,
		OperationID:   record.OperationID,
	})
	if err != nil || len(filtered) != 1 || filtered[0].ID != record.ID {
		t.Fatalf("filtered outcomes = %#v, %v", filtered, err)
	}

	if err := db.Exec(`
		UPDATE public.controlled_learning_outcomes
		SET operation_id = 'tampered'
		WHERE owner_identity = ? AND id = ?`,
		owner,
		record.ID,
	).Error; err == nil {
		t.Fatal("database allowed immutable outcome update")
	}
	if err := db.Exec(`
		DELETE FROM public.controlled_learning_outcomes
		WHERE owner_identity = ? AND id = ?`,
		owner,
		record.ID,
	).Error; err == nil {
		t.Fatal("database allowed immutable outcome deletion")
	}

	tampered := postgresTestOutcome(t, owner, "raw-tamper")
	tampered.Summary = "changed without recomputing the digest"
	tamperedPayload, err := json.Marshal(tampered)
	if err != nil {
		t.Fatalf("marshal raw tampered outcome: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO public.controlled_learning_outcomes (
			id, protocol_version, owner_identity, idempotency_key,
			operation_id, project_key, basis, outcome_status,
			verification_status, evidence_digest, occurred_at,
			recorded_at, payload
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb))`,
		tampered.ID,
		tampered.ProtocolVersion,
		tampered.OwnerIdentity,
		tampered.IdempotencyKey,
		tampered.OperationID,
		tampered.ProjectKey,
		string(tampered.Basis),
		string(tampered.Status),
		string(tampered.Verification),
		tampered.EvidenceDigest,
		tampered.OccurredAt,
		tampered.RecordedAt,
		string(tamperedPayload),
	).Error; err != nil {
		t.Fatalf("insert raw tampered fixture: %v", err)
	}
	if _, err := repository.GetOutcome(
		ctx,
		owner,
		tampered.ID,
	); !errors.Is(err, ErrIntegrityViolation) {
		t.Fatalf("tampered stored outcome error = %v, want ErrIntegrityViolation", err)
	}
}

func TestPostgresRepositoryProposalEvidenceAndAtomicReview(t *testing.T) {
	repository, db := controlledLearningPostgresRepository(t)
	ctx := context.Background()
	owner := "proposal-" + uuid.NewString() + "@example.com"
	outcome := postgresTestOutcome(t, owner, "proposal-evidence")
	if _, err := repository.CreateOutcome(ctx, outcome); err != nil {
		t.Fatalf("CreateOutcome: %v", err)
	}
	proposal := postgresTestProposal(t, outcome, "atomic-review")

	created, err := repository.CreateProposal(ctx, proposal)
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	if !reflect.DeepEqual(created, proposal) {
		t.Fatalf("created proposal differs:\ngot  %#v\nwant %#v", created, proposal)
	}
	replay := proposal
	replay.ID = uuid.NewString()
	replayed, err := repository.CreateProposal(ctx, replay)
	if err != nil {
		t.Fatalf("replay CreateProposal: %v", err)
	}
	if replayed.ID != proposal.ID {
		t.Fatalf("proposal replay id = %s, want %s", replayed.ID, proposal.ID)
	}

	conflict := replay
	conflict.Title = "different proposal under the same key"
	conflict.ProposalDigest, err = proposalDigest(conflict)
	if err != nil {
		t.Fatalf("digest conflicting proposal: %v", err)
	}
	if _, err := repository.CreateProposal(
		ctx,
		conflict,
	); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("proposal conflict error = %v, want ErrIdempotencyConflict", err)
	}

	if _, err := repository.GetProposal(
		ctx,
		"other-"+owner,
		proposal.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner GetProposal error = %v, want ErrNotFound", err)
	}

	unpairedState := postgresTestProposal(t, outcome, "unpaired-state")
	if _, err := repository.CreateProposal(ctx, unpairedState); err != nil {
		t.Fatalf("create unpaired-state proposal: %v", err)
	}
	unpairedTime := unpairedState.UpdatedAt.Add(time.Second)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(`
			UPDATE public.controlled_learning_proposals
			SET proposal_status = 'approved',
				revision = revision + 1,
				updated_at = ?,
				updated_at_unix_nano = ?
			WHERE owner_identity = ? AND id = ?`,
			unpairedTime,
			unpairedTime.UnixNano(),
			owner,
			unpairedState.ID,
		).Error
	}); err == nil {
		t.Fatal("database committed a proposal transition without a decision")
	}
	unchanged, err := repository.GetProposal(ctx, owner, unpairedState.ID)
	if err != nil || unchanged.Revision != 1 ||
		unchanged.Status != ProposalReviewRequired {
		t.Fatalf("unpaired state transaction was not atomic: %#v, %v", unchanged, err)
	}

	unpairedDecision := postgresTestProposal(t, outcome, "unpaired-decision")
	if _, err := repository.CreateProposal(ctx, unpairedDecision); err != nil {
		t.Fatalf("create unpaired-decision proposal: %v", err)
	}
	orphanDecision := ReviewDecision{
		ID:               uuid.NewString(),
		ProposalID:       unpairedDecision.ID,
		OwnerIdentity:    owner,
		ProposalRevision: 1,
		Kind:             DecisionApprove,
		ActorIdentity:    owner,
		HumanConfirmed:   true,
		Rationale:        "This direct decision must not commit alone.",
		ProposalDigest:   unpairedDecision.ProposalDigest,
		DecidedAt:        unpairedDecision.UpdatedAt.Add(time.Second),
	}
	orphanDecision.DecisionDigest, err = reviewDecisionDigest(orphanDecision)
	if err != nil {
		t.Fatalf("digest orphan decision: %v", err)
	}
	orphanPayload, err := json.Marshal(orphanDecision)
	if err != nil {
		t.Fatalf("marshal orphan decision: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO public.controlled_learning_review_decisions (
				id, owner_identity, proposal_id, proposal_revision,
				decision_kind, actor_identity, human_confirmed,
				proposal_digest, decision_digest, decided_at, payload
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb))`,
			orphanDecision.ID,
			owner,
			unpairedDecision.ID,
			orphanDecision.ProposalRevision,
			string(orphanDecision.Kind),
			orphanDecision.ActorIdentity,
			orphanDecision.HumanConfirmed,
			orphanDecision.ProposalDigest,
			orphanDecision.DecisionDigest,
			orphanDecision.DecidedAt,
			string(orphanPayload),
		).Error
	}); err == nil {
		t.Fatal("database committed a review decision without a state transition")
	}
	orphanDecisions, err := repository.ListDecisions(
		ctx,
		owner,
		unpairedDecision.ID,
	)
	if err != nil || len(orphanDecisions) != 0 {
		t.Fatalf("orphan decision transaction was not atomic: %#v, %v", orphanDecisions, err)
	}

	service, err := NewService(repository, func() time.Time {
		return proposal.UpdatedAt.Add(time.Second)
	}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	service, promoter := configureTestPromoter(
		t,
		service,
		proposal.UpdatedAt.Add(time.Second),
	)
	const contenders = 12
	var successes atomic.Int32
	var inProgress atomic.Int32
	var unexpected atomic.Value
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(contenders)
	for range contenders {
		go func() {
			defer wait.Done()
			<-start
			_, decideErr := service.Decide(ctx, DecideRequest{
				OwnerIdentity:    owner,
				ProposalID:       proposal.ID,
				ExpectedRevision: 1,
				Kind:             DecisionApprove,
				ActorIdentity:    owner,
				HumanConfirmed:   true,
				Rationale:        "Approved after verified evidence review.",
			})
			switch {
			case decideErr == nil:
				successes.Add(1)
			case errors.Is(decideErr, ErrApplicationInProgress):
				inProgress.Add(1)
			default:
				unexpected.Store(decideErr)
			}
		}()
	}
	close(start)
	wait.Wait()
	if value := unexpected.Load(); value != nil {
		t.Fatalf("unexpected concurrent review error: %v", value)
	}
	if successes.Load()+inProgress.Load() != contenders {
		t.Fatalf(
			"review results: successes=%d in-progress=%d",
			successes.Load(),
			inProgress.Load(),
		)
	}
	applyCalls, _, _ := promoter.calls()
	if applyCalls != 1 {
		t.Fatalf("promoter apply calls = %d, want 1", applyCalls)
	}

	updated, err := repository.GetProposal(ctx, owner, proposal.ID)
	if err != nil {
		t.Fatalf("GetProposal after review: %v", err)
	}
	if updated.Status != ProposalApproved || updated.Revision != 2 {
		t.Fatalf("updated proposal = %#v, want approved revision 2", updated)
	}
	decisions, err := repository.ListDecisions(ctx, owner, proposal.ID)
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(decisions) != 1 ||
		decisions[0].ProposalRevision != 1 ||
		!decisions[0].HumanConfirmed {
		t.Fatalf("review decisions = %#v, want one immutable decision", decisions)
	}
	if _, err := repository.ListDecisions(
		ctx,
		"other-"+owner,
		proposal.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner ListDecisions error = %v, want ErrNotFound", err)
	}

	if err := db.Exec(`
		UPDATE public.controlled_learning_proposals
		SET definition_payload =
			jsonb_set(definition_payload, '{title}', '"tampered"')
		WHERE owner_identity = ? AND id = ?`,
		owner,
		proposal.ID,
	).Error; err == nil {
		t.Fatal("database allowed immutable proposal definition update")
	}
	if err := db.Exec(`
		UPDATE public.controlled_learning_proposals
		SET proposal_status = 'rejected', revision = revision + 1
		WHERE owner_identity = ? AND id = ?`,
		owner,
		proposal.ID,
	).Error; err == nil {
		t.Fatal("database allowed illegal terminal proposal transition")
	}
	if err := db.Exec(`
		DELETE FROM public.controlled_learning_proposals
		WHERE owner_identity = ? AND id = ?`,
		owner,
		proposal.ID,
	).Error; err == nil {
		t.Fatal("database allowed proposal deletion")
	}
	if err := db.Exec(`
		UPDATE public.controlled_learning_review_decisions
		SET actor_identity = 'tampered'
		WHERE owner_identity = ? AND proposal_id = ?`,
		owner,
		proposal.ID,
	).Error; err == nil {
		t.Fatal("database allowed immutable review decision update")
	}
	if err := db.Exec(`
		DELETE FROM public.controlled_learning_review_decisions
		WHERE owner_identity = ? AND proposal_id = ?`,
		owner,
		proposal.ID,
	).Error; err == nil {
		t.Fatal("database allowed immutable review decision deletion")
	}
}

func TestPostgresRepositoryRejectsCrossOwnerEvidenceBinding(t *testing.T) {
	repository, _ := controlledLearningPostgresRepository(t)
	ctx := context.Background()
	owner := "evidence-owner-" + uuid.NewString() + "@example.com"
	outcome := postgresTestOutcome(t, owner, "cross-owner")
	if _, err := repository.CreateOutcome(ctx, outcome); err != nil {
		t.Fatalf("CreateOutcome: %v", err)
	}

	crossOwnerOutcome := outcome
	crossOwnerOutcome.OwnerIdentity = "other-" + owner
	crossOwnerOutcome.IdempotencyKey = "other-" + outcome.IdempotencyKey
	crossOwnerOutcome.ID = uuid.NewString()
	crossOwnerOutcome.EvidenceDigest, _ = outcomeDigest(crossOwnerOutcome)
	proposal := postgresTestProposal(t, crossOwnerOutcome, "cross-owner")
	if _, err := repository.CreateProposal(ctx, proposal); err == nil {
		t.Fatal("database accepted a cross-owner proposal evidence binding")
	}
	if _, err := repository.GetProposal(
		ctx,
		crossOwnerOutcome.OwnerIdentity,
		proposal.ID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed proposal transaction left a row behind: %v", err)
	}
}

func TestControlledLearningMigrationsApplyIdempotentlyPostgres(t *testing.T) {
	_, db := controlledLearningPostgresRepository(t)

	for _, table := range []string{
		"controlled_learning_outcomes",
		"controlled_learning_proposals",
		"controlled_learning_proposal_evidence",
		"controlled_learning_review_decisions",
	} {
		if !controlledLearningTableExists(t, db, table) {
			t.Fatalf("controlled learning table %q was not created", table)
		}
	}
	var triggerCount int64
	if err := db.Raw(`
		SELECT count(*)
		FROM pg_trigger
		WHERE NOT tgisinternal
		  AND tgname LIKE 'trg_controlled_learning_%'`).
		Scan(&triggerCount).Error; err != nil {
		t.Fatalf("count controlled learning triggers: %v", err)
	}
	if triggerCount != 18 {
		t.Fatalf("controlled learning trigger count = %d, want 18", triggerCount)
	}
	if rerun, err := infra.ApplyMigrations(
		db,
		migrations.Files,
		"pre",
	); err != nil || rerun != 0 {
		t.Fatalf("idempotent migration replay = (%d, %v), want 0, nil", rerun, err)
	}

	// Rollback semantics are covered by the migration runner and migration
	// contract tests. This integration database is fully migrated and shared
	// across packages; rolling back an older version would require destructively
	// removing every later schema and can race unrelated package tests.
}

func controlledLearningTableExists(
	t *testing.T,
	db *gorm.DB,
	table string,
) bool {
	t.Helper()
	var exists bool
	if err := db.Raw(`
		SELECT to_regclass(?) IS NOT NULL`,
		"public."+table,
	).Row().Scan(&exists); err != nil {
		t.Fatalf("check table %q: %v", table, err)
	}
	return exists
}

func controlledLearningPostgresRepository(
	t *testing.T,
) (*PostgresRepository, *gorm.DB) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("HAI_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open Postgres: %v", err)
	}
	if _, err := infra.ApplyMigrations(db, migrations.Files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}
	return NewPostgresRepository(db), db
}

func postgresTestOutcome(
	t *testing.T,
	ownerIdentity string,
	suffix string,
) OutcomeRecord {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute)
	record := OutcomeRecord{
		ID:              uuid.NewString(),
		ProtocolVersion: ProtocolVersion,
		OwnerIdentity:   ownerIdentity,
		IdempotencyKey:  "outcome-" + suffix,
		OperationID:     "operation-" + suffix,
		ProjectKey:      "hai",
		DomainPackIDs:   []string{"software_delivery"},
		Basis:           EvidenceVerifiedOutcome,
		Status:          OutcomeSucceeded,
		Summary:         "The controlled operation completed and was verified.",
		Verification:    VerificationTestPassed,
		Sources: []SourceReference{{
			ID:          "source-" + suffix,
			Kind:        "test_report",
			URI:         "file:///verified/" + suffix + ".json",
			RetrievedAt: now,
			ContentHash: strings.Repeat("a", 64),
		}},
		Criteria: []CriterionResult{{
			ID:          "criterion-" + suffix,
			Description: "The expected result exists.",
			Passed:      true,
			SourceIDs:   []string{"source-" + suffix},
		}},
		Metrics: []MetricResult{{
			Name:      "passed-tests",
			Expected:  1,
			Actual:    1,
			Tolerance: 0,
			Direction: MetricAtLeast,
			Unit:      "count",
		}},
		Tags:       []string{"verified"},
		OccurredAt: now,
		RecordedAt: now.Add(time.Second),
	}
	record.Reconciliation = reconcile(record)
	var err error
	record.EvidenceDigest, err = outcomeDigest(record)
	if err != nil {
		t.Fatalf("digest test outcome: %v", err)
	}
	return record
}

func postgresTestProposal(
	t *testing.T,
	outcome OutcomeRecord,
	suffix string,
) LearningProposal {
	t.Helper()
	now := outcome.RecordedAt.Add(time.Second)
	proposal := LearningProposal{
		ID:              uuid.NewString(),
		ProtocolVersion: ProtocolVersion,
		OwnerIdentity:   outcome.OwnerIdentity,
		IdempotencyKey:  "proposal-" + suffix,
		Revision:        1,
		Status:          ProposalReviewRequired,
		Method:          MethodAfterActionReview,
		Target:          TargetReusablePlan,
		Title:           "Improve the verified execution plan",
		Hypothesis:      "The reviewed change will improve completion quality.",
		ProposedChange:  "Apply the verified ordering to the reusable plan.",
		CurrentVersion:  "1.0.0",
		ProposedVersion: "1.1.0",
		RollbackPlan:    "Restore version 1.0.0.",
		EvaluationPlan:  "Run the same verified criteria before adoption.",
		EvidenceIDs:     []string{outcome.ID},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	var err error
	proposal.ProposalDigest, err = proposalDigest(proposal)
	if err != nil {
		t.Fatalf("digest test proposal: %v", err)
	}
	return proposal
}

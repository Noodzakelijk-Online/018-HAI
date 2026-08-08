package proactivity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPostgresRepositoryFailsClosedWithoutDatabase(t *testing.T) {
	t.Parallel()
	repository := NewPostgresRepository(nil)
	ctx := context.Background()
	now := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	record := PolicyRecord{
		ContractVersion: ContractVersion,
		OwnerIdentity:   "owner-a",
		Policy:          DefaultPreferences("owner-a"),
		RecordedAt:      now,
	}
	if _, _, err := repository.RecordPolicy(ctx, "owner-a", "policy-a", testDigest(1), record); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatalf("nil database write error = %v", err)
	}
	if _, err := repository.CurrentPolicy(ctx, "owner-a"); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatalf("nil database read error = %v", err)
	}
	if _, _, err := repository.FindDecisionBatch(ctx, "owner-a", "decision-a", testDigest(2)); !errors.Is(err, ErrRepositoryUnavailable) {
		t.Fatalf("nil database idempotency read error = %v", err)
	}
}

func TestPostgresRepositoryRejectsCallerSuppliedDigestMismatchBeforeStorage(t *testing.T) {
	t.Parallel()
	owner := "owner-a"
	now := time.Date(2026, time.August, 1, 12, 30, 0, 0, time.UTC)
	preferences, _, err := normalizePreferences(owner, DefaultPreferences(owner))
	if err != nil {
		t.Fatal(err)
	}
	record := PolicyRecord{
		ContractVersion: ContractVersion,
		OwnerIdentity:   owner,
		Policy:          preferences,
		RecordedAt:      now,
	}
	if err := validatePostgresPayloadDigest(idempotencyKindPolicy, owner, record.Policy, testDigest(999)); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("digest mismatch error = %v", err)
	}
}

func TestPostgresDecodersStrictlyRevalidatePayloadAndMetadata(t *testing.T) {
	t.Parallel()
	owner := "owner-a"
	now := time.Date(2026, time.August, 1, 13, 0, 0, 0, time.UTC)
	preferences, _, err := normalizePreferences(owner, DefaultPreferences(owner))
	if err != nil {
		t.Fatal(err)
	}
	policy := PolicyRecord{
		ContractVersion: ContractVersion,
		OwnerIdentity:   owner,
		Policy:          preferences,
		RecordedAt:      now,
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	policyDigest, err := advisoryDigest(idempotencyKindPolicy, owner, policy.Policy)
	if err != nil {
		t.Fatal(err)
	}
	row := postgresPolicyRow{
		OwnerIdentity:  owner,
		IdempotencyKey: "policy-a",
		PayloadDigest:  policyDigest,
		RecordedAt:     now,
		Payload:        string(payload),
	}
	decoded, err := decodePostgresPolicyRow(row, owner)
	if err != nil || !reflect.DeepEqual(decoded, policy) {
		t.Fatalf("valid policy decode = %#v, err=%v", decoded, err)
	}

	malformed := row
	malformed.Payload = `{"contractVersion":`
	if _, err := decodePostgresPolicyRow(malformed, owner); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("malformed payload error = %v", err)
	}
	unknown := row
	unknown.Payload = strings.TrimSuffix(string(payload), "}") + `,"unexpected":true}`
	if _, err := decodePostgresPolicyRow(unknown, owner); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("unknown field error = %v", err)
	}
	trailing := row
	trailing.Payload = string(payload) + ` {}`
	if _, err := decodePostgresPolicyRow(trailing, owner); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("trailing JSON error = %v", err)
	}
	wrongOwner := row
	wrongOwner.OwnerIdentity = "owner-b"
	if _, err := decodePostgresPolicyRow(wrongOwner, owner); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("owner metadata mismatch error = %v", err)
	}
	wrongTime := row
	wrongTime.RecordedAt = now.Add(time.Second)
	if _, err := decodePostgresPolicyRow(wrongTime, owner); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("timestamp metadata mismatch error = %v", err)
	}
}

func TestPostgresDecisionDecoderRejectsAuthorityBearingStorage(t *testing.T) {
	t.Parallel()
	owner := "owner-a"
	now := time.Date(2026, time.August, 1, 14, 0, 0, 0, time.UTC)
	batch := validPostgresDecisionBatch(owner, now)
	batch.Result.Decisions[0].ExecutionAuthorized = true
	payload, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	row := postgresDecisionBatchRow{
		OwnerIdentity:  owner,
		IdempotencyKey: "decision-a",
		PayloadDigest:  mustAdvisoryDigest(t, idempotencyKindDecisions, owner, batch.Result.DecidedAt),
		DecisionCount:  1,
		RecordedAt:     batch.RecordedAt,
		Payload:        string(payload),
	}
	if _, err := decodePostgresDecisionBatchRow(row, owner); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("authority-bearing stored batch error = %v", err)
	}
}

func TestPostgresSchemaContractNamesConstraintsAndIndexes(t *testing.T) {
	t.Parallel()
	required := []string{
		"CREATE TABLE public.proactivity_idempotency",
		"PRIMARY KEY (owner_identity, idempotency_key)",
		"record_kind IN ('policy', 'signals', 'decisions')",
		"CREATE TABLE public.proactivity_policy_records",
		"CREATE TABLE public.proactivity_signal_batches",
		"CREATE TABLE public.proactivity_signal_records",
		"CREATE TABLE public.proactivity_decision_batches",
		"CREATE TABLE public.proactivity_decision_records",
		"CREATE TABLE public.proactivity_feedback_records",
		"authority = 'attention_feedback_only'",
		"can_execute = false",
		"proactivity_policy_owner_recorded_idx",
		"proactivity_signal_owner_latest_idx",
		"proactivity_decision_owner_history_idx",
		"ON DELETE RESTRICT",
		"jsonb_typeof(payload)",
	}
	for _, fragment := range required {
		if !strings.Contains(PostgresSchemaContract, fragment) {
			t.Errorf("schema contract is missing %q", fragment)
		}
	}
	if strings.Contains(strings.ToUpper(PostgresSchemaContract), "CASCADE") {
		t.Fatal("schema contract must not cascade-delete the advisory audit ledger")
	}
}

func TestPostgresRepositoryLifecycleAndOwnerScopedIdempotency(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("HAI_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping proactivity Postgres integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open Postgres: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin Postgres schema transaction: %v", tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	existing := 0
	for _, relation := range []string{
		"public.proactivity_idempotency",
		"public.proactivity_policy_records",
		"public.proactivity_signal_batches",
		"public.proactivity_signal_records",
		"public.proactivity_decision_batches",
		"public.proactivity_decision_records",
	} {
		var resolved sql.NullString
		if err := tx.Raw(`SELECT to_regclass(?)::text`, relation).Row().Scan(&resolved); err != nil {
			t.Fatalf("inspect %s: %v", relation, err)
		}
		if resolved.Valid {
			existing++
		}
	}
	if existing != 0 && existing != 6 {
		t.Fatalf("partial proactivity schema exists: %d of 6 relations", existing)
	}
	if existing == 0 {
		if err := tx.Exec(PostgresSchemaContract).Error; err != nil {
			t.Fatalf("create transactional proactivity schema: %v", err)
		}
	}

	clock := time.Date(2026, time.August, 1, 15, 0, 0, 0, time.UTC)
	repository := NewPostgresRepository(tx)
	service := newService(repository, func() time.Time { return clock })
	ctx := context.Background()

	policy, created, err := service.RecordPolicy(ctx, "owner-a", "policy-a", DefaultPreferences("owner-a"))
	if err != nil || !created {
		t.Fatalf("record durable policy: created=%v err=%v", created, err)
	}
	clock = clock.Add(time.Minute)
	replayedPolicy, created, err := service.RecordPolicy(ctx, "owner-a", "policy-a", DefaultPreferences("owner-a"))
	if err != nil || created || !reflect.DeepEqual(replayedPolicy, policy) {
		t.Fatalf("replay durable policy: created=%v policy=%#v err=%v", created, replayedPolicy, err)
	}
	if _, _, err := service.RecordSignals(ctx, "owner-a", "policy-a", []OpenLoopSignal{testSignal("owner-a", "cross-kind", "cross-kind", clock)}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("cross-kind idempotency error = %v", err)
	}

	signal := testSignal("owner-a", "signal-a", "loop-a", clock)
	storedSignals, created, err := service.RecordSignals(ctx, "owner-a", "signals-a", []OpenLoopSignal{signal})
	if err != nil || !created || len(storedSignals) != 1 {
		t.Fatalf("record durable signals: created=%v signals=%#v err=%v", created, storedSignals, err)
	}
	snapshot, err := service.CaptureEvaluationSnapshot(ctx, "owner-a", clock)
	if err != nil {
		t.Fatalf("capture durable exact snapshot: %v", err)
	}
	clock = clock.Add(30 * time.Second)
	laterSignal := testSignal("owner-a", "signal-later", "loop-later", clock)
	if _, created, err := service.RecordSignals(ctx, "owner-a", "signals-later", []OpenLoopSignal{laterSignal}); err != nil || !created {
		t.Fatalf("record later durable signal: created=%v err=%v", created, err)
	}
	monitorSignal := testSignal("owner-a", "signal-monitor", "loop-monitor", clock)
	exactBatch, created, err := service.EvaluateStoredSnapshot(ctx, "owner-a", EvaluateStoredSnapshotRequest{
		IdempotencyKey: "decisions-exact", Now: clock, Snapshot: snapshot,
		AdditionalSignals: []OpenLoopSignal{monitorSignal},
	})
	if err != nil || !created {
		t.Fatalf("evaluate durable exact snapshot: created=%v batch=%#v err=%v", created, exactBatch, err)
	}
	assertDecisionSignalIDs(t, exactBatch.Result.Decisions, "signal-a", "signal-monitor")
	if exactBatch.SnapshotInputDigest != snapshot.InputDigest || !digestPattern.MatchString(exactBatch.AdditionalSignalsDigest) {
		t.Fatalf("durable exact decision is not input-bound: %#v", exactBatch)
	}

	clock = clock.Add(30 * time.Second)
	batch, created, err := service.EvaluateStored(ctx, "owner-a", EvaluateStoredRequest{IdempotencyKey: "decisions-a", Now: clock})
	if err != nil || !created || len(batch.Result.Decisions) != 2 {
		t.Fatalf("record durable decisions: created=%v batch=%#v err=%v", created, batch, err)
	}
	assertNoAuthority(t, batch.Result.Decisions[0])
	replayedBatch, found, err := repository.FindDecisionBatch(ctx, "owner-a", "decisions-a", mustAdvisoryDigest(t, "decisions", "owner-a", clock))
	if err != nil || !found || !reflect.DeepEqual(replayedBatch, batch) {
		t.Fatalf("find durable decision batch: found=%v batch=%#v err=%v", found, replayedBatch, err)
	}

	policies, err := repository.ListPolicies(ctx, "owner-a", 1)
	if err != nil || len(policies) != 1 {
		t.Fatalf("bounded policy list = %#v, err=%v", policies, err)
	}
	decisions, err := repository.ListDecisions(ctx, "owner-a", 1)
	if err != nil || len(decisions) != 1 {
		t.Fatalf("bounded decision list = %#v, err=%v", decisions, err)
	}
	if _, err := repository.CurrentPolicy(ctx, "owner-b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner current policy error = %v", err)
	}

	var corruptRow postgresPolicyRow
	if err := tx.Raw(`
		SELECT owner_identity, idempotency_key, payload_digest, recorded_at,
			(payload || '{"unexpected":true}'::jsonb)::text AS payload
		FROM public.proactivity_policy_records
		WHERE owner_identity = ? AND idempotency_key = ?`, "owner-a", "policy-a").
		Scan(&corruptRow).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := decodePostgresPolicyRow(corruptRow, "owner-a"); !errors.Is(err, ErrCorruptStorage) {
		t.Fatalf("corrupt durable policy error = %v", err)
	}
}

func validPostgresDecisionBatch(owner string, now time.Time) DecisionBatch {
	return DecisionBatch{
		ContractVersion: ContractVersion,
		OwnerIdentity:   owner,
		Result: EvaluationResult{
			ContractVersion: ContractVersion,
			OwnerIdentity:   owner,
			DecidedAt:       now,
			TimeZone:        "UTC",
			Decisions: []Decision{{
				ContractVersion: ContractVersion,
				OwnerIdentity:   owner,
				SignalID:        "signal-a",
				OpenLoopKey:     "loop-a",
				SignalDigest:    testDigest(10),
				Title:           "Review open loop",
				Summary:         "A bounded advisory recommendation.",
				Outcome:         OutcomeAmbient,
				Score:           0.5,
				Components: []ScoreComponent{{
					Name:         "impact",
					Value:        0.5,
					Weight:       0.5,
					Contribution: 0.25,
					Explanation:  "bounded impact score",
				}},
				Reasons:   []string{"suitable for ambient visibility"},
				DecidedAt: now,
			}},
		},
		RecordedAt: now,
	}
}

func mustAdvisoryDigest(t *testing.T, kind, owner string, value any) string {
	t.Helper()
	digest, err := advisoryDigest(kind, owner, value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

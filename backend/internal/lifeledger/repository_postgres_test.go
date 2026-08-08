package lifeledger

import (
	"context"
	"errors"
	"os"
	"strings"
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
	record := postgresCommitmentRecord(t, "owner@example.test", "missing/database", 1, "missing-database")
	if _, _, err := repository.SaveCommitment(t.Context(), record, 0); err == nil {
		t.Fatal("SaveCommitment with nil database succeeded")
	}
	if _, err := repository.GetCommitment(t.Context(), record.OwnerIdentity, record.CommitmentKey); err == nil {
		t.Fatal("GetCommitment with nil database succeeded")
	}
	if _, err := repository.ListCommitments(t.Context(), record.OwnerIdentity, 10); err == nil {
		t.Fatal("ListCommitments with nil database succeeded")
	}
	if _, _, err := repository.AppendCost(t.Context(), postgresCostRecord(t, record.OwnerIdentity, CostEstimate, "missing-database-cost")); err == nil {
		t.Fatal("AppendCost with nil database succeeded")
	}
}

func TestPostgresRepositoryRevisionIdempotencyIsolationCostsAndImmutability(t *testing.T) {
	repository, db := lifeLedgerPostgresRepository(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	owner := "life-ledger-" + uuid.NewString() + "@example.test"
	otherOwner := "other-" + owner
	commitmentKey := "project/vendor-" + uuid.NewString()
	service, err := NewService(repository, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	create := postgresCommitmentRequest(owner, commitmentKey, 0, CommitmentProposed, "create-"+uuid.NewString(), now)
	created, err := service.RecordCommitment(ctx, create)
	if err != nil || !created.Created || created.Record.Revision != 1 {
		t.Fatalf("create commitment = %#v err=%v", created, err)
	}
	replayed, err := service.RecordCommitment(ctx, create)
	if err != nil || replayed.Created || replayed.Record.ID != created.Record.ID {
		t.Fatalf("idempotent replay = %#v err=%v", replayed, err)
	}
	conflicting := create
	conflicting.Title = "Conflicting idempotent request"
	if _, err := service.RecordCommitment(ctx, conflicting); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency conflict = %v, want ErrIdempotencyConflict", err)
	}
	wrongRevision := postgresCommitmentRequest(owner, commitmentKey, 2, CommitmentActive, "wrong-revision-"+uuid.NewString(), now)
	if _, err := service.RecordCommitment(ctx, wrongRevision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("revision conflict = %v, want ErrRevisionConflict", err)
	}
	activate := postgresCommitmentRequest(owner, commitmentKey, 1, CommitmentActive, "activate-"+uuid.NewString(), now)
	activated, err := service.RecordCommitment(ctx, activate)
	if err != nil || !activated.Created || activated.Record.Revision != 2 {
		t.Fatalf("activate commitment = %#v err=%v", activated, err)
	}

	current, err := service.GetCommitment(ctx, owner, commitmentKey)
	if err != nil || current.ID != activated.Record.ID || current.Revision != 2 {
		t.Fatalf("current commitment = %#v err=%v", current, err)
	}
	history, err := service.CommitmentHistory(ctx, owner, commitmentKey, 10)
	if err != nil || len(history) != 2 || history[0].Revision != 1 || history[1].Revision != 2 {
		t.Fatalf("history = %#v err=%v", history, err)
	}
	if _, err := service.GetCommitment(ctx, otherOwner, commitmentKey); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner GetCommitment = %v, want ErrNotFound", err)
	}
	if records, err := service.ListCommitments(ctx, otherOwner, 10); err != nil || len(records) != 0 {
		t.Fatalf("cross-owner ListCommitments = %#v err=%v", records, err)
	}
	if _, err := service.CommitmentHistory(ctx, otherOwner, commitmentKey, 10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner history = %v, want ErrNotFound", err)
	}

	estimateRequest := postgresCostRequest(owner, commitmentKey, CostEstimate, "estimate-"+uuid.NewString(), now)
	estimate, err := service.RecordCost(ctx, estimateRequest)
	if err != nil || !estimate.Created || estimate.Record.Kind != CostEstimate {
		t.Fatalf("estimate = %#v err=%v", estimate, err)
	}
	estimateReplay, err := service.RecordCost(ctx, estimateRequest)
	if err != nil || estimateReplay.Created || estimateReplay.Record.ID != estimate.Record.ID {
		t.Fatalf("estimate replay = %#v err=%v", estimateReplay, err)
	}
	incurredRequest := postgresCostRequest(owner, commitmentKey, CostIncurred, "incurred-"+uuid.NewString(), now.Add(time.Microsecond))
	incurred, err := service.RecordCost(ctx, incurredRequest)
	if err != nil || !incurred.Created || incurred.Record.Kind != CostIncurred || incurred.Record.ID == estimate.Record.ID {
		t.Fatalf("incurred = %#v err=%v", incurred, err)
	}
	costs, err := service.ListCosts(ctx, owner, 10)
	if err != nil || len(costs) != 2 {
		t.Fatalf("costs = %#v err=%v", costs, err)
	}
	kinds := map[CostKind]int{}
	for _, cost := range costs {
		kinds[cost.Kind]++
	}
	if kinds[CostEstimate] != 1 || kinds[CostIncurred] != 1 {
		t.Fatalf("estimate/incurred distinction lost: %#v", costs)
	}
	if records, err := service.ListCosts(ctx, otherOwner, 10); err != nil || len(records) != 0 {
		t.Fatalf("cross-owner ListCosts = %#v err=%v", records, err)
	}

	assertLifeLedgerMutationRejected(t, db, `
		UPDATE public.life_ledger_commitment_revisions
		SET commitment_key = commitment_key
		WHERE owner_identity = ? AND commitment_key = ? AND revision = 1`, owner, commitmentKey)
	assertLifeLedgerMutationRejected(t, db, `
		DELETE FROM public.life_ledger_commitment_revisions
		WHERE owner_identity = ? AND commitment_key = ? AND revision = 1`, owner, commitmentKey)
	assertLifeLedgerMutationRejected(t, db, `
		UPDATE public.life_ledger_cost_entries
		SET amount_minor = amount_minor
		WHERE owner_identity = ? AND id = ?`, owner, estimate.Record.ID)
	assertLifeLedgerMutationRejected(t, db, `
		DELETE FROM public.life_ledger_cost_entries
		WHERE owner_identity = ? AND id = ?`, owner, estimate.Record.ID)
	assertLifeLedgerMutationRejected(t, db, `TRUNCATE public.life_ledger_commitment_revisions`)
	assertLifeLedgerMutationRejected(t, db, `TRUNCATE public.life_ledger_cost_entries`)
}

func lifeLedgerPostgresRepository(t *testing.T) (*PostgresRepository, *gorm.DB) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("HAI_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("HAI_TEST_DATABASE_DSN not set; skipping Postgres integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open Postgres: %v", err)
	}
	if _, err := infra.ApplyMigrations(db, migrations.Files, "pre"); err != nil {
		t.Fatalf("apply pre migrations: %v", err)
	}
	return NewPostgresRepository(db), db
}

func postgresCommitmentRequest(owner, key string, expected uint64, status CommitmentStatus, idempotency string, now time.Time) RecordCommitmentRequest {
	request := commitmentRequest(owner, key, expected, status, idempotency)
	request.ObservedAt = now.Add(-time.Minute)
	request.Evidence[0].ObservedAt = now.Add(-2 * time.Minute)
	return request
}

func postgresCostRequest(owner, commitmentKey string, kind CostKind, idempotency string, now time.Time) RecordCostRequest {
	request := costRequest(owner, kind, idempotency)
	request.CommitmentKey = commitmentKey
	request.ObservedAt = now.Add(-time.Minute)
	request.Evidence[0].ObservedAt = now.Add(-2 * time.Minute)
	return request
}

func postgresCommitmentRecord(t *testing.T, owner, key string, revision uint64, idempotency string) CommitmentRevision {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	request := postgresCommitmentRequest(owner, key, revision-1, CommitmentActive, idempotency, now)
	requestDigest, err := commitmentRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	record := CommitmentRevision{
		ContractVersion: ContractVersion,
		ID:              uuid.New(),
		OwnerIdentity:   owner,
		CommitmentKey:   key,
		Revision:        revision,
		Domain:          request.Domain,
		Title:           request.Title,
		Summary:         request.Summary,
		Status:          request.Status,
		Counterparty:    request.Counterparty,
		ProjectKey:      request.ProjectKey,
		Verification:    request.Verification,
		Evidence:        request.Evidence,
		LocalOnly:       true,
		IdempotencyKey:  idempotency,
		RequestDigest:   requestDigest,
		ObservedAt:      request.ObservedAt,
		RecordedAt:      now,
	}
	record.RecordDigest, err = commitmentRecordDigest(record)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func postgresCostRecord(t *testing.T, owner string, kind CostKind, idempotency string) CostEntry {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	request := postgresCostRequest(owner, "", kind, idempotency, now)
	requestDigest, err := costRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	record := CostEntry{
		ContractVersion: ContractVersion,
		ID:              uuid.New(),
		OwnerIdentity:   owner,
		Domain:          request.Domain,
		Title:           request.Title,
		Summary:         request.Summary,
		Kind:            request.Kind,
		AmountMinor:     request.AmountMinor,
		Currency:        "EUR",
		Verification:    request.Verification,
		Evidence:        request.Evidence,
		LocalOnly:       true,
		IdempotencyKey:  idempotency,
		RequestDigest:   requestDigest,
		ObservedAt:      request.ObservedAt,
		RecordedAt:      now,
	}
	record.RecordDigest, err = costRecordDigest(record)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func assertLifeLedgerMutationRejected(t *testing.T, db *gorm.DB, statement string, args ...any) {
	t.Helper()
	if err := db.Exec(statement, args...).Error; err == nil {
		t.Fatalf("append-only ledger accepted mutation: %s", strings.TrimSpace(statement))
	}
}

package lifeledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"automation-hub-backend/internal/infra"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PostgresRepository struct{ DB *gorm.DB }

func NewPostgresRepository(db *gorm.DB) *PostgresRepository { return &PostgresRepository{DB: db} }

func DefaultRepository() (Repository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, fmt.Errorf("open life ledger database: %w", err)
	}
	return NewPostgresRepository(db), nil
}

type commitmentRow struct {
	OwnerIdentity  string
	CommitmentKey  string
	Revision       uint64
	IdempotencyKey string
	RequestDigest  string
	RecordDigest   string
	Payload        string
}

type costRow struct {
	OwnerIdentity  string
	ID             string
	IdempotencyKey string
	RequestDigest  string
	RecordDigest   string
	Payload        string
}

func (r *PostgresRepository) SaveCommitment(ctx context.Context, record CommitmentRevision, expectedRevision uint64) (CommitmentRevision, bool, error) {
	if err := r.ready(ctx); err != nil {
		return CommitmentRevision{}, false, err
	}
	if err := verifyCommitment(record); err != nil {
		return CommitmentRevision{}, false, err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return CommitmentRevision{}, false, err
	}
	var stored CommitmentRevision
	created := false
	err = r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if existing, found, loadErr := loadCommitmentByIdempotency(tx, record.OwnerIdentity, record.IdempotencyKey); loadErr != nil {
			return loadErr
		} else if found {
			if existing.RequestDigest != record.RequestDigest {
				return ErrIdempotencyConflict
			}
			stored = existing
			return nil
		}
		result := tx.Exec(`
			INSERT INTO public.life_ledger_commitment_revisions (
				owner_identity, commitment_key, revision, idempotency_key,
				request_digest, record_digest, observed_at, recorded_at, payload
			)
			SELECT ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb)
			WHERE COALESCE((
				SELECT revision FROM public.life_ledger_commitment_revisions
				WHERE owner_identity = ? AND commitment_key = ?
				ORDER BY revision DESC LIMIT 1
			), 0) = ?
			ON CONFLICT DO NOTHING`,
			record.OwnerIdentity, record.CommitmentKey, record.Revision, record.IdempotencyKey,
			record.RequestDigest, record.RecordDigest, record.ObservedAt, record.RecordedAt, string(payload),
			record.OwnerIdentity, record.CommitmentKey, expectedRevision,
		)
		if result.Error != nil {
			return fmt.Errorf("append commitment revision: %w", result.Error)
		}
		if result.RowsAffected == 1 {
			stored, created = record, true
			return nil
		}
		if existing, found, loadErr := loadCommitmentByIdempotency(tx, record.OwnerIdentity, record.IdempotencyKey); loadErr != nil {
			return loadErr
		} else if found {
			if existing.RequestDigest != record.RequestDigest {
				return ErrIdempotencyConflict
			}
			stored = existing
			return nil
		}
		return ErrRevisionConflict
	})
	return stored, created, err
}

func (r *PostgresRepository) GetCommitment(ctx context.Context, owner, key string) (CommitmentRevision, error) {
	if err := r.ready(ctx); err != nil {
		return CommitmentRevision{}, err
	}
	var row commitmentRow
	err := scanCommitmentRow(r.DB.WithContext(ctx).Raw(`
		SELECT owner_identity, commitment_key, revision, idempotency_key,
			request_digest, record_digest, payload::text
		FROM public.life_ledger_commitment_revisions
		WHERE owner_identity = ? AND commitment_key = ?
		ORDER BY revision DESC LIMIT 1`, strings.TrimSpace(owner), strings.TrimSpace(key)).Row(), &row)
	if errors.Is(err, sql.ErrNoRows) {
		return CommitmentRevision{}, ErrNotFound
	}
	if err != nil {
		return CommitmentRevision{}, fmt.Errorf("read commitment: %w", err)
	}
	return decodeCommitment(row)
}

func (r *PostgresRepository) ListCommitments(ctx context.Context, owner string, limit int) ([]CommitmentRevision, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	var rows []commitmentRow
	if err := r.DB.WithContext(ctx).Raw(`
		SELECT owner_identity, commitment_key, revision, idempotency_key,
			request_digest, record_digest, payload
		FROM (
			SELECT DISTINCT ON (commitment_key)
				owner_identity, commitment_key, revision, idempotency_key,
				request_digest, record_digest, payload::text AS payload,
				observed_at
			FROM public.life_ledger_commitment_revisions
			WHERE owner_identity = ?
			ORDER BY commitment_key, revision DESC
		) latest
		ORDER BY observed_at DESC, commitment_key ASC
		LIMIT ?`, strings.TrimSpace(owner), boundedLimit(limit)).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list commitments: %w", err)
	}
	result := make([]CommitmentRevision, 0, len(rows))
	for _, row := range rows {
		record, err := decodeCommitment(row)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func (r *PostgresRepository) ListCommitmentHistory(ctx context.Context, owner, key string, limit int) ([]CommitmentRevision, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	var rows []commitmentRow
	if err := r.DB.WithContext(ctx).Raw(`
		SELECT * FROM (
			SELECT owner_identity, commitment_key, revision, idempotency_key,
				request_digest, record_digest, payload::text AS payload
			FROM public.life_ledger_commitment_revisions
			WHERE owner_identity = ? AND commitment_key = ?
			ORDER BY revision DESC LIMIT ?
		) history ORDER BY revision ASC`, strings.TrimSpace(owner), strings.TrimSpace(key), boundedLimit(limit)).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list commitment history: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	result := make([]CommitmentRevision, 0, len(rows))
	for _, row := range rows {
		record, err := decodeCommitment(row)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func (r *PostgresRepository) AppendCost(ctx context.Context, record CostEntry) (CostEntry, bool, error) {
	if err := r.ready(ctx); err != nil {
		return CostEntry{}, false, err
	}
	if err := verifyCost(record); err != nil {
		return CostEntry{}, false, err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return CostEntry{}, false, err
	}
	result := r.DB.WithContext(ctx).Exec(`
		INSERT INTO public.life_ledger_cost_entries (
			id, owner_identity, idempotency_key, request_digest, record_digest,
			kind, currency, amount_minor, observed_at, recorded_at, payload
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb))
		ON CONFLICT DO NOTHING`, record.ID, record.OwnerIdentity, record.IdempotencyKey,
		record.RequestDigest, record.RecordDigest, record.Kind, record.Currency,
		record.AmountMinor, record.ObservedAt, record.RecordedAt, string(payload))
	if result.Error != nil {
		return CostEntry{}, false, fmt.Errorf("append cost entry: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return record, true, nil
	}
	existing, found, err := loadCostByIdempotency(r.DB.WithContext(ctx), record.OwnerIdentity, record.IdempotencyKey)
	if err != nil {
		return CostEntry{}, false, err
	}
	if !found || existing.RequestDigest != record.RequestDigest {
		return CostEntry{}, false, ErrIdempotencyConflict
	}
	return existing, false, nil
}

func (r *PostgresRepository) ListCosts(ctx context.Context, owner string, limit int) ([]CostEntry, error) {
	if err := r.ready(ctx); err != nil {
		return nil, err
	}
	var rows []costRow
	if err := r.DB.WithContext(ctx).Raw(`
		SELECT owner_identity, id::text AS id, idempotency_key, request_digest,
			record_digest, payload::text AS payload
		FROM public.life_ledger_cost_entries
		WHERE owner_identity = ?
		ORDER BY observed_at DESC, id DESC LIMIT ?`, strings.TrimSpace(owner), boundedLimit(limit)).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list cost entries: %w", err)
	}
	result := make([]CostEntry, 0, len(rows))
	for _, row := range rows {
		record, err := decodeCost(row)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, nil
}

func (r *PostgresRepository) ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r == nil || r.DB == nil {
		return fmt.Errorf("life ledger database is unavailable")
	}
	return nil
}

func loadCommitmentByIdempotency(db *gorm.DB, owner, key string) (CommitmentRevision, bool, error) {
	var row commitmentRow
	err := scanCommitmentRow(db.Raw(`
		SELECT owner_identity, commitment_key, revision, idempotency_key,
			request_digest, record_digest, payload::text
		FROM public.life_ledger_commitment_revisions
		WHERE owner_identity = ? AND idempotency_key = ?`, owner, key).Row(), &row)
	if errors.Is(err, sql.ErrNoRows) {
		return CommitmentRevision{}, false, nil
	}
	if err != nil {
		return CommitmentRevision{}, false, err
	}
	record, err := decodeCommitment(row)
	return record, err == nil, err
}

func loadCostByIdempotency(db *gorm.DB, owner, key string) (CostEntry, bool, error) {
	var row costRow
	err := db.Raw(`
		SELECT owner_identity, id::text, idempotency_key, request_digest,
			record_digest, payload::text
		FROM public.life_ledger_cost_entries
		WHERE owner_identity = ? AND idempotency_key = ?`, owner, key).Row().Scan(
		&row.OwnerIdentity, &row.ID, &row.IdempotencyKey, &row.RequestDigest,
		&row.RecordDigest, &row.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return CostEntry{}, false, nil
	}
	if err != nil {
		return CostEntry{}, false, err
	}
	record, err := decodeCost(row)
	return record, err == nil, err
}

func scanCommitmentRow(row *sql.Row, target *commitmentRow) error {
	return row.Scan(&target.OwnerIdentity, &target.CommitmentKey, &target.Revision,
		&target.IdempotencyKey, &target.RequestDigest, &target.RecordDigest, &target.Payload)
}

func decodeCommitment(row commitmentRow) (CommitmentRevision, error) {
	var record CommitmentRevision
	if err := strictDecode(row.Payload, &record); err != nil {
		return CommitmentRevision{}, fmt.Errorf("decode commitment record: %w", err)
	}
	if record.OwnerIdentity != row.OwnerIdentity || record.CommitmentKey != row.CommitmentKey ||
		record.Revision != row.Revision || record.IdempotencyKey != row.IdempotencyKey ||
		record.RequestDigest != row.RequestDigest || record.RecordDigest != row.RecordDigest {
		return CommitmentRevision{}, ErrCorruptRecord
	}
	if err := verifyCommitment(record); err != nil {
		return CommitmentRevision{}, err
	}
	return record, nil
}

func decodeCost(row costRow) (CostEntry, error) {
	var record CostEntry
	if err := strictDecode(row.Payload, &record); err != nil {
		return CostEntry{}, fmt.Errorf("decode cost record: %w", err)
	}
	if record.OwnerIdentity != row.OwnerIdentity || record.ID.String() != row.ID ||
		record.IdempotencyKey != row.IdempotencyKey || record.RequestDigest != row.RequestDigest ||
		record.RecordDigest != row.RecordDigest {
		return CostEntry{}, ErrCorruptRecord
	}
	if err := verifyCost(record); err != nil {
		return CostEntry{}, err
	}
	return record, nil
}

func strictDecode(value string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func verifyCommitment(record CommitmentRevision) error {
	if record.ContractVersion != ContractVersion || record.ID == uuid.Nil || record.Revision == 0 ||
		!record.LocalOnly || record.RecordedAt.IsZero() || record.LifeGraph != nil || record.LifeGraphWarning != "" {
		return ErrCorruptRecord
	}
	request := RecordCommitmentRequest{
		OwnerIdentity: record.OwnerIdentity, CommitmentKey: record.CommitmentKey,
		ExpectedRevision: record.Revision - 1, Domain: record.Domain, Title: record.Title,
		Summary: record.Summary, Status: record.Status, Counterparty: record.Counterparty,
		ProjectKey: record.ProjectKey, DueAt: record.DueAt, Verification: record.Verification,
		Evidence: record.Evidence, IdempotencyKey: record.IdempotencyKey, ObservedAt: record.ObservedAt,
	}
	normalized, err := normalizeCommitmentRequest(request, record.RecordedAt)
	if err != nil {
		return ErrCorruptRecord
	}
	expectedRequestDigest, err := commitmentRequestDigest(normalized)
	if err != nil || expectedRequestDigest != record.RequestDigest {
		return ErrCorruptRecord
	}
	expected, err := commitmentRecordDigest(record)
	if err != nil || expected != record.RecordDigest {
		return ErrCorruptRecord
	}
	return nil
}

func verifyCost(record CostEntry) error {
	if record.ContractVersion != ContractVersion || record.ID == uuid.Nil || !record.LocalOnly ||
		record.RecordedAt.IsZero() || record.LifeGraph != nil || record.LifeGraphWarning != "" {
		return ErrCorruptRecord
	}
	request := RecordCostRequest{
		OwnerIdentity: record.OwnerIdentity, Domain: record.Domain, Title: record.Title,
		Summary: record.Summary, Kind: record.Kind, AmountMinor: record.AmountMinor,
		Currency: record.Currency, CommitmentKey: record.CommitmentKey, ProjectKey: record.ProjectKey,
		Verification: record.Verification, Evidence: record.Evidence,
		IdempotencyKey: record.IdempotencyKey, ObservedAt: record.ObservedAt,
	}
	normalized, err := normalizeCostRequest(request, record.RecordedAt)
	if err != nil {
		return ErrCorruptRecord
	}
	expectedRequestDigest, err := costRequestDigest(normalized)
	if err != nil || expectedRequestDigest != record.RequestDigest {
		return ErrCorruptRecord
	}
	expected, err := costRecordDigest(record)
	if err != nil || expected != record.RecordDigest {
		return ErrCorruptRecord
	}
	return nil
}

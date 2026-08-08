package controlledlearning

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"automation-hub-backend/internal/infra"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PostgresRepository stores controlled-learning evidence, proposal
// definitions, evidence bindings, and review decisions in the canonical
// PostgreSQL database.
type PostgresRepository struct {
	DB *gorm.DB
}

var _ Repository = (*PostgresRepository)(nil)
var _ ApplicationRepository = (*PostgresRepository)(nil)

func NewPostgresRepository(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{DB: db}
}

// DefaultRepository opens the configured database and returns the durable
// controlled-learning repository. Database startup applies the embedded
// versioned migrations before this repository is used.
func DefaultRepository() (Repository, error) {
	db, err := infra.GetDefaultDB()
	if err != nil {
		return nil, fmt.Errorf("open controlled learning database: %w", err)
	}
	return NewPostgresRepository(db), nil
}

func (repository *PostgresRepository) CreateOutcome(
	ctx context.Context,
	record OutcomeRecord,
) (OutcomeRecord, error) {
	if err := repository.ready(); err != nil {
		return OutcomeRecord{}, err
	}
	if err := validateStoredOutcome(record); err != nil {
		return OutcomeRecord{}, err
	}
	payload, err := marshalControlledLearning("outcome", record)
	if err != nil {
		return OutcomeRecord{}, err
	}
	result := repository.DB.WithContext(ctx).Exec(`
		INSERT INTO public.controlled_learning_outcomes (
			id, protocol_version, owner_identity, idempotency_key,
			operation_id, project_key, basis, outcome_status,
			verification_status, evidence_digest, occurred_at,
			recorded_at, payload
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb))
		ON CONFLICT DO NOTHING`,
		record.ID,
		record.ProtocolVersion,
		record.OwnerIdentity,
		record.IdempotencyKey,
		record.OperationID,
		record.ProjectKey,
		string(record.Basis),
		string(record.Status),
		string(record.Verification),
		record.EvidenceDigest,
		record.OccurredAt.UTC(),
		record.RecordedAt.UTC(),
		string(payload),
	)
	if result.Error != nil {
		return OutcomeRecord{}, fmt.Errorf("create controlled learning outcome: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return cloneOutcome(record), nil
	}

	existing, err := repository.getOutcomeByIdempotency(
		ctx,
		record.OwnerIdentity,
		record.IdempotencyKey,
	)
	if errors.Is(err, ErrNotFound) {
		return OutcomeRecord{}, ErrIdempotencyConflict
	}
	if err != nil {
		return OutcomeRecord{}, err
	}
	if existing.EvidenceDigest != record.EvidenceDigest {
		return OutcomeRecord{}, ErrIdempotencyConflict
	}
	return existing, nil
}

func (repository *PostgresRepository) GetOutcome(
	ctx context.Context,
	ownerIdentity string,
	id string,
) (OutcomeRecord, error) {
	if err := repository.ready(); err != nil {
		return OutcomeRecord{}, err
	}
	owner, parsedID, err := validatePostgresLookup(ownerIdentity, id)
	if err != nil {
		return OutcomeRecord{}, err
	}
	var payload []byte
	err = repository.DB.WithContext(ctx).Raw(`
		SELECT payload
		FROM public.controlled_learning_outcomes
		WHERE owner_identity = ? AND id = ?`,
		owner,
		parsedID,
	).Row().Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return OutcomeRecord{}, ErrNotFound
	}
	if err != nil {
		return OutcomeRecord{}, fmt.Errorf("get controlled learning outcome: %w", err)
	}
	return decodeOutcome(payload)
}

func (repository *PostgresRepository) ListOutcomes(
	ctx context.Context,
	query OutcomeQuery,
) ([]OutcomeRecord, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	owner := strings.TrimSpace(query.OwnerIdentity)
	if owner == "" {
		return nil, ErrOwnerScopeViolation
	}
	statement := `
		SELECT payload
		FROM public.controlled_learning_outcomes
		WHERE owner_identity = ?`
	args := []any{owner}
	if operationID := strings.TrimSpace(query.OperationID); operationID != "" {
		statement += ` AND operation_id = ?`
		args = append(args, operationID)
	}
	statement += ` ORDER BY recorded_at DESC, id ASC LIMIT ?`
	args = append(args, normalizedLimit(query.Limit))

	rows, err := repository.DB.WithContext(ctx).Raw(statement, args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("list controlled learning outcomes: %w", err)
	}
	defer rows.Close()

	result := make([]OutcomeRecord, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan controlled learning outcome: %w", err)
		}
		record, err := decodeOutcome(payload)
		if err != nil {
			return nil, err
		}
		if record.OwnerIdentity != owner {
			return nil, ErrOwnerScopeViolation
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate controlled learning outcomes: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) CreateProposal(
	ctx context.Context,
	proposal LearningProposal,
) (LearningProposal, error) {
	if err := repository.ready(); err != nil {
		return LearningProposal{}, err
	}
	if err := validateStoredProposal(proposal); err != nil {
		return LearningProposal{}, err
	}
	payload, err := marshalControlledLearning("proposal", proposal)
	if err != nil {
		return LearningProposal{}, err
	}

	created := false
	err = repository.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`
			INSERT INTO public.controlled_learning_proposals (
				id, protocol_version, owner_identity, idempotency_key,
				revision, proposal_status, learning_method, target_kind,
				protected_target, proposal_digest, created_at, updated_at,
				updated_at_unix_nano, definition_payload
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb))
			ON CONFLICT DO NOTHING`,
			proposal.ID,
			proposal.ProtocolVersion,
			proposal.OwnerIdentity,
			proposal.IdempotencyKey,
			proposal.Revision,
			string(proposal.Status),
			string(proposal.Method),
			string(proposal.Target),
			proposal.ProtectedTarget,
			proposal.ProposalDigest,
			proposal.CreatedAt.UTC(),
			proposal.UpdatedAt.UTC(),
			proposal.UpdatedAt.UnixNano(),
			string(payload),
		)
		if result.Error != nil {
			return fmt.Errorf("create controlled learning proposal: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		for ordinal, outcomeID := range proposal.EvidenceIDs {
			result = tx.Exec(`
				INSERT INTO public.controlled_learning_proposal_evidence (
					owner_identity, proposal_id, outcome_id, ordinal
				) VALUES (?, ?, ?, ?)`,
				proposal.OwnerIdentity,
				proposal.ID,
				outcomeID,
				ordinal,
			)
			if result.Error != nil {
				return fmt.Errorf(
					"bind controlled learning proposal evidence %q: %w",
					outcomeID,
					result.Error,
				)
			}
		}
		created = true
		return nil
	})
	if err != nil {
		return LearningProposal{}, err
	}
	if created {
		return cloneProposal(proposal), nil
	}

	existing, err := repository.getProposalByIdempotency(
		ctx,
		proposal.OwnerIdentity,
		proposal.IdempotencyKey,
	)
	if errors.Is(err, ErrNotFound) {
		return LearningProposal{}, ErrIdempotencyConflict
	}
	if err != nil {
		return LearningProposal{}, err
	}
	if existing.ProposalDigest != proposal.ProposalDigest {
		return LearningProposal{}, ErrIdempotencyConflict
	}
	return existing, nil
}

func (repository *PostgresRepository) GetProposal(
	ctx context.Context,
	ownerIdentity string,
	id string,
) (LearningProposal, error) {
	if err := repository.ready(); err != nil {
		return LearningProposal{}, err
	}
	owner, parsedID, err := validatePostgresLookup(ownerIdentity, id)
	if err != nil {
		return LearningProposal{}, err
	}
	row := repository.DB.WithContext(ctx).Raw(
		proposalSelect+` WHERE owner_identity = ? AND id = ?`,
		owner,
		parsedID,
	).Row()
	proposal, err := scanProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return LearningProposal{}, ErrNotFound
	}
	if err != nil {
		return LearningProposal{}, fmt.Errorf("get controlled learning proposal: %w", err)
	}
	return proposal, nil
}

func (repository *PostgresRepository) ListProposals(
	ctx context.Context,
	query ProposalQuery,
) ([]LearningProposal, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	owner := strings.TrimSpace(query.OwnerIdentity)
	if owner == "" {
		return nil, ErrOwnerScopeViolation
	}
	statement := proposalSelect + ` WHERE owner_identity = ?`
	args := []any{owner}
	if query.Status != "" {
		statement += ` AND proposal_status = ?`
		args = append(args, string(query.Status))
	}
	statement += ` ORDER BY updated_at DESC, id ASC LIMIT ?`
	args = append(args, normalizedLimit(query.Limit))

	rows, err := repository.DB.WithContext(ctx).Raw(statement, args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("list controlled learning proposals: %w", err)
	}
	defer rows.Close()

	result := make([]LearningProposal, 0)
	for rows.Next() {
		proposal, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		if proposal.OwnerIdentity != owner {
			return nil, ErrOwnerScopeViolation
		}
		result = append(result, proposal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate controlled learning proposals: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) DecideProposal(
	ctx context.Context,
	ownerIdentity string,
	proposalID string,
	expectedRevision int64,
	decision ReviewDecision,
	nextStatus ProposalStatus,
) (LearningProposal, error) {
	if err := repository.ready(); err != nil {
		return LearningProposal{}, err
	}
	owner, parsedProposalID, err := validatePostgresLookup(
		ownerIdentity,
		proposalID,
	)
	if err != nil {
		return LearningProposal{}, err
	}
	if expectedRevision <= 0 {
		return LearningProposal{}, ErrRevisionConflict
	}
	if decision.Kind == DecisionApprove ||
		decision.Kind == DecisionEscalateGovernance ||
		nextStatus == ProposalApproved ||
		nextStatus == ProposalGovernanceReview {
		return LearningProposal{}, ErrInvalidStateChange
	}
	if err := validateStoredDecision(
		owner,
		proposalID,
		expectedRevision,
		decision,
	); err != nil {
		return LearningProposal{}, err
	}
	payload, err := marshalControlledLearning("review decision", decision)
	if err != nil {
		return LearningProposal{}, err
	}

	var updated LearningProposal
	err = repository.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := tx.Raw(
			proposalSelect+`
			WHERE owner_identity = ? AND id = ?
			FOR UPDATE`,
			owner,
			parsedProposalID,
		).Row()
		current, scanErr := scanProposal(row)
		if errors.Is(scanErr, sql.ErrNoRows) {
			return ErrNotFound
		}
		if scanErr != nil {
			return fmt.Errorf("lock controlled learning proposal: %w", scanErr)
		}
		if current.Revision != expectedRevision {
			return ErrRevisionConflict
		}
		if current.ProposalDigest != decision.ProposalDigest {
			return ErrIntegrityViolation
		}
		if err := validateDecisionTransition(current, decision, nextStatus); err != nil {
			return err
		}

		if err := insertReviewDecision(tx, decision, payload); err != nil {
			return err
		}

		result := tx.Exec(`
			UPDATE public.controlled_learning_proposals
			SET proposal_status = ?,
				revision = revision + 1,
				updated_at = ?,
				updated_at_unix_nano = ?
			WHERE owner_identity = ?
			  AND id = ?
			  AND revision = ?
			  AND proposal_digest = ?`,
			string(nextStatus),
			decision.DecidedAt.UTC(),
			decision.DecidedAt.UnixNano(),
			owner,
			parsedProposalID,
			expectedRevision,
			decision.ProposalDigest,
		)
		if result.Error != nil {
			return fmt.Errorf("advance controlled learning proposal: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		current.Status = nextStatus
		current.Revision++
		current.UpdatedAt = decision.DecidedAt.UTC()
		updated = current
		return nil
	})
	if err != nil {
		return LearningProposal{}, err
	}
	return cloneProposal(updated), nil
}

func insertReviewDecision(
	tx *gorm.DB,
	decision ReviewDecision,
	payload []byte,
) error {
	result := tx.Exec(`
		INSERT INTO public.controlled_learning_review_decisions (
			id, owner_identity, proposal_id, proposal_revision,
			decision_kind, actor_identity, human_confirmed,
			proposal_digest, application_id, decision_digest,
			decided_at, payload
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, '')::uuid, ?, ?, CAST(? AS jsonb)
		)`,
		decision.ID,
		decision.OwnerIdentity,
		decision.ProposalID,
		decision.ProposalRevision,
		string(decision.Kind),
		decision.ActorIdentity,
		decision.HumanConfirmed,
		decision.ProposalDigest,
		decision.ApplicationID,
		decision.DecisionDigest,
		decision.DecidedAt.UTC(),
		string(payload),
	)
	if result.Error != nil {
		return fmt.Errorf("append controlled learning review decision: %w", result.Error)
	}
	return nil
}

func (repository *PostgresRepository) ListDecisions(
	ctx context.Context,
	ownerIdentity string,
	proposalID string,
) ([]ReviewDecision, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	owner, parsedProposalID, err := validatePostgresLookup(
		ownerIdentity,
		proposalID,
	)
	if err != nil {
		return nil, err
	}
	var exists bool
	if err := repository.DB.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM public.controlled_learning_proposals
			WHERE owner_identity = ? AND id = ?
		)`,
		owner,
		parsedProposalID,
	).Row().Scan(&exists); err != nil {
		return nil, fmt.Errorf("check controlled learning proposal: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := repository.DB.WithContext(ctx).Raw(`
		SELECT payload
		FROM public.controlled_learning_review_decisions
		WHERE owner_identity = ? AND proposal_id = ?
		ORDER BY proposal_revision ASC, id ASC`,
		owner,
		parsedProposalID,
	).Rows()
	if err != nil {
		return nil, fmt.Errorf("list controlled learning review decisions: %w", err)
	}
	defer rows.Close()

	result := make([]ReviewDecision, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan controlled learning review decision: %w", err)
		}
		decision, err := decodeReviewDecision(payload)
		if err != nil {
			return nil, err
		}
		if decision.OwnerIdentity != owner ||
			decision.ProposalID != proposalID {
			return nil, ErrOwnerScopeViolation
		}
		result = append(result, decision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate controlled learning review decisions: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) getOutcomeByIdempotency(
	ctx context.Context,
	ownerIdentity string,
	idempotencyKey string,
) (OutcomeRecord, error) {
	var payload []byte
	err := repository.DB.WithContext(ctx).Raw(`
		SELECT payload
		FROM public.controlled_learning_outcomes
		WHERE owner_identity = ? AND idempotency_key = ?`,
		ownerIdentity,
		idempotencyKey,
	).Row().Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return OutcomeRecord{}, ErrNotFound
	}
	if err != nil {
		return OutcomeRecord{}, fmt.Errorf("resolve outcome idempotency: %w", err)
	}
	return decodeOutcome(payload)
}

func (repository *PostgresRepository) getProposalByIdempotency(
	ctx context.Context,
	ownerIdentity string,
	idempotencyKey string,
) (LearningProposal, error) {
	row := repository.DB.WithContext(ctx).Raw(
		proposalSelect+`
		WHERE owner_identity = ? AND idempotency_key = ?`,
		ownerIdentity,
		idempotencyKey,
	).Row()
	proposal, err := scanProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return LearningProposal{}, ErrNotFound
	}
	if err != nil {
		return LearningProposal{}, fmt.Errorf("resolve proposal idempotency: %w", err)
	}
	return proposal, nil
}

func (repository *PostgresRepository) ready() error {
	if repository == nil || repository.DB == nil {
		return fmt.Errorf("controlled learning database is required")
	}
	return nil
}

const proposalSelect = `
	SELECT definition_payload, proposal_status, revision, updated_at_unix_nano
	FROM public.controlled_learning_proposals`

type rowScanner interface {
	Scan(...any) error
}

func scanProposal(row rowScanner) (LearningProposal, error) {
	var payload []byte
	var status string
	var revision int64
	var updatedAtUnixNano int64
	if err := row.Scan(
		&payload,
		&status,
		&revision,
		&updatedAtUnixNano,
	); err != nil {
		return LearningProposal{}, err
	}
	proposal, err := decodeProposal(payload)
	if err != nil {
		return LearningProposal{}, err
	}
	proposal.Status = ProposalStatus(status)
	proposal.Revision = revision
	proposal.UpdatedAt = time.Unix(0, updatedAtUnixNano).UTC()
	if !validProposalStatus(proposal.Status) || proposal.Revision <= 0 {
		return LearningProposal{}, ErrIntegrityViolation
	}
	if err := verifyProposalIntegrity(proposal); err != nil {
		return LearningProposal{}, err
	}
	return proposal, nil
}

func validatePostgresLookup(
	ownerIdentity string,
	id string,
) (string, uuid.UUID, error) {
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return "", uuid.Nil, ErrOwnerScopeViolation
	}
	parsedID, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil || parsedID == uuid.Nil {
		return "", uuid.Nil, ErrNotFound
	}
	return owner, parsedID, nil
}

func validateStoredOutcome(record OutcomeRecord) error {
	if strings.TrimSpace(record.OwnerIdentity) == "" {
		return ErrOwnerScopeViolation
	}
	if _, err := uuid.Parse(record.ID); err != nil {
		return fmt.Errorf("controlled learning outcome id must be a UUID: %w", err)
	}
	if record.ProtocolVersion != ProtocolVersion ||
		strings.TrimSpace(record.IdempotencyKey) == "" ||
		strings.TrimSpace(record.OperationID) == "" {
		return ErrIntegrityViolation
	}
	return verifyOutcomeIntegrity(record)
}

func validateStoredProposal(proposal LearningProposal) error {
	if strings.TrimSpace(proposal.OwnerIdentity) == "" {
		return ErrOwnerScopeViolation
	}
	if _, err := uuid.Parse(proposal.ID); err != nil {
		return fmt.Errorf("controlled learning proposal id must be a UUID: %w", err)
	}
	if proposal.ProtocolVersion != ProtocolVersion ||
		strings.TrimSpace(proposal.IdempotencyKey) == "" ||
		proposal.Revision != 1 ||
		!validProposalStatus(proposal.Status) ||
		len(proposal.EvidenceIDs) == 0 {
		return ErrIntegrityViolation
	}
	for _, evidenceID := range proposal.EvidenceIDs {
		if _, err := uuid.Parse(evidenceID); err != nil {
			return fmt.Errorf(
				"controlled learning evidence id must be a UUID: %w",
				err,
			)
		}
	}
	return verifyProposalIntegrity(proposal)
}

func validateStoredDecision(
	ownerIdentity string,
	proposalID string,
	expectedRevision int64,
	decision ReviewDecision,
) error {
	if decision.OwnerIdentity != ownerIdentity ||
		decision.ProposalID != proposalID {
		return ErrOwnerScopeViolation
	}
	if _, err := uuid.Parse(decision.ID); err != nil {
		return fmt.Errorf("controlled learning decision id must be a UUID: %w", err)
	}
	if decision.ProposalRevision != expectedRevision ||
		!decision.HumanConfirmed ||
		strings.TrimSpace(decision.ActorIdentity) == "" {
		return ErrIntegrityViolation
	}
	return verifyReviewDecisionIntegrity(decision)
}

func validateDecisionTransition(
	proposal LearningProposal,
	decision ReviewDecision,
	nextStatus ProposalStatus,
) error {
	request := DecideRequest{
		OwnerIdentity:       proposal.OwnerIdentity,
		ProposalID:          proposal.ID,
		ExpectedRevision:    proposal.Revision,
		Kind:                decision.Kind,
		ActorIdentity:       decision.ActorIdentity,
		HumanConfirmed:      decision.HumanConfirmed,
		Rationale:           decision.Rationale,
		GovernanceReference: decision.GovernanceReference,
	}
	expected, err := nextProposalStatus(proposal, request)
	if err != nil {
		return err
	}
	if expected != nextStatus {
		return ErrInvalidStateChange
	}
	return nil
}

func validProposalStatus(status ProposalStatus) bool {
	switch status {
	case ProposalReviewRequired,
		ProposalGovernanceRequired,
		ProposalGovernanceReview,
		ProposalApproved,
		ProposalRejected,
		ProposalChangesRequested:
		return true
	default:
		return false
	}
}

func marshalControlledLearning(kind string, value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode controlled learning %s: %w", kind, err)
	}
	return payload, nil
}

func decodeOutcome(payload []byte) (OutcomeRecord, error) {
	var record OutcomeRecord
	if err := decodeControlledLearning("outcome", payload, &record); err != nil {
		return OutcomeRecord{}, err
	}
	if err := validateStoredOutcome(record); err != nil {
		return OutcomeRecord{}, fmt.Errorf("stored controlled learning outcome is invalid: %w", err)
	}
	return cloneOutcome(record), nil
}

func decodeProposal(payload []byte) (LearningProposal, error) {
	var proposal LearningProposal
	if err := decodeControlledLearning("proposal", payload, &proposal); err != nil {
		return LearningProposal{}, err
	}
	if strings.TrimSpace(proposal.OwnerIdentity) == "" {
		return LearningProposal{}, ErrOwnerScopeViolation
	}
	if _, err := uuid.Parse(proposal.ID); err != nil {
		return LearningProposal{}, ErrIntegrityViolation
	}
	return cloneProposal(proposal), nil
}

func decodeReviewDecision(payload []byte) (ReviewDecision, error) {
	var decision ReviewDecision
	if err := decodeControlledLearning(
		"review decision",
		payload,
		&decision,
	); err != nil {
		return ReviewDecision{}, err
	}
	if err := validateStoredDecision(
		decision.OwnerIdentity,
		decision.ProposalID,
		decision.ProposalRevision,
		decision,
	); err != nil {
		return ReviewDecision{}, fmt.Errorf(
			"stored controlled learning review decision is invalid: %w",
			err,
		)
	}
	return cloneDecision(decision), nil
}

func decodeControlledLearning(kind string, payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode controlled learning %s: %w", kind, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode controlled learning %s: trailing data", kind)
	}
	return nil
}

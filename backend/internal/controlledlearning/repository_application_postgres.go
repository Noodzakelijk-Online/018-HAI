package controlledlearning

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const postgresApplicationSelect = `
	SELECT payload
	FROM public.controlled_learning_applications`

func (repository *PostgresRepository) AcquireApplication(
	ctx context.Context,
	candidate ApplicationRecord,
) (ApplicationRecord, bool, error) {
	if err := repository.ready(); err != nil {
		return ApplicationRecord{}, false, err
	}
	if err := validateStoredApplicationCandidate(candidate); err != nil {
		return ApplicationRecord{}, false, err
	}
	payload, err := marshalControlledLearning("application", candidate)
	if err != nil {
		return ApplicationRecord{}, false, err
	}
	var acquired ApplicationRecord
	execute := false
	err = repository.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`
			INSERT INTO public.controlled_learning_applications (
				id, protocol_version, owner_identity, proposal_id,
				proposal_revision, proposal_digest, idempotency_key,
				intent_digest, application_mode, application_status,
				target_kind, protected_target, applier_id, current_version,
				proposed_version, attempt, lease_expires_at,
				definition_digest, result_digest, created_at, updated_at,
				completed_at, payload
			) VALUES (
				?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
				?, NULL, ?, ?, NULL, CAST(? AS jsonb)
			)
			ON CONFLICT DO NOTHING`,
			candidate.ID,
			candidate.ProtocolVersion,
			candidate.OwnerIdentity,
			candidate.ProposalID,
			candidate.ProposalRevision,
			candidate.ProposalDigest,
			candidate.IdempotencyKey,
			candidate.IntentDigest,
			string(candidate.Mode),
			string(candidate.Status),
			string(candidate.Target),
			candidate.ProtectedTarget,
			candidate.ApplierID,
			candidate.CurrentVersion,
			candidate.ProposedVersion,
			candidate.Attempt,
			candidate.LeaseExpiresAt.UTC(),
			candidate.DefinitionDigest,
			candidate.CreatedAt.UTC(),
			candidate.UpdatedAt.UTC(),
			string(payload),
		)
		if result.Error != nil {
			return fmt.Errorf("reserve controlled learning application: %w", result.Error)
		}
		if result.RowsAffected == 1 {
			if err := insertPostgresApplicationEvent(
				tx,
				newApplicationEvent(
					candidate,
					ApplicationEventReserved,
					nil,
					"",
					"",
					candidate.CreatedAt,
				),
			); err != nil {
				return err
			}
			acquired = candidate
			execute = true
			return nil
		}

		existing, err := scanPostgresApplication(tx.Raw(
			postgresApplicationSelect+`
			WHERE owner_identity = ? AND idempotency_key = ?
			FOR UPDATE`,
			candidate.OwnerIdentity,
			candidate.IdempotencyKey,
		).Row())
		if errors.Is(err, sql.ErrNoRows) {
			return ErrIdempotencyConflict
		}
		if err != nil {
			return fmt.Errorf("lock controlled learning application: %w", err)
		}
		if existing.DefinitionDigest != candidate.DefinitionDigest ||
			existing.IntentDigest != candidate.IntentDigest {
			return ErrIdempotencyConflict
		}
		switch existing.Status {
		case ApplicationApplied, ApplicationHandoffReady, ApplicationRolledBack:
			acquired = existing
			return nil
		case ApplicationApplying, ApplicationHandoffPending, ApplicationRollbackApplying:
			if existing.LeaseExpiresAt.After(candidate.UpdatedAt) {
				return ErrApplicationInProgress
			}
		case ApplicationFailed, ApplicationRollbackFailed:
		default:
			return ErrIntegrityViolation
		}
		existing.Attempt++
		existing.Status = candidate.Status
		existing.LeaseExpiresAt = candidate.LeaseExpiresAt
		existing.UpdatedAt = candidate.UpdatedAt
		existing.LastErrorCode = ""
		if err := updatePostgresApplication(tx, existing); err != nil {
			return err
		}
		if err := insertPostgresApplicationEvent(
			tx,
			newApplicationEvent(
				existing,
				ApplicationEventAttemptStarted,
				nil,
				"",
				"",
				existing.UpdatedAt,
			),
		); err != nil {
			return err
		}
		acquired = existing
		execute = true
		return nil
	})
	if err != nil {
		return ApplicationRecord{}, false, err
	}
	return cloneApplication(acquired), execute, nil
}

func (repository *PostgresRepository) CompleteApplication(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
	expectedAttempt int,
	decision ReviewDecision,
	nextStatus ProposalStatus,
	completion ApplicationCompletion,
) (LearningProposal, ApplicationRecord, error) {
	if err := repository.ready(); err != nil {
		return LearningProposal{}, ApplicationRecord{}, err
	}
	owner, parsedApplicationID, err := validatePostgresLookup(ownerIdentity, applicationID)
	if err != nil {
		return LearningProposal{}, ApplicationRecord{}, err
	}
	decisionPayload, err := marshalControlledLearning("review decision", decision)
	if err != nil {
		return LearningProposal{}, ApplicationRecord{}, err
	}
	var updatedProposal LearningProposal
	var updatedApplication ApplicationRecord
	err = repository.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		application, err := scanPostgresApplication(tx.Raw(
			postgresApplicationSelect+`
			WHERE owner_identity = ? AND id = ?
			FOR UPDATE`,
			owner,
			parsedApplicationID,
		).Row())
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock controlled learning application: %w", err)
		}
		if err := validateApplicationCompletion(
			application,
			expectedAttempt,
			decision,
			nextStatus,
			completion,
		); err != nil {
			return err
		}
		proposal, err := scanProposal(tx.Raw(
			proposalSelect+`
			WHERE owner_identity = ? AND id = ?
			FOR UPDATE`,
			owner,
			application.ProposalID,
		).Row())
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock controlled learning proposal: %w", err)
		}
		if proposal.Revision != application.ProposalRevision ||
			proposal.ProposalDigest != application.ProposalDigest {
			return ErrRevisionConflict
		}
		if err := validateDecisionTransition(proposal, decision, nextStatus); err != nil {
			return err
		}

		application.Status = completion.Status
		application.AppliedVersion = completion.AppliedVersion
		application.RollbackToken = completion.RollbackToken
		application.HandoffReference = completion.HandoffReference
		application.Evidence = append([]ApplicationEvidence(nil), completion.Evidence...)
		application.ResultDigest = completion.ResultDigest
		application.DecisionID = decision.ID
		application.DecisionDigest = decision.DecisionDigest
		application.LeaseExpiresAt = time.Time{}
		application.LastErrorCode = ""
		application.CompletedAt = completion.CompletedAt.UTC()
		application.UpdatedAt = completion.CompletedAt.UTC()
		if err := verifyApplicationIntegrity(application); err != nil {
			return err
		}
		if err := updatePostgresApplication(tx, application); err != nil {
			return err
		}
		if err := insertReviewDecision(tx, decision, decisionPayload); err != nil {
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
			application.ProposalID,
			application.ProposalRevision,
			application.ProposalDigest,
		)
		if result.Error != nil {
			return fmt.Errorf("advance controlled learning proposal: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrRevisionConflict
		}
		eventKind := ApplicationEventApplied
		if completion.Status == ApplicationHandoffReady {
			eventKind = ApplicationEventHandoffReady
		}
		if err := insertPostgresApplicationEvent(
			tx,
			newApplicationEvent(
				application,
				eventKind,
				completion.Evidence,
				completionVersion(completion),
				completionReference(completion),
				completion.CompletedAt,
			),
		); err != nil {
			return err
		}
		proposal.Status = nextStatus
		proposal.Revision++
		proposal.UpdatedAt = decision.DecidedAt.UTC()
		updatedProposal = proposal
		updatedApplication = application
		return nil
	})
	if err != nil {
		return LearningProposal{}, ApplicationRecord{}, err
	}
	return cloneProposal(updatedProposal), cloneApplication(updatedApplication), nil
}

func (repository *PostgresRepository) FailApplication(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
	expectedAttempt int,
	errorCode string,
	failedAt time.Time,
) (ApplicationRecord, error) {
	return repository.failPostgresApplication(
		ctx,
		ownerIdentity,
		applicationID,
		expectedAttempt,
		[]ApplicationStatus{ApplicationApplying, ApplicationHandoffPending},
		ApplicationFailed,
		ApplicationEventFailed,
		errorCode,
		failedAt,
	)
}

func (repository *PostgresRepository) GetApplication(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
) (ApplicationRecord, error) {
	if err := repository.ready(); err != nil {
		return ApplicationRecord{}, err
	}
	owner, parsedID, err := validatePostgresLookup(ownerIdentity, applicationID)
	if err != nil {
		return ApplicationRecord{}, err
	}
	application, err := scanPostgresApplication(repository.DB.WithContext(ctx).Raw(
		postgresApplicationSelect+` WHERE owner_identity = ? AND id = ?`,
		owner,
		parsedID,
	).Row())
	if errors.Is(err, sql.ErrNoRows) {
		return ApplicationRecord{}, ErrNotFound
	}
	if err != nil {
		return ApplicationRecord{}, fmt.Errorf("get controlled learning application: %w", err)
	}
	return application, nil
}

func (repository *PostgresRepository) GetProposalApplication(
	ctx context.Context,
	ownerIdentity string,
	proposalID string,
	proposalRevision int64,
	mode ApplicationMode,
) (ApplicationRecord, error) {
	if err := repository.ready(); err != nil {
		return ApplicationRecord{}, err
	}
	owner, parsedProposalID, err := validatePostgresLookup(ownerIdentity, proposalID)
	if err != nil {
		return ApplicationRecord{}, err
	}
	application, err := scanPostgresApplication(repository.DB.WithContext(ctx).Raw(
		postgresApplicationSelect+`
		WHERE owner_identity = ?
		  AND proposal_id = ?
		  AND proposal_revision = ?
		  AND application_mode = ?`,
		owner,
		parsedProposalID,
		proposalRevision,
		string(mode),
	).Row())
	if errors.Is(err, sql.ErrNoRows) {
		return ApplicationRecord{}, ErrNotFound
	}
	if err != nil {
		return ApplicationRecord{}, fmt.Errorf("get proposal application: %w", err)
	}
	return application, nil
}

func (repository *PostgresRepository) ListApplications(
	ctx context.Context,
	query ApplicationQuery,
) ([]ApplicationRecord, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	owner := strings.TrimSpace(query.OwnerIdentity)
	if owner == "" {
		return nil, ErrOwnerScopeViolation
	}
	statement := postgresApplicationSelect + ` WHERE owner_identity = ?`
	args := []any{owner}
	if proposalID := strings.TrimSpace(query.ProposalID); proposalID != "" {
		parsed, err := uuid.Parse(proposalID)
		if err != nil {
			return nil, ErrNotFound
		}
		statement += ` AND proposal_id = ?`
		args = append(args, parsed)
	}
	if query.Status != "" {
		statement += ` AND application_status = ?`
		args = append(args, string(query.Status))
	}
	statement += ` ORDER BY updated_at DESC, id ASC LIMIT ?`
	args = append(args, normalizedLimit(query.Limit))
	rows, err := repository.DB.WithContext(ctx).Raw(statement, args...).Rows()
	if err != nil {
		return nil, fmt.Errorf("list controlled learning applications: %w", err)
	}
	defer rows.Close()
	result := make([]ApplicationRecord, 0)
	for rows.Next() {
		application, err := scanPostgresApplication(rows)
		if err != nil {
			return nil, err
		}
		if application.OwnerIdentity != owner {
			return nil, ErrOwnerScopeViolation
		}
		result = append(result, application)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate controlled learning applications: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) ListApplicationEvents(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
) ([]ApplicationEvent, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	owner, parsedID, err := validatePostgresLookup(ownerIdentity, applicationID)
	if err != nil {
		return nil, err
	}
	var exists bool
	if err := repository.DB.WithContext(ctx).Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM public.controlled_learning_applications
			WHERE owner_identity = ? AND id = ?
		)`,
		owner,
		parsedID,
	).Row().Scan(&exists); err != nil {
		return nil, fmt.Errorf("check controlled learning application: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := repository.DB.WithContext(ctx).Raw(`
		SELECT payload
		FROM public.controlled_learning_application_events
		WHERE owner_identity = ? AND application_id = ?
		ORDER BY occurred_at ASC, id ASC`,
		owner,
		parsedID,
	).Rows()
	if err != nil {
		return nil, fmt.Errorf("list controlled learning application events: %w", err)
	}
	defer rows.Close()
	result := make([]ApplicationEvent, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan controlled learning application event: %w", err)
		}
		event, err := decodeApplicationEvent(payload)
		if err != nil {
			return nil, err
		}
		if event.OwnerIdentity != owner || event.ApplicationID != applicationID {
			return nil, ErrOwnerScopeViolation
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate controlled learning application events: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) AcquireRollback(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
	intentDigest string,
	now time.Time,
	leaseExpiresAt time.Time,
) (ApplicationRecord, bool, error) {
	if err := repository.ready(); err != nil {
		return ApplicationRecord{}, false, err
	}
	owner, parsedID, err := validatePostgresLookup(ownerIdentity, applicationID)
	if err != nil {
		return ApplicationRecord{}, false, err
	}
	var acquired ApplicationRecord
	execute := false
	err = repository.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		application, err := scanPostgresApplication(tx.Raw(
			postgresApplicationSelect+`
			WHERE owner_identity = ? AND id = ?
			FOR UPDATE`,
			owner,
			parsedID,
		).Row())
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock controlled learning rollback: %w", err)
		}
		if application.Mode != ApplicationModeApply || application.ProtectedTarget {
			return ErrRollbackUnavailable
		}
		switch application.Status {
		case ApplicationApplied:
		case ApplicationRolledBack:
			if application.RollbackIntentDigest != intentDigest {
				return ErrIdempotencyConflict
			}
			acquired = application
			return nil
		case ApplicationRollbackApplying:
			if application.RollbackIntentDigest != intentDigest {
				return ErrIdempotencyConflict
			}
			if application.LeaseExpiresAt.After(now) {
				return ErrApplicationInProgress
			}
		case ApplicationRollbackFailed:
			if application.RollbackIntentDigest != intentDigest {
				return ErrIdempotencyConflict
			}
		default:
			return ErrInvalidStateChange
		}
		application.Attempt++
		application.Status = ApplicationRollbackApplying
		application.RollbackIntentDigest = intentDigest
		application.LeaseExpiresAt = leaseExpiresAt.UTC()
		application.LastErrorCode = ""
		application.UpdatedAt = now.UTC()
		if err := updatePostgresApplication(tx, application); err != nil {
			return err
		}
		if err := insertPostgresApplicationEvent(
			tx,
			newApplicationEvent(
				application,
				ApplicationEventRollbackStarted,
				nil,
				application.AppliedVersion,
				"",
				now,
			),
		); err != nil {
			return err
		}
		acquired = application
		execute = true
		return nil
	})
	if err != nil {
		return ApplicationRecord{}, false, err
	}
	return cloneApplication(acquired), execute, nil
}

func (repository *PostgresRepository) CompleteRollback(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
	expectedAttempt int,
	result PromotionRollbackResult,
	completedAt time.Time,
) (ApplicationRecord, error) {
	if err := repository.ready(); err != nil {
		return ApplicationRecord{}, err
	}
	owner, parsedID, err := validatePostgresLookup(ownerIdentity, applicationID)
	if err != nil {
		return ApplicationRecord{}, err
	}
	var completed ApplicationRecord
	err = repository.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		application, err := scanPostgresApplication(tx.Raw(
			postgresApplicationSelect+`
			WHERE owner_identity = ? AND id = ?
			FOR UPDATE`,
			owner,
			parsedID,
		).Row())
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock controlled learning rollback: %w", err)
		}
		if application.Attempt != expectedAttempt ||
			application.Status != ApplicationRollbackApplying ||
			strings.TrimSpace(result.RestoredVersion) != application.CurrentVersion ||
			len(result.Evidence) == 0 {
			return ErrInvalidStateChange
		}
		application.Status = ApplicationRolledBack
		application.RestoredVersion = strings.TrimSpace(result.RestoredVersion)
		application.RollbackEvidence = append([]ApplicationEvidence(nil), result.Evidence...)
		application.LeaseExpiresAt = time.Time{}
		application.LastErrorCode = ""
		application.RolledBackAt = completedAt.UTC()
		application.UpdatedAt = completedAt.UTC()
		if err := updatePostgresApplication(tx, application); err != nil {
			return err
		}
		if err := insertPostgresApplicationEvent(
			tx,
			newApplicationEvent(
				application,
				ApplicationEventRolledBack,
				result.Evidence,
				application.RestoredVersion,
				"",
				completedAt,
			),
		); err != nil {
			return err
		}
		completed = application
		return nil
	})
	if err != nil {
		return ApplicationRecord{}, err
	}
	return cloneApplication(completed), nil
}

func (repository *PostgresRepository) FailRollback(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
	expectedAttempt int,
	errorCode string,
	failedAt time.Time,
) (ApplicationRecord, error) {
	return repository.failPostgresApplication(
		ctx,
		ownerIdentity,
		applicationID,
		expectedAttempt,
		[]ApplicationStatus{ApplicationRollbackApplying},
		ApplicationRollbackFailed,
		ApplicationEventRollbackFailed,
		errorCode,
		failedAt,
	)
}

func (repository *PostgresRepository) failPostgresApplication(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
	expectedAttempt int,
	allowed []ApplicationStatus,
	failedStatus ApplicationStatus,
	eventKind ApplicationEventKind,
	errorCode string,
	failedAt time.Time,
) (ApplicationRecord, error) {
	if err := repository.ready(); err != nil {
		return ApplicationRecord{}, err
	}
	if err := validateFailureCode(errorCode); err != nil {
		return ApplicationRecord{}, err
	}
	owner, parsedID, err := validatePostgresLookup(ownerIdentity, applicationID)
	if err != nil {
		return ApplicationRecord{}, err
	}
	var failed ApplicationRecord
	err = repository.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		application, err := scanPostgresApplication(tx.Raw(
			postgresApplicationSelect+`
			WHERE owner_identity = ? AND id = ?
			FOR UPDATE`,
			owner,
			parsedID,
		).Row())
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock failed controlled learning application: %w", err)
		}
		if application.Attempt != expectedAttempt ||
			!containsApplicationStatus(allowed, application.Status) {
			return ErrRevisionConflict
		}
		application.Status = failedStatus
		application.LastErrorCode = strings.TrimSpace(errorCode)
		application.LeaseExpiresAt = time.Time{}
		application.UpdatedAt = failedAt.UTC()
		if err := updatePostgresApplication(tx, application); err != nil {
			return err
		}
		if err := insertPostgresApplicationEvent(
			tx,
			newApplicationEvent(application, eventKind, nil, "", "", failedAt),
		); err != nil {
			return err
		}
		failed = application
		return nil
	})
	if err != nil {
		return ApplicationRecord{}, err
	}
	return cloneApplication(failed), nil
}

func updatePostgresApplication(tx *gorm.DB, application ApplicationRecord) error {
	payload, err := marshalControlledLearning("application", application)
	if err != nil {
		return err
	}
	var leaseExpiresAt any
	if !application.LeaseExpiresAt.IsZero() {
		leaseExpiresAt = application.LeaseExpiresAt.UTC()
	}
	var completedAt any
	if !application.CompletedAt.IsZero() {
		completedAt = application.CompletedAt.UTC()
	}
	var resultDigest any
	if application.ResultDigest != "" {
		resultDigest = application.ResultDigest
	}
	result := tx.Exec(`
		UPDATE public.controlled_learning_applications
		SET application_status = ?,
			attempt = ?,
			lease_expires_at = ?,
			result_digest = ?,
			updated_at = ?,
			completed_at = ?,
			payload = CAST(? AS jsonb)
		WHERE owner_identity = ?
		  AND id = ?
		  AND definition_digest = ?`,
		string(application.Status),
		application.Attempt,
		leaseExpiresAt,
		resultDigest,
		application.UpdatedAt.UTC(),
		completedAt,
		string(payload),
		application.OwnerIdentity,
		application.ID,
		application.DefinitionDigest,
	)
	if result.Error != nil {
		return fmt.Errorf("update controlled learning application: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	return nil
}

func insertPostgresApplicationEvent(tx *gorm.DB, event ApplicationEvent) error {
	if err := verifyApplicationEventIntegrity(event); err != nil {
		return err
	}
	payload, err := marshalControlledLearning("application event", event)
	if err != nil {
		return err
	}
	result := tx.Exec(`
		INSERT INTO public.controlled_learning_application_events (
			id, owner_identity, application_id, proposal_id, attempt,
			event_kind, application_status, application_digest,
			event_digest, occurred_at, payload
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS jsonb))`,
		event.ID,
		event.OwnerIdentity,
		event.ApplicationID,
		event.ProposalID,
		event.Attempt,
		string(event.Kind),
		string(event.Status),
		event.ApplicationDigest,
		event.EventDigest,
		event.OccurredAt.UTC(),
		string(payload),
	)
	if result.Error != nil {
		return fmt.Errorf("append controlled learning application event: %w", result.Error)
	}
	return nil
}

func newApplicationEvent(
	application ApplicationRecord,
	kind ApplicationEventKind,
	evidence []ApplicationEvidence,
	version string,
	reference string,
	occurredAt time.Time,
) ApplicationEvent {
	event := ApplicationEvent{
		ID:                uuid.NewString(),
		ApplicationID:     application.ID,
		OwnerIdentity:     application.OwnerIdentity,
		ProposalID:        application.ProposalID,
		Attempt:           application.Attempt,
		Kind:              kind,
		Status:            application.Status,
		Version:           strings.TrimSpace(version),
		Reference:         strings.TrimSpace(reference),
		ErrorCode:         application.LastErrorCode,
		Evidence:          append([]ApplicationEvidence(nil), evidence...),
		ApplicationDigest: applicationStateDigest(application),
		OccurredAt:        occurredAt.UTC(),
	}
	event.EventDigest, _ = applicationEventDigest(event)
	return event
}

func scanPostgresApplication(row rowScanner) (ApplicationRecord, error) {
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		return ApplicationRecord{}, err
	}
	return decodeApplication(payload)
}

func decodeApplication(payload []byte) (ApplicationRecord, error) {
	var application ApplicationRecord
	if err := decodeControlledLearning("application", payload, &application); err != nil {
		return ApplicationRecord{}, err
	}
	if err := validateDecodedApplication(application); err != nil {
		return ApplicationRecord{}, fmt.Errorf(
			"stored controlled learning application is invalid: %w",
			err,
		)
	}
	return cloneApplication(application), nil
}

func decodeApplicationEvent(payload []byte) (ApplicationEvent, error) {
	var event ApplicationEvent
	if err := decodeControlledLearning("application event", payload, &event); err != nil {
		return ApplicationEvent{}, err
	}
	if _, err := uuid.Parse(event.ID); err != nil {
		return ApplicationEvent{}, ErrIntegrityViolation
	}
	if err := verifyApplicationEventIntegrity(event); err != nil {
		return ApplicationEvent{}, err
	}
	return cloneApplicationEvent(event), nil
}

func validateStoredApplicationCandidate(application ApplicationRecord) error {
	if err := validateApplicationCandidate(application); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"application id":          application.ID,
		"application proposal id": application.ProposalID,
	} {
		parsed, err := uuid.Parse(value)
		if err != nil || parsed == uuid.Nil {
			return fmt.Errorf("%s must be a UUID", label)
		}
	}
	return nil
}

func validateDecodedApplication(application ApplicationRecord) error {
	if _, err := uuid.Parse(application.ID); err != nil {
		return ErrIntegrityViolation
	}
	if _, err := uuid.Parse(application.ProposalID); err != nil {
		return ErrIntegrityViolation
	}
	if application.ProtocolVersion != ProtocolVersion ||
		application.OwnerIdentity == "" ||
		application.Attempt <= 0 ||
		!validApplicationMode(application.Mode) ||
		!validApplicationStatus(application.Status) {
		return ErrIntegrityViolation
	}
	return verifyApplicationIntegrity(application)
}

func validApplicationMode(mode ApplicationMode) bool {
	return mode == ApplicationModeApply || mode == ApplicationModeProtectedHandoff
}

func validApplicationStatus(status ApplicationStatus) bool {
	switch status {
	case ApplicationApplying,
		ApplicationApplied,
		ApplicationHandoffPending,
		ApplicationHandoffReady,
		ApplicationFailed,
		ApplicationRollbackApplying,
		ApplicationRolledBack,
		ApplicationRollbackFailed:
		return true
	default:
		return false
	}
}

func containsApplicationStatus(values []ApplicationStatus, value ApplicationStatus) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

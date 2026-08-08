package controlledlearning

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (repository *MemoryRepository) AcquireApplication(
	ctx context.Context,
	candidate ApplicationRecord,
) (ApplicationRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return ApplicationRecord{}, false, err
	}
	if err := validateApplicationCandidate(candidate); err != nil {
		return ApplicationRecord{}, false, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()

	idempotency := scopedKey(candidate.OwnerIdentity, candidate.IdempotencyKey)
	if existingID, exists := repository.applicationByKey[idempotency]; exists {
		key := scopedKey(candidate.OwnerIdentity, existingID)
		existing := repository.applications[key]
		if existing.DefinitionDigest != candidate.DefinitionDigest ||
			existing.IntentDigest != candidate.IntentDigest {
			return ApplicationRecord{}, false, ErrIdempotencyConflict
		}
		if err := verifyApplicationIntegrity(existing); err != nil {
			return ApplicationRecord{}, false, err
		}
		switch existing.Status {
		case ApplicationApplied, ApplicationHandoffReady, ApplicationRolledBack:
			return cloneApplication(existing), false, nil
		case ApplicationApplying, ApplicationHandoffPending, ApplicationRollbackApplying:
			if existing.LeaseExpiresAt.After(candidate.UpdatedAt) {
				return ApplicationRecord{}, false, ErrApplicationInProgress
			}
		case ApplicationFailed, ApplicationRollbackFailed:
		default:
			return ApplicationRecord{}, false, ErrIntegrityViolation
		}
		existing.Attempt++
		existing.Status = candidate.Status
		existing.LeaseExpiresAt = candidate.LeaseExpiresAt
		existing.UpdatedAt = candidate.UpdatedAt
		existing.LastErrorCode = ""
		repository.applications[key] = cloneApplication(existing)
		if err := repository.appendApplicationEventLocked(
			existing,
			ApplicationEventAttemptStarted,
			nil,
			"",
			"",
			existing.UpdatedAt,
		); err != nil {
			return ApplicationRecord{}, false, err
		}
		return cloneApplication(existing), true, nil
	}
	proposal, exists := repository.proposals[scopedKey(candidate.OwnerIdentity, candidate.ProposalID)]
	if !exists {
		return ApplicationRecord{}, false, ErrNotFound
	}
	if proposal.Revision != candidate.ProposalRevision ||
		proposal.ProposalDigest != candidate.ProposalDigest {
		return ApplicationRecord{}, false, ErrRevisionConflict
	}
	key := scopedKey(candidate.OwnerIdentity, candidate.ID)
	if _, exists := repository.applications[key]; exists {
		return ApplicationRecord{}, false, ErrIdempotencyConflict
	}
	repository.applications[key] = cloneApplication(candidate)
	repository.applicationByKey[idempotency] = candidate.ID
	if err := repository.appendApplicationEventLocked(
		candidate,
		ApplicationEventReserved,
		nil,
		"",
		"",
		candidate.CreatedAt,
	); err != nil {
		delete(repository.applications, key)
		delete(repository.applicationByKey, idempotency)
		return ApplicationRecord{}, false, err
	}
	return cloneApplication(candidate), true, nil
}

func (repository *MemoryRepository) CompleteApplication(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
	expectedAttempt int,
	decision ReviewDecision,
	nextStatus ProposalStatus,
	completion ApplicationCompletion,
) (LearningProposal, ApplicationRecord, error) {
	if err := ctx.Err(); err != nil {
		return LearningProposal{}, ApplicationRecord{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()

	owner := strings.TrimSpace(ownerIdentity)
	key := scopedKey(owner, applicationID)
	application, exists := repository.applications[key]
	if !exists {
		return LearningProposal{}, ApplicationRecord{}, ErrNotFound
	}
	if err := validateApplicationCompletion(application, expectedAttempt, decision, nextStatus, completion); err != nil {
		return LearningProposal{}, ApplicationRecord{}, err
	}
	proposalKey := scopedKey(owner, application.ProposalID)
	proposal, exists := repository.proposals[proposalKey]
	if !exists {
		return LearningProposal{}, ApplicationRecord{}, ErrNotFound
	}
	if proposal.Revision != application.ProposalRevision ||
		proposal.ProposalDigest != application.ProposalDigest {
		return LearningProposal{}, ApplicationRecord{}, ErrRevisionConflict
	}
	if err := validateDecisionTransition(proposal, decision, nextStatus); err != nil {
		return LearningProposal{}, ApplicationRecord{}, err
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
		return LearningProposal{}, ApplicationRecord{}, err
	}

	proposal.Status = nextStatus
	proposal.Revision++
	proposal.UpdatedAt = decision.DecidedAt.UTC()
	repository.proposals[proposalKey] = cloneProposal(proposal)
	repository.decisions[proposalKey] = append(repository.decisions[proposalKey], cloneDecision(decision))
	repository.applications[key] = cloneApplication(application)

	eventKind := ApplicationEventApplied
	if completion.Status == ApplicationHandoffReady {
		eventKind = ApplicationEventHandoffReady
	}
	if err := repository.appendApplicationEventLocked(
		application,
		eventKind,
		completion.Evidence,
		completionVersion(completion),
		completionReference(completion),
		completion.CompletedAt,
	); err != nil {
		return LearningProposal{}, ApplicationRecord{}, err
	}
	return cloneProposal(proposal), cloneApplication(application), nil
}

func (repository *MemoryRepository) FailApplication(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
	expectedAttempt int,
	errorCode string,
	failedAt time.Time,
) (ApplicationRecord, error) {
	if err := ctx.Err(); err != nil {
		return ApplicationRecord{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	application, key, err := repository.applicationForMutationLocked(
		ownerIdentity,
		applicationID,
		expectedAttempt,
	)
	if err != nil {
		return ApplicationRecord{}, err
	}
	if application.Status != ApplicationApplying &&
		application.Status != ApplicationHandoffPending {
		return ApplicationRecord{}, ErrInvalidStateChange
	}
	if err := validateFailureCode(errorCode); err != nil {
		return ApplicationRecord{}, err
	}
	application.Status = ApplicationFailed
	application.LastErrorCode = strings.TrimSpace(errorCode)
	application.LeaseExpiresAt = time.Time{}
	application.UpdatedAt = failedAt.UTC()
	repository.applications[key] = cloneApplication(application)
	if err := repository.appendApplicationEventLocked(
		application,
		ApplicationEventFailed,
		nil,
		"",
		"",
		failedAt,
	); err != nil {
		return ApplicationRecord{}, err
	}
	return cloneApplication(application), nil
}

func (repository *MemoryRepository) GetApplication(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
) (ApplicationRecord, error) {
	if err := ctx.Err(); err != nil {
		return ApplicationRecord{}, err
	}
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return ApplicationRecord{}, ErrOwnerScopeViolation
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	application, exists := repository.applications[scopedKey(owner, applicationID)]
	if !exists {
		return ApplicationRecord{}, ErrNotFound
	}
	if err := verifyApplicationIntegrity(application); err != nil {
		return ApplicationRecord{}, err
	}
	return cloneApplication(application), nil
}

func (repository *MemoryRepository) GetProposalApplication(
	ctx context.Context,
	ownerIdentity string,
	proposalID string,
	proposalRevision int64,
	mode ApplicationMode,
) (ApplicationRecord, error) {
	if err := ctx.Err(); err != nil {
		return ApplicationRecord{}, err
	}
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return ApplicationRecord{}, ErrOwnerScopeViolation
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	for _, application := range repository.applications {
		if application.OwnerIdentity == owner &&
			application.ProposalID == strings.TrimSpace(proposalID) &&
			application.ProposalRevision == proposalRevision &&
			application.Mode == mode {
			if err := verifyApplicationIntegrity(application); err != nil {
				return ApplicationRecord{}, err
			}
			return cloneApplication(application), nil
		}
	}
	return ApplicationRecord{}, ErrNotFound
}

func (repository *MemoryRepository) ListApplications(
	ctx context.Context,
	query ApplicationQuery,
) ([]ApplicationRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner := strings.TrimSpace(query.OwnerIdentity)
	if owner == "" {
		return nil, ErrOwnerScopeViolation
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	result := make([]ApplicationRecord, 0)
	for _, application := range repository.applications {
		if application.OwnerIdentity != owner ||
			(query.ProposalID != "" && application.ProposalID != query.ProposalID) ||
			(query.Status != "" && application.Status != query.Status) {
			continue
		}
		if err := verifyApplicationIntegrity(application); err != nil {
			return nil, err
		}
		result = append(result, cloneApplication(application))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	if len(result) > normalizedLimit(query.Limit) {
		result = result[:normalizedLimit(query.Limit)]
	}
	return result, nil
}

func (repository *MemoryRepository) ListApplicationEvents(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
) ([]ApplicationEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return nil, ErrOwnerScopeViolation
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	key := scopedKey(owner, applicationID)
	if _, exists := repository.applications[key]; !exists {
		return nil, ErrNotFound
	}
	events := repository.applicationEvents[key]
	result := make([]ApplicationEvent, len(events))
	for index := range events {
		if err := verifyApplicationEventIntegrity(events[index]); err != nil {
			return nil, err
		}
		result[index] = cloneApplicationEvent(events[index])
	}
	return result, nil
}

func (repository *MemoryRepository) AcquireRollback(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
	intentDigest string,
	now time.Time,
	leaseExpiresAt time.Time,
) (ApplicationRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return ApplicationRecord{}, false, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	owner := strings.TrimSpace(ownerIdentity)
	key := scopedKey(owner, applicationID)
	application, exists := repository.applications[key]
	if !exists {
		return ApplicationRecord{}, false, ErrNotFound
	}
	if err := verifyApplicationIntegrity(application); err != nil {
		return ApplicationRecord{}, false, err
	}
	if application.Mode != ApplicationModeApply || application.ProtectedTarget {
		return ApplicationRecord{}, false, ErrRollbackUnavailable
	}
	switch application.Status {
	case ApplicationApplied:
	case ApplicationRolledBack:
		if application.RollbackIntentDigest != intentDigest {
			return ApplicationRecord{}, false, ErrIdempotencyConflict
		}
		return cloneApplication(application), false, nil
	case ApplicationRollbackApplying:
		if application.RollbackIntentDigest != intentDigest {
			return ApplicationRecord{}, false, ErrIdempotencyConflict
		}
		if application.LeaseExpiresAt.After(now) {
			return ApplicationRecord{}, false, ErrApplicationInProgress
		}
	case ApplicationRollbackFailed:
		if application.RollbackIntentDigest != intentDigest {
			return ApplicationRecord{}, false, ErrIdempotencyConflict
		}
	default:
		return ApplicationRecord{}, false, ErrInvalidStateChange
	}
	application.Attempt++
	application.Status = ApplicationRollbackApplying
	application.RollbackIntentDigest = intentDigest
	application.LeaseExpiresAt = leaseExpiresAt.UTC()
	application.LastErrorCode = ""
	application.UpdatedAt = now.UTC()
	repository.applications[key] = cloneApplication(application)
	if err := repository.appendApplicationEventLocked(
		application,
		ApplicationEventRollbackStarted,
		nil,
		application.AppliedVersion,
		"",
		now,
	); err != nil {
		return ApplicationRecord{}, false, err
	}
	return cloneApplication(application), true, nil
}

func (repository *MemoryRepository) CompleteRollback(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
	expectedAttempt int,
	result PromotionRollbackResult,
	completedAt time.Time,
) (ApplicationRecord, error) {
	if err := ctx.Err(); err != nil {
		return ApplicationRecord{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	application, key, err := repository.applicationForMutationLocked(
		ownerIdentity,
		applicationID,
		expectedAttempt,
	)
	if err != nil {
		return ApplicationRecord{}, err
	}
	if application.Status != ApplicationRollbackApplying ||
		strings.TrimSpace(result.RestoredVersion) != application.CurrentVersion ||
		len(result.Evidence) == 0 {
		return ApplicationRecord{}, ErrInvalidStateChange
	}
	application.Status = ApplicationRolledBack
	application.RestoredVersion = strings.TrimSpace(result.RestoredVersion)
	application.RollbackEvidence = append([]ApplicationEvidence(nil), result.Evidence...)
	application.LeaseExpiresAt = time.Time{}
	application.LastErrorCode = ""
	application.RolledBackAt = completedAt.UTC()
	application.UpdatedAt = completedAt.UTC()
	repository.applications[key] = cloneApplication(application)
	if err := repository.appendApplicationEventLocked(
		application,
		ApplicationEventRolledBack,
		result.Evidence,
		application.RestoredVersion,
		"",
		completedAt,
	); err != nil {
		return ApplicationRecord{}, err
	}
	return cloneApplication(application), nil
}

func (repository *MemoryRepository) FailRollback(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
	expectedAttempt int,
	errorCode string,
	failedAt time.Time,
) (ApplicationRecord, error) {
	if err := ctx.Err(); err != nil {
		return ApplicationRecord{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	application, key, err := repository.applicationForMutationLocked(
		ownerIdentity,
		applicationID,
		expectedAttempt,
	)
	if err != nil {
		return ApplicationRecord{}, err
	}
	if application.Status != ApplicationRollbackApplying {
		return ApplicationRecord{}, ErrInvalidStateChange
	}
	if err := validateFailureCode(errorCode); err != nil {
		return ApplicationRecord{}, err
	}
	application.Status = ApplicationRollbackFailed
	application.LastErrorCode = strings.TrimSpace(errorCode)
	application.LeaseExpiresAt = time.Time{}
	application.UpdatedAt = failedAt.UTC()
	repository.applications[key] = cloneApplication(application)
	if err := repository.appendApplicationEventLocked(
		application,
		ApplicationEventRollbackFailed,
		nil,
		"",
		"",
		failedAt,
	); err != nil {
		return ApplicationRecord{}, err
	}
	return cloneApplication(application), nil
}

func (repository *MemoryRepository) applicationForMutationLocked(
	ownerIdentity string,
	applicationID string,
	expectedAttempt int,
) (ApplicationRecord, string, error) {
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" {
		return ApplicationRecord{}, "", ErrOwnerScopeViolation
	}
	key := scopedKey(owner, applicationID)
	application, exists := repository.applications[key]
	if !exists {
		return ApplicationRecord{}, "", ErrNotFound
	}
	if application.Attempt != expectedAttempt {
		return ApplicationRecord{}, "", ErrRevisionConflict
	}
	if err := verifyApplicationIntegrity(application); err != nil {
		return ApplicationRecord{}, "", err
	}
	return application, key, nil
}

func (repository *MemoryRepository) appendApplicationEventLocked(
	application ApplicationRecord,
	kind ApplicationEventKind,
	evidence []ApplicationEvidence,
	version string,
	reference string,
	occurredAt time.Time,
) error {
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
	var err error
	event.EventDigest, err = applicationEventDigest(event)
	if err != nil {
		return err
	}
	key := scopedKey(application.OwnerIdentity, application.ID)
	repository.applicationEvents[key] = append(
		repository.applicationEvents[key],
		cloneApplicationEvent(event),
	)
	return nil
}

func validateApplicationCandidate(application ApplicationRecord) error {
	for label, value := range map[string]string{
		"application id":                application.ID,
		"application owner identity":    application.OwnerIdentity,
		"application proposal id":       application.ProposalID,
		"application proposal digest":   application.ProposalDigest,
		"application idempotency key":   application.IdempotencyKey,
		"application intent digest":     application.IntentDigest,
		"application applier id":        application.ApplierID,
		"application current version":   application.CurrentVersion,
		"application proposed version":  application.ProposedVersion,
		"application rollback plan":     application.RollbackPlan,
		"application definition digest": application.DefinitionDigest,
	} {
		if err := validateRequired(label, value, maxDetailLength); err != nil {
			return err
		}
	}
	if application.ProtocolVersion != ProtocolVersion ||
		application.ProposalRevision <= 0 ||
		application.Attempt != 1 ||
		application.CreatedAt.IsZero() ||
		application.UpdatedAt.IsZero() ||
		application.LeaseExpiresAt.IsZero() {
		return ErrIntegrityViolation
	}
	if application.Mode == ApplicationModeApply {
		if application.ProtectedTarget || application.Status != ApplicationApplying {
			return ErrProtectedTarget
		}
	} else if application.Mode == ApplicationModeProtectedHandoff {
		if !application.ProtectedTarget ||
			application.Status != ApplicationHandoffPending ||
			strings.TrimSpace(application.GovernanceReference) == "" {
			return ErrIntegrityViolation
		}
	} else {
		return ErrIntegrityViolation
	}
	return verifyApplicationIntegrity(application)
}

func validateApplicationCompletion(
	application ApplicationRecord,
	expectedAttempt int,
	decision ReviewDecision,
	nextStatus ProposalStatus,
	completion ApplicationCompletion,
) error {
	if application.Attempt != expectedAttempt ||
		decision.ApplicationID != application.ID ||
		decision.ProposalDigest != application.ProposalDigest ||
		decision.ProposalRevision != application.ProposalRevision ||
		completion.CompletedAt.IsZero() ||
		completion.ResultDigest == "" ||
		len(completion.Evidence) == 0 {
		return ErrIntegrityViolation
	}
	if err := verifyReviewDecisionIntegrity(decision); err != nil {
		return err
	}
	expectedDigest, err := applicationCompletionDigest(application, completion)
	if err != nil || expectedDigest != completion.ResultDigest {
		return ErrIntegrityViolation
	}
	switch application.Mode {
	case ApplicationModeApply:
		if application.Status != ApplicationApplying ||
			completion.Status != ApplicationApplied ||
			nextStatus != ProposalApproved ||
			completion.AppliedVersion != application.ProposedVersion ||
			strings.TrimSpace(completion.RollbackToken) == "" ||
			strings.TrimSpace(completion.HandoffReference) != "" {
			return ErrIntegrityViolation
		}
	case ApplicationModeProtectedHandoff:
		if application.Status != ApplicationHandoffPending ||
			completion.Status != ApplicationHandoffReady ||
			nextStatus != ProposalGovernanceReview ||
			strings.TrimSpace(completion.HandoffReference) == "" ||
			strings.TrimSpace(completion.AppliedVersion) != "" ||
			strings.TrimSpace(completion.RollbackToken) != "" {
			return ErrIntegrityViolation
		}
	default:
		return ErrIntegrityViolation
	}
	return nil
}

func validateFailureCode(value string) error {
	code := strings.TrimSpace(value)
	if code == "" || len(code) > 128 {
		return fmt.Errorf("application failure code is required")
	}
	for _, char := range code {
		if (char < 'a' || char > 'z') &&
			(char < '0' || char > '9') &&
			char != '_' && char != '-' {
			return fmt.Errorf("invalid application failure code")
		}
	}
	return nil
}

func completionVersion(completion ApplicationCompletion) string {
	if completion.AppliedVersion != "" {
		return completion.AppliedVersion
	}
	return ""
}

func completionReference(completion ApplicationCompletion) string {
	if completion.HandoffReference != "" {
		return completion.HandoffReference
	}
	return ""
}

func applicationStateDigest(application ApplicationRecord) string {
	digest, _ := digestValue(struct {
		DefinitionDigest     string
		ResultDigest         string
		Status               ApplicationStatus
		Attempt              int
		DecisionDigest       string
		RollbackIntentDigest string
		RestoredVersion      string
		LastErrorCode        string
		UpdatedAt            time.Time
	}{
		application.DefinitionDigest,
		application.ResultDigest,
		application.Status,
		application.Attempt,
		application.DecisionDigest,
		application.RollbackIntentDigest,
		application.RestoredVersion,
		application.LastErrorCode,
		application.UpdatedAt,
	})
	return digest
}

func applicationEventDigest(event ApplicationEvent) (string, error) {
	copy := cloneApplicationEvent(event)
	copy.EventDigest = ""
	return digestValue(copy)
}

func verifyApplicationEventIntegrity(event ApplicationEvent) error {
	expected, err := applicationEventDigest(event)
	if err != nil {
		return err
	}
	if event.EventDigest == "" || event.EventDigest != expected {
		return ErrIntegrityViolation
	}
	return nil
}

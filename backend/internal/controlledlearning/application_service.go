package controlledlearning

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

var sha256DigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func (service *Service) DecideAndApply(
	ctx context.Context,
	request DecideRequest,
) (DecisionResult, error) {
	proposal, err := service.validateDecisionRequest(ctx, request)
	if err != nil {
		return DecisionResult{}, err
	}
	mode, usesApplication := applicationModeForDecision(request.Kind)
	if proposal.Revision != request.ExpectedRevision {
		if usesApplication {
			replayed, replayErr := service.replayApplicationDecision(ctx, proposal, request, mode)
			if replayErr == nil {
				return replayed, nil
			}
			if !errors.Is(replayErr, ErrNotFound) {
				return DecisionResult{}, replayErr
			}
		}
		return DecisionResult{}, ErrRevisionConflict
	}
	nextStatus, err := nextProposalStatus(proposal, request)
	if err != nil {
		return DecisionResult{}, err
	}
	if !usesApplication {
		updated, err := service.decideWithoutApplication(ctx, proposal, request, nextStatus)
		return DecisionResult{Proposal: updated}, err
	}
	return service.applyDecision(ctx, proposal, request, mode, nextStatus)
}

func (service *Service) applyDecision(
	ctx context.Context,
	proposal LearningProposal,
	request DecideRequest,
	mode ApplicationMode,
	nextStatus ProposalStatus,
) (DecisionResult, error) {
	if service.promoter == nil {
		return DecisionResult{}, ErrPromoterUnavailable
	}
	if service.applicationRepository == nil {
		return DecisionResult{}, fmt.Errorf(
			"%w: repository does not provide durable application storage",
			ErrPromoterUnavailable,
		)
	}
	now := service.now().UTC()
	idempotencyKey, err := decisionApplicationIdempotencyKey(proposal, request)
	if err != nil {
		return DecisionResult{}, err
	}
	intentDigest, err := decisionIntentDigest(proposal, request, mode)
	if err != nil {
		return DecisionResult{}, err
	}
	status := ApplicationApplying
	if mode == ApplicationModeProtectedHandoff {
		status = ApplicationHandoffPending
	}
	application := ApplicationRecord{
		ID:                  service.newID(),
		ProtocolVersion:     ProtocolVersion,
		OwnerIdentity:       proposal.OwnerIdentity,
		ProposalID:          proposal.ID,
		ProposalRevision:    proposal.Revision,
		ProposalDigest:      proposal.ProposalDigest,
		IdempotencyKey:      idempotencyKey,
		IntentDigest:        intentDigest,
		Mode:                mode,
		Status:              status,
		Target:              proposal.Target,
		ProtectedTarget:     proposal.ProtectedTarget,
		ApplierID:           strings.TrimSpace(service.promoter.ID()),
		CurrentVersion:      proposal.CurrentVersion,
		ProposedVersion:     proposal.ProposedVersion,
		RollbackPlan:        proposal.RollbackPlan,
		GovernanceReference: strings.TrimSpace(request.GovernanceReference),
		Attempt:             1,
		LeaseExpiresAt:      now.Add(applicationLeaseDuration),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	application.DefinitionDigest, err = applicationDefinitionDigest(application)
	if err != nil {
		return DecisionResult{}, err
	}
	application, execute, err := service.applicationRepository.AcquireApplication(ctx, application)
	if err != nil {
		return DecisionResult{}, err
	}
	if !execute {
		currentProposal, getErr := service.repository.GetProposal(
			ctx,
			proposal.OwnerIdentity,
			proposal.ID,
		)
		if getErr != nil {
			return DecisionResult{}, getErr
		}
		return service.completedApplicationDecision(currentProposal, application, nextStatus)
	}

	completion, promotionErr := service.promote(ctx, proposal, application)
	if promotionErr != nil {
		code := applicationFailureCode(promotionErr)
		failureContext := context.WithoutCancel(ctx)
		failed, persistErr := service.applicationRepository.FailApplication(
			failureContext,
			application.OwnerIdentity,
			application.ID,
			application.Attempt,
			code,
			service.now().UTC(),
		)
		if persistErr != nil {
			return DecisionResult{}, fmt.Errorf(
				"%w: failed to persist application failure",
				ErrApplicationFailed,
			)
		}
		return DecisionResult{
			Proposal:    proposal,
			Application: applicationPointer(failed),
		}, fmt.Errorf("%w: %s", ErrApplicationFailed, code)
	}
	decision, err := service.buildReviewDecision(
		proposal,
		request,
		application.ID,
		completion.CompletedAt,
	)
	if err != nil {
		return DecisionResult{}, err
	}
	updated, completed, err := service.applicationRepository.CompleteApplication(
		ctx,
		application.OwnerIdentity,
		application.ID,
		application.Attempt,
		decision,
		nextStatus,
		completion,
	)
	if err != nil {
		return DecisionResult{}, err
	}
	return DecisionResult{
		Proposal:    updated,
		Application: applicationPointer(completed),
	}, nil
}

func (service *Service) promote(
	ctx context.Context,
	proposal LearningProposal,
	application ApplicationRecord,
) (ApplicationCompletion, error) {
	now := service.now().UTC()
	switch application.Mode {
	case ApplicationModeApply:
		result, err := service.promoter.Apply(ctx, PromotionRequest{
			ApplicationID:   application.ID,
			IdempotencyKey:  application.IdempotencyKey,
			OwnerIdentity:   application.OwnerIdentity,
			ProposalID:      proposal.ID,
			ProposalDigest:  proposal.ProposalDigest,
			Target:          proposal.Target,
			CurrentVersion:  proposal.CurrentVersion,
			ProposedVersion: proposal.ProposedVersion,
			ProposedChange:  proposal.ProposedChange,
			RollbackPlan:    proposal.RollbackPlan,
			EvidenceIDs:     append([]string(nil), proposal.EvidenceIDs...),
		})
		if err != nil {
			return ApplicationCompletion{}, err
		}
		evidence, err := normalizeApplicationEvidence(result.Evidence, now)
		if err != nil {
			return ApplicationCompletion{}, err
		}
		if strings.TrimSpace(result.AppliedVersion) != proposal.ProposedVersion {
			return ApplicationCompletion{}, fmt.Errorf(
				"promoter applied version does not match the proposed version",
			)
		}
		if err := validateRequired("rollback token", result.RollbackToken, maxDetailLength); err != nil {
			return ApplicationCompletion{}, err
		}
		completion := ApplicationCompletion{
			Status:         ApplicationApplied,
			AppliedVersion: proposal.ProposedVersion,
			RollbackToken:  strings.TrimSpace(result.RollbackToken),
			Evidence:       evidence,
			CompletedAt:    now,
		}
		completion.ResultDigest, err = applicationCompletionDigest(application, completion)
		return completion, err
	case ApplicationModeProtectedHandoff:
		if !proposal.ProtectedTarget {
			return ApplicationCompletion{}, ErrIntegrityViolation
		}
		result, err := service.promoter.HandoffProtected(ctx, ProtectedHandoffRequest{
			ApplicationID:       application.ID,
			IdempotencyKey:      application.IdempotencyKey,
			OwnerIdentity:       application.OwnerIdentity,
			ProposalID:          proposal.ID,
			ProposalDigest:      proposal.ProposalDigest,
			Target:              proposal.Target,
			CurrentVersion:      proposal.CurrentVersion,
			ProposedVersion:     proposal.ProposedVersion,
			ProposedChange:      proposal.ProposedChange,
			RollbackPlan:        proposal.RollbackPlan,
			EvidenceIDs:         append([]string(nil), proposal.EvidenceIDs...),
			GovernanceReference: application.GovernanceReference,
		})
		if err != nil {
			return ApplicationCompletion{}, err
		}
		if err := validateRequired(
			"protected policy handoff reference",
			result.HandoffReference,
			maxDetailLength,
		); err != nil {
			return ApplicationCompletion{}, err
		}
		evidence, err := normalizeApplicationEvidence(result.Evidence, now)
		if err != nil {
			return ApplicationCompletion{}, err
		}
		completion := ApplicationCompletion{
			Status:           ApplicationHandoffReady,
			HandoffReference: strings.TrimSpace(result.HandoffReference),
			Evidence:         evidence,
			CompletedAt:      now,
		}
		completion.ResultDigest, err = applicationCompletionDigest(application, completion)
		return completion, err
	default:
		return ApplicationCompletion{}, ErrIntegrityViolation
	}
}

func (service *Service) replayApplicationDecision(
	ctx context.Context,
	proposal LearningProposal,
	request DecideRequest,
	mode ApplicationMode,
) (DecisionResult, error) {
	if service.applicationRepository == nil {
		return DecisionResult{}, ErrNotFound
	}
	application, err := service.applicationRepository.GetProposalApplication(
		ctx,
		proposal.OwnerIdentity,
		proposal.ID,
		request.ExpectedRevision,
		mode,
	)
	if err != nil {
		return DecisionResult{}, err
	}
	expectedIntent, err := decisionIntentDigest(proposal, request, mode)
	if err != nil {
		return DecisionResult{}, err
	}
	if application.IntentDigest != expectedIntent {
		return DecisionResult{}, ErrIdempotencyConflict
	}
	nextStatus := ProposalApproved
	if mode == ApplicationModeProtectedHandoff {
		nextStatus = ProposalGovernanceReview
	}
	return service.completedApplicationDecision(proposal, application, nextStatus)
}

func (service *Service) completedApplicationDecision(
	proposal LearningProposal,
	application ApplicationRecord,
	expectedProposalStatus ProposalStatus,
) (DecisionResult, error) {
	expectedApplicationStatus := ApplicationApplied
	if application.Mode == ApplicationModeProtectedHandoff {
		expectedApplicationStatus = ApplicationHandoffReady
	}
	if application.Status != expectedApplicationStatus {
		if application.Status == ApplicationFailed {
			return DecisionResult{}, ErrApplicationFailed
		}
		return DecisionResult{}, ErrApplicationInProgress
	}
	if proposal.Status != expectedProposalStatus ||
		proposal.Revision != application.ProposalRevision+1 {
		return DecisionResult{}, ErrIntegrityViolation
	}
	if err := verifyApplicationIntegrity(application); err != nil {
		return DecisionResult{}, err
	}
	return DecisionResult{
		Proposal:    proposal,
		Application: applicationPointer(application),
	}, nil
}

func (service *Service) Rollback(
	ctx context.Context,
	request RollbackRequest,
) (ApplicationRecord, error) {
	if err := ctx.Err(); err != nil {
		return ApplicationRecord{}, err
	}
	if service.promoter == nil || service.applicationRepository == nil {
		return ApplicationRecord{}, ErrRollbackUnavailable
	}
	for label, value := range map[string]string{
		"owner identity":           request.OwnerIdentity,
		"application id":           request.ApplicationID,
		"actor identity":           request.ActorIdentity,
		"rollback rationale":       request.Rationale,
		"expected applied version": request.ExpectedVersion,
	} {
		if err := validateRequired(label, value, maxDetailLength); err != nil {
			return ApplicationRecord{}, err
		}
	}
	owner := strings.TrimSpace(request.OwnerIdentity)
	if !request.HumanConfirmed {
		return ApplicationRecord{}, fmt.Errorf("controlled learning rollback requires explicit human confirmation")
	}
	if strings.TrimSpace(request.ActorIdentity) != owner {
		return ApplicationRecord{}, ErrOwnerScopeViolation
	}
	application, err := service.applicationRepository.GetApplication(
		ctx,
		owner,
		request.ApplicationID,
	)
	if err != nil {
		return ApplicationRecord{}, err
	}
	if application.Mode != ApplicationModeApply || application.ProtectedTarget {
		return ApplicationRecord{}, ErrRollbackUnavailable
	}
	if application.ApplierID != strings.TrimSpace(service.promoter.ID()) {
		return ApplicationRecord{}, ErrRollbackUnavailable
	}
	if application.AppliedVersion != strings.TrimSpace(request.ExpectedVersion) {
		return ApplicationRecord{}, ErrRevisionConflict
	}
	idempotencyKey := strings.TrimSpace(request.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = application.IdempotencyKey + ":rollback"
	}
	if err := validateRequired("rollback idempotency key", idempotencyKey, maxIdentifierLength); err != nil {
		return ApplicationRecord{}, err
	}
	intentDigest, err := rollbackIntentDigest(application, request, idempotencyKey)
	if err != nil {
		return ApplicationRecord{}, err
	}
	now := service.now().UTC()
	application, execute, err := service.applicationRepository.AcquireRollback(
		ctx,
		owner,
		application.ID,
		intentDigest,
		now,
		now.Add(applicationLeaseDuration),
	)
	if err != nil {
		return ApplicationRecord{}, err
	}
	if !execute {
		if application.Status == ApplicationRolledBack {
			return application, nil
		}
		return ApplicationRecord{}, ErrApplicationInProgress
	}
	result, promotionErr := service.promoter.Rollback(ctx, PromotionRollbackRequest{
		ApplicationID:  application.ID,
		IdempotencyKey: idempotencyKey,
		OwnerIdentity:  application.OwnerIdentity,
		ProposalID:     application.ProposalID,
		Target:         application.Target,
		AppliedVersion: application.AppliedVersion,
		RestoreVersion: application.CurrentVersion,
		RollbackPlan:   application.RollbackPlan,
		RollbackToken:  application.RollbackToken,
	})
	if promotionErr != nil {
		code := applicationFailureCode(promotionErr)
		failed, persistErr := service.applicationRepository.FailRollback(
			context.WithoutCancel(ctx),
			owner,
			application.ID,
			application.Attempt,
			code,
			service.now().UTC(),
		)
		if persistErr != nil {
			return ApplicationRecord{}, ErrApplicationFailed
		}
		return failed, fmt.Errorf("%w: %s", ErrApplicationFailed, code)
	}
	result.Evidence, err = normalizeApplicationEvidence(result.Evidence, service.now().UTC())
	if err != nil || strings.TrimSpace(result.RestoredVersion) != application.CurrentVersion {
		failed, persistErr := service.applicationRepository.FailRollback(
			context.WithoutCancel(ctx),
			owner,
			application.ID,
			application.Attempt,
			"invalid_rollback_result",
			service.now().UTC(),
		)
		if persistErr != nil {
			return ApplicationRecord{}, ErrApplicationFailed
		}
		return failed, fmt.Errorf("%w: invalid_rollback_result", ErrApplicationFailed)
	}
	return service.applicationRepository.CompleteRollback(
		ctx,
		owner,
		application.ID,
		application.Attempt,
		result,
		service.now().UTC(),
	)
}

func (service *Service) GetApplication(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
) (ApplicationRecord, error) {
	if service.applicationRepository == nil {
		return ApplicationRecord{}, ErrPromoterUnavailable
	}
	return service.applicationRepository.GetApplication(ctx, ownerIdentity, applicationID)
}

func (service *Service) ListApplications(
	ctx context.Context,
	query ApplicationQuery,
) ([]ApplicationRecord, error) {
	if service.applicationRepository == nil {
		return nil, ErrPromoterUnavailable
	}
	return service.applicationRepository.ListApplications(ctx, query)
}

func (service *Service) ListApplicationEvents(
	ctx context.Context,
	ownerIdentity string,
	applicationID string,
) ([]ApplicationEvent, error) {
	if service.applicationRepository == nil {
		return nil, ErrPromoterUnavailable
	}
	return service.applicationRepository.ListApplicationEvents(ctx, ownerIdentity, applicationID)
}

func applicationModeForDecision(kind DecisionKind) (ApplicationMode, bool) {
	switch kind {
	case DecisionApprove:
		return ApplicationModeApply, true
	case DecisionEscalateGovernance:
		return ApplicationModeProtectedHandoff, true
	default:
		return "", false
	}
}

func decisionApplicationIdempotencyKey(
	proposal LearningProposal,
	request DecideRequest,
) (string, error) {
	value := strings.TrimSpace(request.IdempotencyKey)
	if value == "" {
		value = fmt.Sprintf(
			"proposal:%s:revision:%d:%s",
			proposal.ID,
			request.ExpectedRevision,
			request.Kind,
		)
	}
	if err := validateRequired("application idempotency key", value, maxIdentifierLength); err != nil {
		return "", err
	}
	return value, nil
}

func decisionIntentDigest(
	proposal LearningProposal,
	request DecideRequest,
	mode ApplicationMode,
) (string, error) {
	return digestValue(struct {
		OwnerIdentity       string
		ProposalID          string
		ProposalRevision    int64
		ProposalDigest      string
		Kind                DecisionKind
		Mode                ApplicationMode
		ActorIdentity       string
		HumanConfirmed      bool
		Rationale           string
		GovernanceReference string
	}{
		strings.TrimSpace(request.OwnerIdentity),
		proposal.ID,
		request.ExpectedRevision,
		proposal.ProposalDigest,
		request.Kind,
		mode,
		strings.TrimSpace(request.ActorIdentity),
		request.HumanConfirmed,
		strings.TrimSpace(request.Rationale),
		strings.TrimSpace(request.GovernanceReference),
	})
}

func rollbackIntentDigest(
	application ApplicationRecord,
	request RollbackRequest,
	idempotencyKey string,
) (string, error) {
	return digestValue(struct {
		ApplicationID    string
		DefinitionDigest string
		ResultDigest     string
		IdempotencyKey   string
		ActorIdentity    string
		Rationale        string
		ExpectedVersion  string
	}{
		application.ID,
		application.DefinitionDigest,
		application.ResultDigest,
		idempotencyKey,
		strings.TrimSpace(request.ActorIdentity),
		strings.TrimSpace(request.Rationale),
		strings.TrimSpace(request.ExpectedVersion),
	})
}

func applicationDefinitionDigest(application ApplicationRecord) (string, error) {
	return digestValue(struct {
		ProtocolVersion     string
		OwnerIdentity       string
		ProposalID          string
		ProposalRevision    int64
		ProposalDigest      string
		IdempotencyKey      string
		IntentDigest        string
		Mode                ApplicationMode
		Target              TargetKind
		ProtectedTarget     bool
		ApplierID           string
		CurrentVersion      string
		ProposedVersion     string
		RollbackPlan        string
		GovernanceReference string
	}{
		application.ProtocolVersion,
		application.OwnerIdentity,
		application.ProposalID,
		application.ProposalRevision,
		application.ProposalDigest,
		application.IdempotencyKey,
		application.IntentDigest,
		application.Mode,
		application.Target,
		application.ProtectedTarget,
		application.ApplierID,
		application.CurrentVersion,
		application.ProposedVersion,
		application.RollbackPlan,
		application.GovernanceReference,
	})
}

func applicationCompletionDigest(
	application ApplicationRecord,
	completion ApplicationCompletion,
) (string, error) {
	return digestValue(struct {
		DefinitionDigest string
		Status           ApplicationStatus
		AppliedVersion   string
		RollbackToken    string
		HandoffReference string
		Evidence         []ApplicationEvidence
		CompletedAt      time.Time
	}{
		application.DefinitionDigest,
		completion.Status,
		completion.AppliedVersion,
		completion.RollbackToken,
		completion.HandoffReference,
		completion.Evidence,
		completion.CompletedAt,
	})
}

func verifyApplicationIntegrity(application ApplicationRecord) error {
	expected, err := applicationDefinitionDigest(application)
	if err != nil {
		return err
	}
	if application.DefinitionDigest == "" || application.DefinitionDigest != expected {
		return ErrIntegrityViolation
	}
	if application.Status == ApplicationApplied ||
		application.Status == ApplicationHandoffReady ||
		application.Status == ApplicationRolledBack {
		completion := ApplicationCompletion{
			Status:           applicationStatusBeforeRollback(application),
			AppliedVersion:   application.AppliedVersion,
			RollbackToken:    application.RollbackToken,
			HandoffReference: application.HandoffReference,
			Evidence:         application.Evidence,
			CompletedAt:      application.CompletedAt,
		}
		resultDigest, err := applicationCompletionDigest(application, completion)
		if err != nil {
			return err
		}
		if application.ResultDigest == "" || application.ResultDigest != resultDigest {
			return ErrIntegrityViolation
		}
	}
	return nil
}

func applicationStatusBeforeRollback(application ApplicationRecord) ApplicationStatus {
	if application.Mode == ApplicationModeProtectedHandoff {
		return ApplicationHandoffReady
	}
	return ApplicationApplied
}

func normalizeApplicationEvidence(
	evidence []ApplicationEvidence,
	now time.Time,
) ([]ApplicationEvidence, error) {
	if len(evidence) == 0 {
		return nil, fmt.Errorf("application result requires durable evidence")
	}
	if len(evidence) > maxCollectionSize {
		return nil, fmt.Errorf("application evidence may contain at most %d items", maxCollectionSize)
	}
	result := append([]ApplicationEvidence(nil), evidence...)
	seen := make(map[string]struct{}, len(result))
	for index := range result {
		result[index].ID = strings.TrimSpace(result[index].ID)
		result[index].Kind = strings.TrimSpace(result[index].Kind)
		result[index].URI = strings.TrimSpace(result[index].URI)
		result[index].Digest = strings.TrimSpace(result[index].Digest)
		result[index].RecordedAt = result[index].RecordedAt.UTC()
		for label, value := range map[string]string{
			"application evidence id":     result[index].ID,
			"application evidence kind":   result[index].Kind,
			"application evidence uri":    result[index].URI,
			"application evidence digest": result[index].Digest,
		} {
			if err := validateRequired(label, value, maxDetailLength); err != nil {
				return nil, err
			}
		}
		if _, exists := seen[result[index].ID]; exists {
			return nil, fmt.Errorf("duplicate application evidence id %q", result[index].ID)
		}
		seen[result[index].ID] = struct{}{}
		parsed, err := url.Parse(result[index].URI)
		if err != nil || parsed.Scheme == "" || parsed.User != nil {
			return nil, fmt.Errorf("invalid application evidence URI")
		}
		if !sha256DigestPattern.MatchString(result[index].Digest) {
			return nil, fmt.Errorf("invalid application evidence digest")
		}
		if result[index].RecordedAt.IsZero() ||
			result[index].RecordedAt.After(now.Add(futureTolerance)) {
			return nil, fmt.Errorf("invalid application evidence recorded time")
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID == result[j].ID {
			return result[i].URI < result[j].URI
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func applicationFailureCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "promoter_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "promoter_deadline_exceeded"
	default:
		return "promoter_error"
	}
}

func applicationPointer(value ApplicationRecord) *ApplicationRecord {
	copy := cloneApplication(value)
	return &copy
}

package ambientmonitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Service) PendingCompositionScopes(ctx context.Context, at time.Time, limit int) ([]Scope, error) {
	if err := s.available(false); err != nil {
		return nil, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	cleanAt, err := validateRequestTime("composition scope time", at, now)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > maxClaimLimit {
		return nil, fmt.Errorf("%w: composition scope limit is invalid", ErrInvalidInput)
	}
	return s.repository.ListPendingCompositionScopes(ctx, cleanAt, limit)
}

func (s *Service) ProcessCompositions(ctx context.Context, request ProcessCompositionsRequest) (ProcessCompositionsResult, error) {
	if err := s.available(false); err != nil {
		return ProcessCompositionsResult{}, err
	}
	scope, err := validateScope(request.Scope)
	if err != nil {
		return ProcessCompositionsResult{}, err
	}
	worker := strings.TrimSpace(request.WorkerID)
	if err := validateIdentifier("composition worker id", worker); err != nil {
		return ProcessCompositionsResult{}, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	if request.Now, err = validateRequestTime("composition processing time", request.Now, now); err != nil {
		return ProcessCompositionsResult{}, err
	}
	if err := validateLeaseDuration(request.LeaseDuration); err != nil {
		return ProcessCompositionsResult{}, err
	}
	if request.Limit < 1 || request.Limit > maxClaimLimit {
		return ProcessCompositionsResult{}, fmt.Errorf("%w: composition limit is invalid", ErrInvalidInput)
	}
	claimed, err := s.repository.ClaimDueCompositions(ctx, scope.OwnerID, scope.WorkspaceID, worker, request.Now, request.LeaseDuration, request.Limit)
	if err != nil {
		return ProcessCompositionsResult{}, err
	}
	result := ProcessCompositionsResult{Claimed: len(claimed), Failures: []CompositionFailure{}, Records: []CompositionDelivery{}, Authority: advisoryAuthority()}
	for _, delivery := range claimed {
		finishedAt := s.now().UTC().Truncate(time.Microsecond)
		if !finishedAt.After(delivery.Lease.ClaimedAt) {
			finishedAt = delivery.Lease.ClaimedAt.Add(time.Microsecond)
		}
		attempt, buildErr := newCompositionAttempt(delivery, worker, finishedAt)
		if buildErr != nil {
			return result, buildErr
		}
		signal, signalErr := s.repository.LoadCompositionSignal(ctx, scope.OwnerID, scope.WorkspaceID, delivery.ID)
		var compositionResult CompositionResult
		composeErr := signalErr
		if composeErr == nil && signal.Snapshot.Status != CompositionSnapshotPinned {
			composeErr = ErrSnapshotUnavailable
		}
		if composeErr == nil {
			if s.sink == nil || isTypedNil(s.sink) {
				composeErr = ErrSinkFailed
			} else {
				compositionResult, composeErr = s.sink.Compose(ctx, signal)
			}
		}
		if composeErr == nil && compositionResult.DisableTarget {
			_, _, composeErr = s.SetEnabled(ctx, SetEnabledRequest{
				IdempotencyKey: "window-closed-" + delivery.RunDigest[:32],
				Scope:          scope, TargetID: delivery.TargetID, Enabled: false,
				RequestedAt: finishedAt.Add(time.Microsecond),
			})
		}
		if composeErr == nil {
			attempt.Status = CompositionAttemptSucceeded
			attempt.RecordDigest, err = compositionAttemptDigest(attempt)
			if err != nil {
				return result, err
			}
			stored, _, finishErr := s.repository.CompleteComposition(ctx, scope.OwnerID, scope.WorkspaceID, delivery.ID, worker, delivery.Lease.Generation, attempt, finishedAt)
			if finishErr != nil {
				return result, finishErr
			}
			result.Succeeded++
			result.Records = append(result.Records, stored)
			continue
		}
		failureCode := compositionFailureCode(composeErr)
		attempt.Status = CompositionAttemptFailed
		attempt.FailureCode = failureCode
		attempt.RecordDigest, err = compositionAttemptDigest(attempt)
		if err != nil {
			return result, err
		}
		deadLetter := delivery.AttemptCount+1 >= delivery.MaxAttempts
		next := finishedAt.Add(compositionRetryDelay(delivery.AttemptCount + 1))
		stored, _, finishErr := s.repository.FailComposition(ctx, scope.OwnerID, scope.WorkspaceID, delivery.ID, worker, delivery.Lease.Generation, attempt, next, deadLetter)
		if finishErr != nil {
			return result, finishErr
		}
		result.Failures = append(result.Failures, CompositionFailure{DeliveryID: delivery.ID, Code: failureCode, Retrying: stored.Status == CompositionPending})
		result.Records = append(result.Records, stored)
	}
	return result, nil
}

func (s *Service) RecoverExpiredCompositionLeases(ctx context.Context, scope Scope, at time.Time) (int, error) {
	if err := s.available(false); err != nil {
		return 0, err
	}
	clean, err := validateScope(scope)
	if err != nil {
		return 0, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	if at, err = validateRequestTime("composition recovery time", at, now); err != nil {
		return 0, err
	}
	return s.repository.RecoverExpiredCompositionLeases(ctx, clean.OwnerID, clean.WorkspaceID, at)
}

func (s *Service) Compositions(ctx context.Context, scope Scope, targetID string, limit int) ([]CompositionDelivery, error) {
	if err := s.available(false); err != nil {
		return nil, err
	}
	clean, err := validateScope(scope)
	if err != nil {
		return nil, err
	}
	if err := validateIdentifier("target id", strings.TrimSpace(targetID)); err != nil {
		return nil, err
	}
	return s.repository.ListCompositions(ctx, clean.OwnerID, clean.WorkspaceID, strings.TrimSpace(targetID), limit)
}

func (s *Service) Composition(ctx context.Context, scope Scope, deliveryID string) (CompositionDelivery, error) {
	if err := s.available(false); err != nil {
		return CompositionDelivery{}, err
	}
	clean, err := validateScope(scope)
	if err != nil {
		return CompositionDelivery{}, err
	}
	if err := validateIdentifier("composition id", strings.TrimSpace(deliveryID)); err != nil {
		return CompositionDelivery{}, err
	}
	return s.repository.GetComposition(ctx, clean.OwnerID, clean.WorkspaceID, strings.TrimSpace(deliveryID))
}

func (s *Service) CompositionAttempts(ctx context.Context, scope Scope, deliveryID string, limit int) ([]CompositionAttempt, error) {
	if err := s.available(false); err != nil {
		return nil, err
	}
	clean, err := validateScope(scope)
	if err != nil {
		return nil, err
	}
	if err := validateIdentifier("composition id", strings.TrimSpace(deliveryID)); err != nil {
		return nil, err
	}
	return s.repository.ListCompositionAttempts(ctx, clean.OwnerID, clean.WorkspaceID, strings.TrimSpace(deliveryID), limit)
}

func newCompositionAttempt(delivery CompositionDelivery, worker string, finishedAt time.Time) (CompositionAttempt, error) {
	id, err := newRecordID("cat")
	if err != nil {
		return CompositionAttempt{}, err
	}
	attempt := CompositionAttempt{
		ContractVersion: ContractVersion, ID: id, Scope: delivery.Scope,
		DeliveryID: delivery.ID, TargetID: delivery.TargetID,
		RunID: delivery.RunID, RunDigest: delivery.RunDigest, SnapshotDigest: delivery.Snapshot.SnapshotDigest,
		AttemptNumber: delivery.AttemptCount + 1, LeaseGeneration: delivery.Lease.Generation,
		WorkerID: worker, StartedAt: delivery.Lease.ClaimedAt, FinishedAt: finishedAt,
		Authority: advisoryAuthority(),
	}
	attempt.RequestDigest, err = compositionAttemptRequestDigest(attempt)
	return attempt, err
}

func compositionAttemptRequestDigest(attempt CompositionAttempt) (string, error) {
	return exactDigest("composition_attempt_request", struct {
		Scope                           Scope
		DeliveryID, RunDigest, WorkerID string
		AttemptNumber                   int
		LeaseGeneration                 uint64
		SnapshotDigest                  string
		StartedAt                       time.Time
	}{attempt.Scope, attempt.DeliveryID, attempt.RunDigest, attempt.WorkerID, attempt.AttemptNumber, attempt.LeaseGeneration, attempt.SnapshotDigest, attempt.StartedAt})
}

func compositionAttemptDigest(attempt CompositionAttempt) (string, error) {
	copy := attempt
	copy.ID = ""
	copy.RecordDigest = ""
	return exactDigest("composition_attempt", copy)
}

func compositionFailureCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "cancelled"
	case errors.Is(err, ErrScopeViolation):
		return "scope_violation"
	case errors.Is(err, ErrCorruptStorage):
		return "source_corrupt"
	case errors.Is(err, ErrSnapshotUnavailable):
		return "snapshot_unavailable"
	default:
		return "advisory_composition_failed"
	}
}

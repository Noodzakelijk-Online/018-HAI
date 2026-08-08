package standingmandate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Clock func() time.Time

type Service struct {
	repository Repository
	now        Clock
	lifeGraph  LifeOntologyProjector
}

func NewService(repository Repository, now Clock) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("standing mandate repository is required")
	}
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, now: now}, nil
}

func (s *Service) WithLifeOntologyProjection(projector LifeOntologyProjector) (*Service, error) {
	if s == nil || projector == nil {
		return nil, fmt.Errorf("standing mandate service and life ontology projector are required")
	}
	s.lifeGraph = projector
	return s, nil
}

func (s *Service) Create(ctx context.Context, request CreateRequest) (*StandingMandate, error) {
	now := s.now().UTC()
	if err := validateCreateRequest(request, now); err != nil {
		return nil, err
	}
	mandate := StandingMandate{
		ID:               uuid.New(),
		OwnerIdentity:    strings.TrimSpace(request.OwnerIdentity),
		Name:             strings.TrimSpace(request.Name),
		Purpose:          strings.TrimSpace(request.Purpose),
		Status:           StatusDraft,
		Version:          firstNonEmpty(request.Version, "1.0.0"),
		Revision:         1,
		Scopes:           cloneScopes(request.Scopes),
		AutonomyCeiling:  request.AutonomyCeiling,
		ApprovalPolicy:   cloneApprovalPolicy(request.ApprovalPolicy),
		StopConditions:   append([]StopCondition(nil), request.StopConditions...),
		SourceReferences: cleanValues(request.SourceReferences),
		CreatedBy:        strings.TrimSpace(request.CreatedBy),
		CreatedAt:        now,
		UpdatedAt:        now,
		ExpiresAt:        cloneTime(request.ExpiresAt),
	}
	if err := s.repository.Create(ctx, mandate); err != nil {
		return nil, err
	}
	s.projectMandate(ctx, &mandate)
	return cloneMandatePointer(&mandate), nil
}

func (s *Service) Activate(
	ctx context.Context,
	ownerIdentity string,
	id uuid.UUID,
	expectedRevision uint64,
) (*StandingMandate, error) {
	mandate, err := s.repository.Get(ctx, strings.TrimSpace(ownerIdentity), id)
	if err != nil {
		return nil, err
	}
	if mandate.Revision != expectedRevision {
		return nil, ErrRevisionConflict
	}
	if mandate.Status != StatusDraft {
		return nil, fmt.Errorf("only draft mandates can be activated")
	}
	now := monotonicTime(s.now(), mandate.UpdatedAt)
	if mandate.ExpiresAt != nil && !now.Before(*mandate.ExpiresAt) {
		return nil, fmt.Errorf("expired mandate cannot be activated")
	}
	mandate.Status = StatusActive
	mandate.ActivatedAt = &now
	mandate.UpdatedAt = now
	mandate.Revision++
	if err := s.repository.Update(ctx, *mandate, expectedRevision); err != nil {
		return nil, err
	}
	s.projectMandate(ctx, mandate)
	return cloneMandatePointer(mandate), nil
}

func (s *Service) Revoke(
	ctx context.Context,
	ownerIdentity string,
	id uuid.UUID,
	expectedRevision uint64,
	revokedBy string,
	reason string,
) (*StandingMandate, error) {
	revokedBy = strings.TrimSpace(revokedBy)
	reason = strings.TrimSpace(reason)
	if revokedBy == "" || reason == "" {
		return nil, fmt.Errorf("revoker identity and reason are required")
	}
	mandate, err := s.repository.Get(ctx, strings.TrimSpace(ownerIdentity), id)
	if err != nil {
		return nil, err
	}
	if mandate.Revision != expectedRevision {
		return nil, ErrRevisionConflict
	}
	if mandate.Status == StatusRevoked {
		return nil, fmt.Errorf("mandate is already revoked")
	}
	now := monotonicTime(s.now(), mandate.UpdatedAt)
	mandate.Status = StatusRevoked
	mandate.RevokedAt = &now
	mandate.RevokedBy = revokedBy
	mandate.RevocationReason = reason
	mandate.UpdatedAt = now
	mandate.Revision++
	if err := s.repository.Update(ctx, *mandate, expectedRevision); err != nil {
		return nil, err
	}
	s.projectMandate(ctx, mandate)
	return cloneMandatePointer(mandate), nil
}

func (s *Service) Get(ctx context.Context, ownerIdentity string, id uuid.UUID) (*StandingMandate, error) {
	return s.repository.Get(ctx, strings.TrimSpace(ownerIdentity), id)
}

// GetAuthorizationSnapshot resolves the current owner-scoped policy identity
// without exposing mutable authority to the caller. Execution authorization
// uses this immediately before a final effect to detect revocation, expiry,
// revision changes, or persisted policy drift.
func (s *Service) GetAuthorizationSnapshot(
	ctx context.Context,
	ownerIdentity string,
	id uuid.UUID,
) (AuthorizationSnapshot, error) {
	mandate, err := s.repository.Get(ctx, strings.TrimSpace(ownerIdentity), id)
	if err != nil {
		return AuthorizationSnapshot{}, err
	}
	value, err := digest(normalizedMandate(*mandate))
	if err != nil {
		return AuthorizationSnapshot{}, fmt.Errorf("digest standing mandate snapshot: %w", err)
	}
	return AuthorizationSnapshot{
		ID:            mandate.ID,
		OwnerIdentity: mandate.OwnerIdentity,
		Status:        mandate.Status,
		Revision:      mandate.Revision,
		ExpiresAt:     cloneTime(mandate.ExpiresAt),
		Digest:        value,
	}, nil
}

func (s *Service) List(ctx context.Context, ownerIdentity string) ([]StandingMandate, error) {
	return s.repository.List(ctx, strings.TrimSpace(ownerIdentity))
}

func (s *Service) GetDecision(
	ctx context.Context,
	ownerIdentity string,
	id uuid.UUID,
) (*AuthorizationDecision, error) {
	return s.repository.GetDecision(ctx, strings.TrimSpace(ownerIdentity), id)
}

func (s *Service) ListDecisions(
	ctx context.Context,
	ownerIdentity string,
	mandateID *uuid.UUID,
	limit int,
) ([]AuthorizationDecision, error) {
	return s.repository.ListDecisions(
		ctx,
		strings.TrimSpace(ownerIdentity),
		mandateID,
		limit,
	)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

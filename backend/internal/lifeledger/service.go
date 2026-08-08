package lifeledger

import (
	"context"
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/lifeontology"

	"github.com/google/uuid"
)

type Clock func() time.Time

type Projector interface {
	ProjectOperationalRecord(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error)
}

type Service struct {
	repository Repository
	clock      Clock
	projector  Projector
}

func NewService(repository Repository, clock Clock) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("life ledger repository is required")
	}
	if clock == nil {
		clock = time.Now
	}
	return &Service{repository: repository, clock: clock}, nil
}

func (s *Service) WithProjection(projector Projector) (*Service, error) {
	if s == nil || projector == nil {
		return nil, fmt.Errorf("life ledger service and projector are required")
	}
	s.projector = projector
	return s, nil
}

func (s *Service) RecordCommitment(ctx context.Context, request RecordCommitmentRequest) (CommitmentWriteResult, error) {
	now := s.clock().UTC()
	normalized, err := normalizeCommitmentRequest(request, now)
	if err != nil {
		return CommitmentWriteResult{}, err
	}
	if normalized.ExpectedRevision > 0 {
		current, getErr := s.repository.GetCommitment(ctx, normalized.OwnerIdentity, normalized.CommitmentKey)
		if getErr != nil {
			return CommitmentWriteResult{}, getErr
		}
		if current.Revision != normalized.ExpectedRevision {
			return CommitmentWriteResult{}, ErrRevisionConflict
		}
		if !validCommitmentTransition(current.Status, normalized.Status) {
			return CommitmentWriteResult{}, fmt.Errorf("commitment transition %q -> %q is not allowed", current.Status, normalized.Status)
		}
	}
	requestDigest, err := commitmentRequestDigest(normalized)
	if err != nil {
		return CommitmentWriteResult{}, err
	}
	record := CommitmentRevision{
		ContractVersion: ContractVersion, ID: uuid.New(), OwnerIdentity: normalized.OwnerIdentity,
		CommitmentKey: normalized.CommitmentKey, Revision: normalized.ExpectedRevision + 1,
		Domain: normalized.Domain, Title: normalized.Title, Summary: normalized.Summary,
		Status: normalized.Status, Counterparty: normalized.Counterparty, ProjectKey: normalized.ProjectKey,
		DueAt: normalized.DueAt, Verification: normalized.Verification, Evidence: normalized.Evidence,
		LocalOnly: true, IdempotencyKey: normalized.IdempotencyKey, RequestDigest: requestDigest,
		ObservedAt: normalized.ObservedAt, RecordedAt: now,
	}
	record.RecordDigest, err = commitmentRecordDigest(record)
	if err != nil {
		return CommitmentWriteResult{}, err
	}
	stored, created, err := s.repository.SaveCommitment(ctx, record, normalized.ExpectedRevision)
	if err != nil {
		return CommitmentWriteResult{}, err
	}
	s.projectCommitment(ctx, &stored)
	return CommitmentWriteResult{Record: stored, Created: created}, nil
}

func (s *Service) RecordCost(ctx context.Context, request RecordCostRequest) (CostWriteResult, error) {
	now := s.clock().UTC()
	normalized, err := normalizeCostRequest(request, now)
	if err != nil {
		return CostWriteResult{}, err
	}
	if normalized.CommitmentKey != "" {
		if _, err := s.repository.GetCommitment(ctx, normalized.OwnerIdentity, normalized.CommitmentKey); err != nil {
			return CostWriteResult{}, fmt.Errorf("linked commitment: %w", err)
		}
	}
	requestDigest, err := costRequestDigest(normalized)
	if err != nil {
		return CostWriteResult{}, err
	}
	record := CostEntry{
		ContractVersion: ContractVersion, ID: uuid.New(), OwnerIdentity: normalized.OwnerIdentity,
		Domain: normalized.Domain, Title: normalized.Title, Summary: normalized.Summary,
		Kind: normalized.Kind, AmountMinor: normalized.AmountMinor, Currency: normalized.Currency,
		CommitmentKey: normalized.CommitmentKey, ProjectKey: normalized.ProjectKey,
		Verification: normalized.Verification, Evidence: normalized.Evidence, LocalOnly: true,
		IdempotencyKey: normalized.IdempotencyKey, RequestDigest: requestDigest,
		ObservedAt: normalized.ObservedAt, RecordedAt: now,
	}
	record.RecordDigest, err = costRecordDigest(record)
	if err != nil {
		return CostWriteResult{}, err
	}
	stored, created, err := s.repository.AppendCost(ctx, record)
	if err != nil {
		return CostWriteResult{}, err
	}
	s.projectCost(ctx, &stored)
	return CostWriteResult{Record: stored, Created: created}, nil
}

func (s *Service) GetCommitment(ctx context.Context, owner, key string) (CommitmentRevision, error) {
	owner, key = clean(owner), clean(key)
	if err := bounded("owner identity", owner, 255, true); err != nil {
		return CommitmentRevision{}, err
	}
	if err := bounded("commitment key", key, 256, true); err != nil {
		return CommitmentRevision{}, err
	}
	return s.repository.GetCommitment(ctx, owner, key)
}

func (s *Service) ListCommitments(ctx context.Context, owner string, limit int) ([]CommitmentRevision, error) {
	owner = clean(owner)
	if err := bounded("owner identity", owner, 255, true); err != nil {
		return nil, err
	}
	return s.repository.ListCommitments(ctx, owner, boundedLimit(limit))
}

func (s *Service) CommitmentHistory(ctx context.Context, owner, key string, limit int) ([]CommitmentRevision, error) {
	owner, key = clean(owner), clean(key)
	if err := bounded("owner identity", owner, 255, true); err != nil {
		return nil, err
	}
	if err := bounded("commitment key", key, 256, true); err != nil {
		return nil, err
	}
	return s.repository.ListCommitmentHistory(ctx, owner, key, boundedLimit(limit))
}

func (s *Service) ListCosts(ctx context.Context, owner string, limit int) ([]CostEntry, error) {
	owner = strings.TrimSpace(owner)
	if err := bounded("owner identity", owner, 255, true); err != nil {
		return nil, err
	}
	return s.repository.ListCosts(ctx, owner, boundedLimit(limit))
}

func validCommitmentTransition(from, to CommitmentStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case CommitmentProposed:
		return to == CommitmentActive || to == CommitmentWaiting || to == CommitmentCancelled || to == CommitmentDisputed
	case CommitmentActive:
		return to == CommitmentWaiting || to == CommitmentFulfilled || to == CommitmentCancelled || to == CommitmentBreached || to == CommitmentDisputed
	case CommitmentWaiting:
		return to == CommitmentActive || to == CommitmentFulfilled || to == CommitmentCancelled || to == CommitmentBreached || to == CommitmentDisputed
	case CommitmentDisputed:
		return to == CommitmentActive || to == CommitmentWaiting || to == CommitmentFulfilled || to == CommitmentCancelled || to == CommitmentBreached
	default:
		return false
	}
}

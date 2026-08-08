package plangraph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
	now        func() time.Time
	newID      func() uuid.UUID
}

// AcceptedRevisionResolver is the narrow read-only contract orchestration
// engines use. Resolving a plan never authorizes execution.
type AcceptedRevisionResolver interface {
	ResolveAccepted(ctx context.Context, ownerIdentity string, reference AcceptedRevisionReference) (*AcceptedRevisionBinding, error)
}

// AcceptedRevisionHistoryResolver resolves immutable accepted provenance even
// after a later revision exists. It is reserved for idempotent recovery of an
// already authorized and consumed effect, never for authorizing new work.
type AcceptedRevisionHistoryResolver interface {
	ResolveAcceptedRevision(ctx context.Context, ownerIdentity string, reference AcceptedRevisionReference) (*AcceptedRevisionBinding, error)
}

func NewService(repository Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, now: now, newID: uuid.New}
}

func (service *Service) Preview(ctx context.Context, ownerIdentity string, request PreviewRequest) (*Plan, error) {
	if err := service.ready(); err != nil {
		return nil, err
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	key := strings.TrimSpace(request.IdempotencyKey)
	if key == "" || len(key) > 255 {
		return nil, fmt.Errorf("idempotency key is required and must not exceed 255 characters")
	}
	requestDigest, err := computeRequestDigest(request.Title, request.Nodes, request.Edges, "", "")
	if err != nil {
		return nil, err
	}
	if existing, err := service.repository.FindByIdempotencyKey(ctx, ownerIdentity, key); err == nil {
		if requestDigest != existing.RequestDigest {
			return nil, ErrIdempotencyConflict
		}
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	planID := request.PlanID
	if planID == uuid.Nil {
		planID = service.newID()
	}
	plan := normalizePlan(Plan{
		ID: planID, OwnerIdentity: ownerIdentity, Title: request.Title,
		Status: StatusDraft, Revision: 1, IdempotencyKey: key, RequestDigest: requestDigest,
		Nodes: request.Nodes, Edges: request.Edges,
		CreatedBy: request.CreatedBy, CreatedAt: utcNow(service.now), CanExecute: false,
	})
	if err := validatePlan(plan); err != nil {
		return nil, err
	}
	digest, err := computeDigest(plan)
	if err != nil {
		return nil, err
	}
	plan.Digest = digest
	if err := service.repository.CreateRevision(ctx, plan, 0); err != nil {
		if errors.Is(err, ErrIdempotencyConflict) || errors.Is(err, ErrRevisionConflict) {
			if existing, findErr := service.repository.FindByIdempotencyKey(ctx, ownerIdentity, key); findErr == nil && existing.Digest == plan.Digest {
				return existing, nil
			}
		}
		return nil, err
	}
	return pointerPlan(plan), nil
}

func (service *Service) Accept(ctx context.Context, ownerIdentity string, id uuid.UUID, request AcceptRequest) (*Plan, error) {
	if err := service.ready(); err != nil {
		return nil, err
	}
	current, err := service.repository.GetLatest(ctx, strings.TrimSpace(ownerIdentity), id)
	if err != nil {
		return nil, err
	}
	expectedDigest := strings.TrimSpace(request.ExpectedDigest)
	if current.Revision != request.ExpectedRevision || current.Digest != expectedDigest {
		if current.Status == StatusAccepted && current.ParentRevision == request.ExpectedRevision && current.ParentDigest == expectedDigest {
			return current, nil
		}
		return nil, ErrRevisionConflict
	}
	if current.Status == StatusAccepted {
		return current, nil
	}
	acceptedBy := strings.TrimSpace(request.AcceptedBy)
	if acceptedBy == "" {
		return nil, fmt.Errorf("acceptedBy is required")
	}
	now := utcNow(service.now)
	next := normalizePlan(Plan{
		ID: current.ID, OwnerIdentity: current.OwnerIdentity, Title: current.Title,
		Status: StatusAccepted, Revision: current.Revision + 1,
		ParentRevision: current.Revision, ParentDigest: current.Digest,
		Nodes: current.Nodes, Edges: current.Edges,
		CreatedBy: acceptedBy, CreatedAt: now, AcceptedAt: &now, CanExecute: false,
	})
	if err := validatePlan(next); err != nil {
		return nil, err
	}
	next.Digest, err = computeDigest(next)
	if err != nil {
		return nil, err
	}
	if err := service.repository.CreateRevision(ctx, next, current.Revision); err != nil {
		return nil, err
	}
	return pointerPlan(next), nil
}

func (service *Service) Replan(ctx context.Context, ownerIdentity string, id uuid.UUID, request ReplanRequest) (*Plan, error) {
	if err := service.ready(); err != nil {
		return nil, err
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	key := strings.TrimSpace(request.IdempotencyKey)
	if key == "" || len(key) > 255 {
		return nil, fmt.Errorf("idempotency key is required and must not exceed 255 characters")
	}
	requestDigest, err := computeRequestDigest(request.Title, request.Nodes, request.Edges, request.Reason, request.Trigger)
	if err != nil {
		return nil, err
	}
	if existing, findErr := service.repository.FindByIdempotencyKey(ctx, ownerIdentity, key); findErr == nil {
		if existing.ID != id || existing.RequestDigest != requestDigest {
			return nil, ErrIdempotencyConflict
		}
		return existing, nil
	} else if !errors.Is(findErr, ErrNotFound) {
		return nil, findErr
	}
	current, err := service.repository.GetLatest(ctx, ownerIdentity, id)
	if err != nil {
		return nil, err
	}
	if current.Revision != request.ExpectedRevision || current.Digest != strings.TrimSpace(request.ExpectedDigest) {
		return nil, ErrRevisionConflict
	}
	createdBy := strings.TrimSpace(request.CreatedBy)
	now := utcNow(service.now)
	next := normalizePlan(Plan{
		ID: id, OwnerIdentity: ownerIdentity, Title: request.Title,
		Status: StatusDraft, Revision: current.Revision + 1,
		ParentRevision: current.Revision, ParentDigest: current.Digest,
		IdempotencyKey: key, RequestDigest: requestDigest, Nodes: request.Nodes, Edges: request.Edges,
		Repair: &RepairProvenance{
			Reason: request.Reason, Trigger: request.Trigger,
			PreviousRevision: current.Revision, PreviousDigest: current.Digest,
			CreatedBy: createdBy, CreatedAt: now,
		},
		CreatedBy: createdBy, CreatedAt: now, CanExecute: false,
	})
	if err := validatePlan(next); err != nil {
		return nil, err
	}
	next.Digest, err = computeDigest(next)
	if err != nil {
		return nil, err
	}
	if err := service.repository.CreateRevision(ctx, next, current.Revision); err != nil {
		return nil, err
	}
	return pointerPlan(next), nil
}

func (service *Service) List(ctx context.Context, ownerIdentity string) ([]Plan, error) {
	if err := service.ready(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(ownerIdentity) == "" {
		return nil, fmt.Errorf("owner identity is required")
	}
	return service.repository.ListLatest(ctx, strings.TrimSpace(ownerIdentity))
}

func (service *Service) Get(ctx context.Context, ownerIdentity string, id uuid.UUID, revision uint64) (*Plan, error) {
	if err := service.ready(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(ownerIdentity) == "" || id == uuid.Nil {
		return nil, ErrNotFound
	}
	if revision == 0 {
		return service.repository.GetLatest(ctx, strings.TrimSpace(ownerIdentity), id)
	}
	return service.repository.GetRevision(ctx, strings.TrimSpace(ownerIdentity), id, revision)
}

func (service *Service) ResolveAccepted(ctx context.Context, ownerIdentity string, reference AcceptedRevisionReference) (*AcceptedRevisionBinding, error) {
	if err := service.ready(); err != nil {
		return nil, err
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	reference.Digest = strings.ToLower(strings.TrimSpace(reference.Digest))
	reference.NodeID = strings.TrimSpace(reference.NodeID)
	if ownerIdentity == "" || reference.PlanID == uuid.Nil || reference.Revision == 0 ||
		len(reference.Digest) != 64 || reference.NodeID == "" {
		return nil, ErrReferenceInvalid
	}
	latest, err := service.repository.GetLatest(ctx, ownerIdentity, reference.PlanID)
	if err != nil {
		return nil, err
	}
	if latest.Revision != reference.Revision || latest.Digest != reference.Digest {
		return nil, ErrReferenceStale
	}
	return acceptedRevisionBinding(latest, reference)
}

func (service *Service) ResolveAcceptedRevision(ctx context.Context, ownerIdentity string, reference AcceptedRevisionReference) (*AcceptedRevisionBinding, error) {
	if err := service.ready(); err != nil {
		return nil, err
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	reference.Digest = strings.ToLower(strings.TrimSpace(reference.Digest))
	reference.NodeID = strings.TrimSpace(reference.NodeID)
	if ownerIdentity == "" || reference.PlanID == uuid.Nil || reference.Revision == 0 ||
		len(reference.Digest) != 64 || reference.NodeID == "" {
		return nil, ErrReferenceInvalid
	}
	revision, err := service.repository.GetRevision(ctx, ownerIdentity, reference.PlanID, reference.Revision)
	if err != nil {
		return nil, err
	}
	if revision.Digest != reference.Digest {
		return nil, ErrReferenceStale
	}
	return acceptedRevisionBinding(revision, reference)
}

func acceptedRevisionBinding(plan *Plan, reference AcceptedRevisionReference) (*AcceptedRevisionBinding, error) {
	if plan == nil || plan.ID != reference.PlanID || plan.Revision != reference.Revision || plan.Digest != reference.Digest {
		return nil, ErrReferenceStale
	}
	if plan.Status != StatusAccepted || plan.AcceptedAt == nil {
		return nil, ErrPlanNotAccepted
	}
	if plan.CanExecute {
		return nil, fmt.Errorf("accepted plan violated the non-execution invariant")
	}
	nodes := cloneNodes(plan.Nodes)
	edges := cloneEdges(plan.Edges)
	var selected *Node
	for index := range nodes {
		if nodes[index].ID == reference.NodeID {
			copy := cloneNode(nodes[index])
			selected = &copy
			break
		}
	}
	if selected == nil {
		return nil, ErrReferenceInvalid
	}
	return &AcceptedRevisionBinding{
		PlanID: plan.ID, Revision: plan.Revision, Digest: plan.Digest,
		NodeID: selected.ID, PlanTitle: plan.Title, Node: *selected,
		Nodes: nodes, Edges: edges,
		AcceptedAt: plan.AcceptedAt.UTC(), CanExecute: false,
	}, nil
}

func (service *Service) ready() error {
	if service == nil || service.repository == nil || service.now == nil || service.newID == nil {
		return fmt.Errorf("plan graph service is unavailable")
	}
	return nil
}

func pointerPlan(plan Plan) *Plan {
	copy := clonePlan(plan)
	return &copy
}

package workflow

import (
	"context"
	"fmt"
	"strings"

	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/plangraph"
)

func WithAcceptedPlanResolver(value Service, resolver plangraph.AcceptedRevisionResolver) (Service, error) {
	service, ok := value.(*service)
	if !ok || service == nil {
		return nil, fmt.Errorf("workflow service does not support accepted plan resolution")
	}
	if resolver == nil {
		return nil, fmt.Errorf("accepted plan resolver is required")
	}
	service.acceptedPlanResolver = resolver
	return service, nil
}

func (s *service) resolveAcceptedCoordinationPlan(ownerIdentity string, reference plangraph.AcceptedRevisionReference) (*plangraph.AcceptedRevisionBinding, error) {
	if reference.IsZero() {
		return nil, nil
	}
	if s.acceptedPlanResolver == nil {
		return nil, fmt.Errorf("accepted coordination plan validation is unavailable")
	}
	binding, err := s.acceptedPlanResolver.ResolveAccepted(context.Background(), strings.TrimSpace(ownerIdentity), reference)
	if err != nil {
		return nil, fmt.Errorf("validate accepted coordination plan: %w", err)
	}
	if binding == nil || binding.CanExecute {
		return nil, fmt.Errorf("accepted coordination plan violated the advisory-only invariant")
	}
	return binding, nil
}

func (s *service) resolveWorkflowItemCoordinationPlan(item models.WorkflowItem) (*plangraph.AcceptedRevisionBinding, error) {
	return s.resolveAcceptedCoordinationPlan(item.OwnerIdentity, workflowCoordinationReference(item))
}

func workflowCoordinationReference(item models.WorkflowItem) plangraph.AcceptedRevisionReference {
	reference := plangraph.AcceptedRevisionReference{
		Revision: item.CoordinationPlanRevision,
		Digest:   item.CoordinationPlanDigest,
		NodeID:   item.CoordinationPlanNodeID,
	}
	if item.CoordinationPlanID != nil {
		reference.PlanID = *item.CoordinationPlanID
	}
	return reference
}

func applyWorkflowCoordinationBinding(item *models.WorkflowItem, binding *plangraph.AcceptedRevisionBinding) {
	if item == nil || binding == nil {
		return
	}
	planID := binding.PlanID
	item.CoordinationPlanID = &planID
	item.CoordinationPlanRevision = binding.Revision
	item.CoordinationPlanDigest = binding.Digest
	item.CoordinationPlanNodeID = binding.NodeID
}

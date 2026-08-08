package executionauth

import (
	"context"
	"fmt"

	"automation-hub-backend/internal/frameworkregistry"
)

type frameworkSelectionService interface {
	Selection(context.Context, string, string) (*frameworkregistry.SelectionDecision, error)
}

type registryFrameworkSelectionResolver struct {
	service frameworkSelectionService
}

func NewFrameworkSelectionResolver(
	service frameworkSelectionService,
) (FrameworkSelectionResolver, error) {
	if service == nil {
		return nil, fmt.Errorf("framework selection service is required")
	}
	return &registryFrameworkSelectionResolver{service: service}, nil
}

func (r *registryFrameworkSelectionResolver) ResolveFrameworkSelection(
	ctx context.Context,
	owner string,
	selectionID string,
) (FrameworkSelectionSnapshot, error) {
	decision, err := r.service.Selection(ctx, owner, selectionID)
	if err != nil {
		return FrameworkSelectionSnapshot{}, err
	}
	if decision == nil {
		return FrameworkSelectionSnapshot{}, fmt.Errorf(
			"framework selection service returned no decision",
		)
	}
	return FrameworkSelectionSnapshot{
		SelectionID:              decision.ID,
		TaskPlanID:               decision.TaskPlanID,
		CatalogVersion:           decision.CatalogVersion,
		SelectorAlgorithmVersion: decision.SelectorAlgorithmVersion,
		TaskRiskLevel:            RiskLevel(decision.TaskRiskLevel),
		EffectiveRiskCeiling:     RiskLevel(decision.EffectiveRiskCeiling),
		MaximumAutonomyLevel:     decision.MaximumAutonomyLevel,
		RequiresApproval:         decision.RequiresApproval,
		CatalogDigest:            decision.CatalogDigest,
		PreferenceDigest:         decision.EffectivePreferenceDigest,
		ConstitutionDigest:       decision.ConstitutionDigest,
		OperatingContractDigest:  decision.OperatingContractDigest,
	}, nil
}

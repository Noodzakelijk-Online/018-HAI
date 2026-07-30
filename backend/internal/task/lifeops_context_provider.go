package task

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"automation-hub-backend/internal/frameworkregistry"
	"automation-hub-backend/internal/lifeops"
)

// LifeOpsContextProvider adapts durable owner-scoped observations into the
// framework selector's trusted operating-context contract.
type LifeOpsContextProvider struct {
	service *lifeops.Service
}

func NewLifeOpsContextProvider(service *lifeops.Service) *LifeOpsContextProvider {
	return &LifeOpsContextProvider{service: service}
}

func (p *LifeOpsContextProvider) RecordTaskDomains(
	ownerIdentity string,
	taskPlanID string,
	assignments []frameworkregistry.LifeDomainAssignment,
	selectionID string,
) error {
	if p == nil || p.service == nil {
		return fmt.Errorf("whole-life context provider is unavailable")
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	taskPlanID = strings.TrimSpace(taskPlanID)
	if ownerIdentity == "" || taskPlanID == "" {
		return fmt.Errorf("owner identity and task plan id are required")
	}
	sourceURI := ""
	if strings.TrimSpace(selectionID) != "" {
		sourceURI = "framework-selection://" + strings.TrimSpace(selectionID)
	}
	recorded := 0
	for _, assignment := range assignments {
		domainID := canonicalLifeDomainID(assignment.ID)
		if domainID == "" {
			continue
		}
		evidence := append([]string(nil), assignment.Signals...)
		if strings.TrimSpace(assignment.Need) != "" {
			evidence = append(evidence, assignment.Need)
		}
		if _, err := p.service.LinkEntity(lifeops.LinkEntityRequest{
			OwnerIdentity:      ownerIdentity,
			EntityType:         "task_plan",
			EntityID:           taskPlanID,
			DomainID:           domainID,
			Primary:            assignment.Primary,
			Confidence:         assignment.Confidence,
			SourceLabel:        "framework_registry:deterministic_classification",
			SourceURI:          sourceURI,
			Evidence:           evidence,
			VerificationStatus: "needs_review",
		}); err != nil {
			return err
		}
		recorded++
	}
	if recorded == 0 {
		_, err := p.service.LinkEntity(lifeops.LinkEntityRequest{
			OwnerIdentity:      ownerIdentity,
			EntityType:         "task_plan",
			EntityID:           taskPlanID,
			DomainID:           lifeops.DomainPersonalProductivity,
			Primary:            true,
			Confidence:         0.25,
			SourceLabel:        "framework_registry:unclassified_fallback",
			SourceURI:          sourceURI,
			Evidence:           []string{"task requires classification review"},
			VerificationStatus: "needs_review",
		})
		return err
	}
	return nil
}

func (p *LifeOpsContextProvider) LatestNeeds(ownerIdentity string, at time.Time) ([]frameworkregistry.NeedStateAssessment, error) {
	if p == nil || p.service == nil {
		return nil, fmt.Errorf("whole-life context provider is unavailable")
	}
	observations, err := p.service.Needs(ownerIdentity, "", 500)
	if err != nil {
		return nil, err
	}
	seen := make(map[lifeops.DomainID]struct{})
	result := make([]frameworkregistry.NeedStateAssessment, 0)
	for _, observation := range observations {
		if _, ok := seen[observation.DomainID]; ok {
			continue
		}
		if observation.ExpiresAt != nil && !observation.ExpiresAt.After(at.UTC()) {
			continue
		}
		seen[observation.DomainID] = struct{}{}
		evidence := append([]string(nil), observation.Evidence...)
		if strings.TrimSpace(observation.SourceURI) != "" {
			evidence = append(evidence, observation.SourceURI)
		}
		source := strings.TrimSpace(observation.SourceLabel)
		if source == "" {
			source = "lifeops"
		}
		result = append(result, frameworkregistry.NeedStateAssessment{
			ID:          observation.DomainID.String() + ":" + observation.NeedLevel,
			DomainID:    observation.DomainID.String(),
			Level:       observation.NeedLevel,
			State:       observation.State,
			Priority:    observation.Priority,
			Confidence:  observation.Confidence,
			Evidence:    evidence,
			Source:      source,
			NeedsReview: observation.NeedsReview,
		})
	}
	return result, nil
}

func canonicalLifeDomainID(value string) lifeops.DomainID {
	domainID := lifeops.DomainID(strings.TrimSpace(value))
	if lifeops.IsCanonicalDomain(domainID) {
		return domainID
	}
	return ""
}

func (p *LifeOpsContextProvider) LatestCapacity(ownerIdentity string, at time.Time) (*frameworkregistry.CapacitySnapshot, error) {
	if p == nil || p.service == nil {
		return nil, fmt.Errorf("whole-life context provider is unavailable")
	}
	snapshot, err := p.service.LatestCapacity(ownerIdentity)
	if errors.Is(err, lifeops.ErrNotFound) {
		return &frameworkregistry.CapacitySnapshot{
			Status:            lifeops.CapacityUnknown,
			PlanningStepLimit: 1,
			Constraints: []string{
				"no owner-confirmed capacity snapshot is available",
				"request human review before relying on capacity-sensitive planning",
			},
			SourceLabel: "lifeops:no_observation",
			Confidence:  0,
			Fresh:       false,
			NeedsReview: true,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	fresh := snapshot.Fresh && at.UTC().Sub(snapshot.CapturedAt.UTC()) <= 24*time.Hour
	constraints := append([]string(nil), snapshot.Constraints...)
	if !fresh {
		constraints = append(constraints, "capacity snapshot is stale")
	}
	capturedAt := snapshot.CapturedAt.UTC()
	return &frameworkregistry.CapacitySnapshot{
		Status:               snapshot.Status,
		Energy:               snapshot.Signals.Energy,
		Attention:            snapshot.Signals.AttentionQuality,
		TimeAvailableMinutes: snapshot.TimeAvailableMinutes,
		ConcurrentWorkLimit:  snapshot.ConcurrentWorkLimit,
		CurrentLoad:          snapshot.CurrentLoad,
		PlanningStepLimit:    snapshot.PlanningStepLimit,
		Constraints:          constraints,
		SourceURI:            snapshot.SourceURI,
		SourceLabel:          snapshot.SourceLabel,
		CapturedAt:           &capturedAt,
		Confidence:           snapshot.Confidence,
		Fresh:                fresh,
		NeedsReview:          snapshot.NeedsReview || !fresh,
	}, nil
}

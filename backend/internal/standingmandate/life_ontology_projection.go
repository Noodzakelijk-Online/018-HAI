package standingmandate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"automation-hub-backend/internal/lifeontology"
	"automation-hub-backend/internal/safety"
)

const maximumMandateProjectionLinks = 16

type LifeOntologyProjector interface {
	ProjectOperationalRecord(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error)
}

func (s *Service) projectMandate(ctx context.Context, mandate *StandingMandate) {
	if s == nil || s.lifeGraph == nil || mandate == nil {
		return
	}
	request, err := mandateProjectionRequest(*mandate)
	if err != nil {
		mandate.LifeGraphWarning = mandateProjectionWarning(err)
		return
	}
	result, err := s.lifeGraph.ProjectOperationalRecord(ctx, request)
	if err != nil {
		mandate.LifeGraphWarning = mandateProjectionWarning(err)
		return
	}
	if !result.AdvisoryOnly || result.CanExecute || result.GrantsAuthority {
		mandate.LifeGraphWarning = "whole-life graph projection crossed its advisory-only authority boundary"
		return
	}
	mandate.LifeGraph = &result
}

func mandateProjectionRequest(mandate StandingMandate) (lifeontology.OperationalProjectionRequest, error) {
	mandateDigest, err := digest(normalizedMandate(mandate))
	if err != nil {
		return lifeontology.OperationalProjectionRequest{}, err
	}
	links := make([]lifeontology.OperationalLinkRequest, 0, maximumMandateProjectionLinks)
	seenProjects := map[string]struct{}{}
	for _, scope := range mandate.Scopes {
		for _, project := range scope.Projects {
			project = strings.TrimSpace(project)
			if project == "" {
				continue
			}
			if _, ok := seenProjects[project]; ok {
				continue
			}
			seenProjects[project] = struct{}{}
			links = append(links, lifeontology.OperationalLinkRequest{
				Type: lifeontology.EntityProject, RecordID: project, Name: "Project " + project,
				Relation: lifeontology.RelationBelongsToProject, Status: lifeontology.StatusActive,
			})
			if len(links) >= maximumMandateProjectionLinks {
				break
			}
		}
		if len(links) >= maximumMandateProjectionLinks {
			break
		}
	}
	return lifeontology.OperationalProjectionRequest{
		OwnerIdentity: mandate.OwnerIdentity, Type: lifeontology.EntityOutcome,
		RecordID: fmt.Sprintf("standing-mandate/%s/revision/%d", mandate.ID, mandate.Revision),
		Domain:   mandateProjectionDomain(mandate.Scopes), Name: "Standing mandate: " + mandate.Name,
		Summary: mandate.Purpose, Status: mandateProjectionStatus(mandate.Status),
		Priority: mandateProjectionPriority(mandate.Status), ObservedAt: mandate.UpdatedAt.UTC(),
		Confidence: 1, VerificationStatus: mandateProjectionVerification(mandate.Status),
		Attributes: map[string]string{
			"record_kind": "standing_mandate_revision", "mandate_id": mandate.ID.String(),
			"revision": strconv.FormatUint(mandate.Revision, 10), "status": string(mandate.Status),
			"autonomy_ceiling":       strconv.Itoa(mandate.AutonomyCeiling),
			"grants_graph_authority": "false",
		},
		Provenance:  mandateProjectionProvenance(mandate, mandateDigest),
		Sensitivity: lifeontology.SensitivityRestricted, LocalOnly: true, Links: links,
	}, nil
}

func mandateProjectionDomain(scopes []Scope) lifeontology.Domain {
	valid := map[string]lifeontology.Domain{
		string(lifeontology.DomainSafetySecurity):  lifeontology.DomainSafetySecurity,
		string(lifeontology.DomainHealthWellbeing): lifeontology.DomainHealthWellbeing,
		string(lifeontology.DomainRelationships):   lifeontology.DomainRelationships,
		string(lifeontology.DomainHousingAssets):   lifeontology.DomainHousingAssets,
		string(lifeontology.DomainFinancial):       lifeontology.DomainFinancial,
		string(lifeontology.DomainWorkVenture):     lifeontology.DomainWorkVenture,
		string(lifeontology.DomainLearningGrowth):  lifeontology.DomainLearningGrowth,
		string(lifeontology.DomainMeaningValues):   lifeontology.DomainMeaningValues,
		string(lifeontology.DomainCommunityCivic):  lifeontology.DomainCommunityCivic,
		string(lifeontology.DomainLegalGovernment): lifeontology.DomainLegalGovernment,
		string(lifeontology.DomainPersonalAdmin):   lifeontology.DomainPersonalAdmin,
	}
	for _, scope := range scopes {
		for _, value := range scope.Domains {
			if domain, ok := valid[strings.ToLower(strings.TrimSpace(value))]; ok {
				return domain
			}
		}
	}
	return lifeontology.DomainPersonalAdmin
}

func mandateProjectionStatus(value Status) lifeontology.LifecycleStatus {
	switch value {
	case StatusActive:
		return lifeontology.StatusActive
	case StatusRevoked:
		return lifeontology.StatusArchived
	default:
		return lifeontology.StatusWaiting
	}
}

func mandateProjectionPriority(value Status) int {
	if value == StatusActive {
		return 85
	}
	if value == StatusDraft {
		return 70
	}
	return 35
}

func mandateProjectionVerification(value Status) lifeontology.VerificationStatus {
	if value == StatusActive || value == StatusRevoked {
		return lifeontology.VerificationHumanApproved
	}
	return lifeontology.VerificationSchemaValidated
}

func mandateProjectionProvenance(mandate StandingMandate, digestValue string) []lifeontology.Provenance {
	values := make([]lifeontology.Provenance, 0, max(1, len(mandate.SourceReferences)))
	for index, source := range mandate.SourceReferences {
		values = append(values, lifeontology.Provenance{
			ReferenceID: fmt.Sprintf("mandate-source-%d", index+1), URI: source,
			ContentDigest: digestValue, Authority: "owner_standing_mandate",
			CapturedAt: mandate.UpdatedAt.UTC(), LocalOnly: true,
		})
	}
	if len(values) == 0 {
		values = append(values, lifeontology.Provenance{
			ReferenceID: mandate.ID.String(), URI: "hai://standing-mandates/" + mandate.ID.String(),
			ContentDigest: digestValue, Authority: "owner_standing_mandate",
			CapturedAt: mandate.UpdatedAt.UTC(), LocalOnly: true,
		})
	}
	return values
}

func mandateProjectionWarning(err error) string {
	message := strings.Join(strings.Fields(safety.RedactSecrets(err.Error())), " ")
	if len([]rune(message)) > 500 {
		message = string([]rune(message)[:500])
	}
	return message
}

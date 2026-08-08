package executionauth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"automation-hub-backend/internal/lifeontology"
	"automation-hub-backend/internal/safety"
)

const maximumAuthorizationProjectionLinks = 16

// LifeOntologyProjector is append-only and advisory. Execution authorization
// never reads graph presence as approval or execution authority.
type LifeOntologyProjector interface {
	ProjectOperationalRecord(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error)
}

func (s *Service) WithLifeOntologyProjection(projector LifeOntologyProjector) (*Service, error) {
	if s == nil {
		return nil, fmt.Errorf("execution authorization service is required")
	}
	if projector == nil {
		return nil, fmt.Errorf("life ontology projector is required")
	}
	s.lifeGraph = projector
	return s, nil
}

func (s *Service) projectReceipt(ctx context.Context, receipt *Receipt) {
	if s == nil || s.lifeGraph == nil || receipt == nil {
		return
	}
	result, err := s.lifeGraph.ProjectOperationalRecord(ctx, receiptProjectionRequest(*receipt))
	if err != nil {
		receipt.LifeGraphProjectionWarning = authorizationProjectionWarning(err)
		return
	}
	if !result.AdvisoryOnly || result.CanExecute || result.GrantsAuthority {
		receipt.LifeGraphProjectionWarning = "whole-life graph projection crossed its advisory-only authority boundary"
		return
	}
	receipt.LifeGraphProjection = &result
}

func receiptProjectionRequest(receipt Receipt) lifeontology.OperationalProjectionRequest {
	domain := authorizationProjectionDomain(receipt.Domain, receipt.ProjectKey, receipt.ResourceType)
	recordID := receipt.ID.String()
	links := make([]lifeontology.OperationalLinkRequest, 0, 5)
	if taskID := strings.TrimSpace(receipt.TaskID); taskID != "" {
		links = append(links, lifeontology.OperationalLinkRequest{
			Type: lifeontology.EntityTask, RecordID: taskID,
			Name:     "Task " + boundedAuthorizationText(taskID, 220),
			Summary:  "Task governed by this immutable authorization decision.",
			Relation: lifeontology.RelationSupports, Status: lifeontology.StatusActive,
		})
	}
	if projectKey := strings.TrimSpace(receipt.ProjectKey); projectKey != "" {
		links = append(links, lifeontology.OperationalLinkRequest{
			Type: lifeontology.EntityProject, RecordID: projectKey,
			Name:     "Project " + boundedAuthorizationText(projectKey, 220),
			Relation: lifeontology.RelationBelongsToProject, Status: lifeontology.StatusActive,
		})
	}
	if sourceID := strings.TrimSpace(receipt.ApprovalSourceID); sourceID != "" &&
		strings.TrimSpace(receipt.Evidence.Approval.DecisionDigest) != "" {
		links = append(links, lifeontology.OperationalLinkRequest{
			Type: lifeontology.EntityOutcome, RecordID: "approval/" + sourceID,
			Name:     "Verified case approval",
			Summary:  "Server-resolved approval evidence bound to this exact authorization request.",
			Relation: lifeontology.RelationSupports, Direction: "related_to_primary",
			Status: lifeontology.StatusCompleted,
			Attributes: map[string]string{
				"record_kind": "approval_evidence",
				"source_id":   boundedAuthorizationText(sourceID, 256),
				"decision_id": boundedAuthorizationText(receipt.Evidence.Approval.DecisionID, 256),
				"approved_at": formatAuthorizationTime(receipt.Evidence.Approval.ApprovedAt),
				"expires_at":  formatAuthorizationTime(receipt.Evidence.Approval.ExpiresAt),
			},
		})
	}
	if receipt.Stage == StageCommitment && len(links) < maximumAuthorizationProjectionLinks {
		links = append(links, lifeontology.OperationalLinkRequest{
			Type: lifeontology.EntityCommitment, RecordID: recordID + "/commitment",
			Name:     boundedAuthorizationText("Commitment: "+receipt.Action, 256),
			Summary:  "Proposed external commitment governed by this authorization receipt.",
			Relation: lifeontology.RelationSupports,
			Status:   authorizationCommitmentStatus(receipt.Outcome),
			Attributes: map[string]string{
				"authorization_outcome": string(receipt.Outcome),
				"resource_type":         boundedAuthorizationText(receipt.ResourceType, 128),
				"resource_id":           boundedAuthorizationText(receipt.ResourceID, 256),
			},
		})
	}
	if receipt.EstimatedCostEUR > 0 && len(links) < maximumAuthorizationProjectionLinks {
		links = append(links, lifeontology.OperationalLinkRequest{
			Type: lifeontology.EntityCost, RecordID: recordID + "/estimated-cost",
			Name:     fmt.Sprintf("Estimated execution cost EUR %.2f", receipt.EstimatedCostEUR),
			Summary:  "Estimated cost supplied to the authorization policy boundary; not an incurred or paid amount.",
			Relation: lifeontology.RelationIncursCost, Status: lifeontology.StatusOpen,
			Attributes: map[string]string{
				"amount_eur": strconv.FormatFloat(receipt.EstimatedCostEUR, 'f', 6, 64),
				"cost_kind":  "estimate", "incurred": "false", "paid": "false",
			},
		})
	}

	return lifeontology.OperationalProjectionRequest{
		OwnerIdentity: receipt.OwnerIdentity,
		Type:          lifeontology.EntityOutcome, RecordID: recordID, Domain: domain,
		Name:       boundedAuthorizationText("Authorization decision: "+receipt.Action, 256),
		Summary:    boundedAuthorizationText(fmt.Sprintf("%s: %s", receipt.Outcome, receipt.Reason), 1800),
		Status:     authorizationProjectionStatus(receipt.Outcome),
		Priority:   authorizationProjectionPriority(receipt.Risk, receipt.Outcome),
		ObservedAt: receipt.EvaluatedAt.UTC(), Confidence: 1,
		VerificationStatus: authorizationProjectionVerification(receipt),
		Attributes: map[string]string{
			"record_kind": "execution_authorization_receipt",
			"outcome":     string(receipt.Outcome), "stage": string(receipt.Stage),
			"risk": string(receipt.Risk), "resource_type": boundedAuthorizationText(receipt.ResourceType, 128),
			"resource_id":        boundedAuthorizationText(receipt.ResourceID, 256),
			"reversible":         strconv.FormatBool(receipt.Reversible),
			"estimated_cost_eur": strconv.FormatFloat(receipt.EstimatedCostEUR, 'f', 6, 64),
		},
		Provenance: []lifeontology.Provenance{{
			ReferenceID: recordID, URI: "hai://execution-authorizations/" + recordID,
			ContentDigest: receipt.DecisionDigest, Authority: "hai_execution_authorization_ledger",
			CapturedAt: receipt.EvaluatedAt.UTC(), LocalOnly: true,
		}},
		Sensitivity: lifeontology.SensitivityRestricted, LocalOnly: true, Links: links,
	}
}

func authorizationProjectionDomain(values ...string) lifeontology.Domain {
	value := strings.ToLower(strings.Join(values, " "))
	for _, domain := range []lifeontology.Domain{
		lifeontology.DomainSafetySecurity, lifeontology.DomainHealthWellbeing,
		lifeontology.DomainRelationships, lifeontology.DomainHousingAssets,
		lifeontology.DomainFinancial, lifeontology.DomainWorkVenture,
		lifeontology.DomainLearningGrowth, lifeontology.DomainMeaningValues,
		lifeontology.DomainCommunityCivic, lifeontology.DomainLegalGovernment,
		lifeontology.DomainPersonalAdmin,
	} {
		if strings.Contains(value, string(domain)) {
			return domain
		}
	}
	switch {
	case strings.Contains(value, "legal"), strings.Contains(value, "government"), strings.Contains(value, "lawyer"):
		return lifeontology.DomainLegalGovernment
	case strings.Contains(value, "invoice"), strings.Contains(value, "finance"), strings.Contains(value, "payment"):
		return lifeontology.DomainFinancial
	case strings.Contains(value, "health"), strings.Contains(value, "medical"):
		return lifeontology.DomainHealthWellbeing
	case strings.Contains(value, "work"), strings.Contains(value, "github"), strings.Contains(value, "software"):
		return lifeontology.DomainWorkVenture
	default:
		return lifeontology.DomainPersonalAdmin
	}
}

func authorizationProjectionStatus(outcome Outcome) lifeontology.LifecycleStatus {
	switch outcome {
	case OutcomeAuthorized:
		return lifeontology.StatusCompleted
	case OutcomeRequiresApproval:
		return lifeontology.StatusWaiting
	default:
		return lifeontology.StatusOpen
	}
}

func authorizationCommitmentStatus(outcome Outcome) lifeontology.LifecycleStatus {
	if outcome == OutcomeAuthorized {
		return lifeontology.StatusActive
	}
	if outcome == OutcomeRequiresApproval {
		return lifeontology.StatusWaiting
	}
	return lifeontology.StatusOpen
}

func authorizationProjectionPriority(risk RiskLevel, outcome Outcome) int {
	if outcome == OutcomeRequiresApproval {
		return 95
	}
	switch risk {
	case RiskCritical:
		return 100
	case RiskHigh:
		return 90
	case RiskMedium:
		return 65
	default:
		return 40
	}
}

func authorizationProjectionVerification(receipt Receipt) lifeontology.VerificationStatus {
	if receipt.Outcome == OutcomeAuthorized && strings.TrimSpace(receipt.Evidence.Approval.DecisionDigest) != "" {
		return lifeontology.VerificationHumanApproved
	}
	return lifeontology.VerificationSchemaValidated
}

func authorizationProjectionWarning(err error) string {
	return boundedAuthorizationText(safety.RedactSecrets(err.Error()), 500)
}

func boundedAuthorizationText(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(safety.RedactSecrets(value))), " ")
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func formatAuthorizationTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

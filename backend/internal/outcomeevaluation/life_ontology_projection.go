package outcomeevaluation

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"automation-hub-backend/internal/lifeontology"
	"automation-hub-backend/internal/safety"
)

const maximumOutcomeProjectionLinks = 16

type LifeOntologyProjector interface {
	ProjectOperationalRecord(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error)
}

// WithLifeOntologyProjection attaches the shared advisory life graph to the
// existing outcome service. It does not create a second outcome ledger.
func WithLifeOntologyProjection(service *Service, projector LifeOntologyProjector) (*Service, error) {
	if service == nil {
		return nil, fmt.Errorf("outcome evaluation service is required")
	}
	if projector == nil {
		return nil, fmt.Errorf("life ontology projector is required")
	}
	service.lifeGraph = projector
	return service, nil
}

func (s *Service) projectOutcomeRevision(ctx context.Context, record *OutcomeRevision) {
	if s == nil || s.lifeGraph == nil || record == nil {
		return
	}
	result, err := s.lifeGraph.ProjectOperationalRecord(ctx, outcomeRevisionProjection(*record))
	if err != nil {
		record.LifeGraphProjectionWarning = projectionWarning(err)
		return
	}
	if err := validateProjectionBoundary(result); err != nil {
		record.LifeGraphProjectionWarning = projectionWarning(err)
		return
	}
	record.LifeGraphProjection = &result
}

func (s *Service) projectEvaluationRecord(ctx context.Context, definition OutcomeRevision, record *EvaluationRecord) {
	if s == nil || s.lifeGraph == nil || record == nil {
		return
	}
	// Repair or create the exact immutable definition node first. This prevents
	// an evaluation from materializing a sparse placeholder when a definition
	// predates life-graph projection or an earlier projection failed.
	s.projectOutcomeRevision(ctx, &definition)
	if definition.LifeGraphProjectionWarning != "" {
		record.LifeGraphProjectionWarning = "outcome definition projection needs repair: " + definition.LifeGraphProjectionWarning
		return
	}
	result, err := s.lifeGraph.ProjectOperationalRecord(ctx, evaluationRecordProjection(definition, *record))
	if err != nil {
		record.LifeGraphProjectionWarning = projectionWarning(err)
		return
	}
	if err := validateProjectionBoundary(result); err != nil {
		record.LifeGraphProjectionWarning = projectionWarning(err)
		return
	}
	record.LifeGraphProjection = &result
}

func outcomeRevisionProjection(record OutcomeRevision) lifeontology.OperationalProjectionRequest {
	outcome := record.Outcome
	domain, domainAssignment := outcomeProjectionDomain(outcome)
	recordID := outcomeRevisionGraphID(outcome.Scope.WorkspaceID, outcome.ID, record.Revision)
	dueAt := outcome.Window.End.UTC()
	links := []lifeontology.OperationalLinkRequest{workspaceProjectionLink(outcome.Scope.WorkspaceID)}
	links = append(links, outcomeSourceLinks(outcome, maximumOutcomeProjectionLinks-len(links))...)
	return lifeontology.OperationalProjectionRequest{
		OwnerIdentity: outcome.Scope.OwnerID,
		Type:          lifeontology.EntityOutcome,
		RecordID:      recordID,
		Domain:        domain,
		Name:          boundedProjectionText(outcome.Statement, 256),
		Summary: boundedProjectionText(fmt.Sprintf(
			"Intended outcome revision %d with %d indicators in the %s life domain.",
			record.Revision, len(outcome.Indicators), domain,
		), 1800),
		Status:             lifeontology.StatusActive,
		Priority:           50,
		DueAt:              &dueAt,
		ObservedAt:         record.RecordedAt.UTC(),
		Confidence:         0.8,
		VerificationStatus: lifeontology.VerificationSchemaValidated,
		Attributes: map[string]string{
			"outcome_id": outcome.ID, "workspace_id": outcome.Scope.WorkspaceID,
			"revision":          strconv.FormatInt(record.Revision, 10),
			"indicator_count":   strconv.Itoa(len(outcome.Indicators)),
			"window_start":      outcome.Window.Start.UTC().Format(time.RFC3339Nano),
			"window_end":        outcome.Window.End.UTC().Format(time.RFC3339Nano),
			"domain_assignment": domainAssignment,
		},
		Provenance: []lifeontology.Provenance{{
			ReferenceID:   recordID,
			URI:           fmt.Sprintf("hai://outcomes/%s/%s/revisions/%d", outcome.Scope.WorkspaceID, outcome.ID, record.Revision),
			ContentDigest: record.AuditDigest, Authority: "hai_outcome_ledger",
			CapturedAt: record.RecordedAt.UTC(), LocalOnly: true,
		}},
		Sensitivity: lifeontology.SensitivityInternal,
		LocalOnly:   true,
		Links:       links,
	}
}

func evaluationRecordProjection(definition OutcomeRevision, record EvaluationRecord) lifeontology.OperationalProjectionRequest {
	evaluation := record.Evaluation
	domain, domainAssignment := outcomeProjectionDomain(definition.Outcome)
	recordID := evaluationGraphID(evaluation.Scope.WorkspaceID, evaluation.OutcomeID, evaluation.ID)
	definitionID := outcomeRevisionGraphID(evaluation.Scope.WorkspaceID, evaluation.OutcomeID, record.OutcomeRevision)
	links := []lifeontology.OperationalLinkRequest{
		workspaceProjectionLink(evaluation.Scope.WorkspaceID),
		{
			Type: lifeontology.EntityOutcome, RecordID: definitionID,
			Name:       boundedProjectionText(definition.Outcome.Statement, 256),
			Summary:    "Immutable intended outcome revision evaluated by this record.",
			Relation:   lifeontology.RelationSupports,
			Status:     lifeontology.StatusActive,
			Attributes: map[string]string{"outcome_revision": strconv.FormatInt(record.OutcomeRevision, 10)},
		},
	}
	return lifeontology.OperationalProjectionRequest{
		OwnerIdentity: evaluation.Scope.OwnerID,
		Type:          lifeontology.EntityOutcome,
		RecordID:      recordID,
		Domain:        domain,
		Name:          boundedProjectionText("Outcome evaluation: "+definition.Outcome.Statement, 256),
		Summary: boundedProjectionText(fmt.Sprintf(
			"State %s at %s. %s", evaluation.State, evaluation.AsOf.UTC().Format(time.RFC3339),
			strings.Join(evaluation.ReviewReasons, " "),
		), 1800),
		Status:             lifeontology.StatusCompleted,
		Priority:           evaluationProjectionPriority(evaluation.State),
		ObservedAt:         record.RecordedAt.UTC(),
		Confidence:         evaluationProjectionConfidence(evaluation.State),
		VerificationStatus: evaluationProjectionVerification(evaluation.State),
		Attributes: map[string]string{
			"outcome_id": evaluation.OutcomeID, "workspace_id": evaluation.Scope.WorkspaceID,
			"outcome_revision": strconv.FormatInt(record.OutcomeRevision, 10),
			"evaluation_id":    evaluation.ID, "state": string(evaluation.State),
			"review_required":      strconv.FormatBool(evaluation.ReviewRequired),
			"indicator_count":      strconv.Itoa(len(evaluation.Indicators)),
			"recommendation_count": strconv.Itoa(len(evaluation.Recommendations)),
			"domain_assignment":    domainAssignment,
		},
		Provenance: []lifeontology.Provenance{{
			ReferenceID:   recordID,
			URI:           fmt.Sprintf("hai://outcomes/%s/%s/evaluations/%s", evaluation.Scope.WorkspaceID, evaluation.OutcomeID, evaluation.ID),
			ContentDigest: record.RecordDigest, Authority: "hai_outcome_evaluation_ledger",
			CapturedAt: record.RecordedAt.UTC(), LocalOnly: true,
		}},
		Sensitivity: lifeontology.SensitivityInternal,
		LocalOnly:   true,
		Links:       links,
	}
}

func outcomeProjectionDomain(outcome IntendedOutcome) (lifeontology.Domain, string) {
	if lifeontology.IsValidDomain(outcome.LifeDomain) {
		return outcome.LifeDomain, "explicit_outcome_definition"
	}
	// Existing revisions created before life-domain authoring remain readable
	// and projectable, but the fallback is explicitly review-marked.
	return lifeontology.DomainPersonalAdmin, "legacy_default_needs_review"
}

func workspaceProjectionLink(workspaceID string) lifeontology.OperationalLinkRequest {
	return lifeontology.OperationalLinkRequest{
		Type: lifeontology.EntityProject, RecordID: workspaceID,
		Name:     boundedProjectionText("Workspace "+workspaceID, 256),
		Summary:  "Authenticated workspace containing this outcome record.",
		Relation: lifeontology.RelationBelongsToProject,
		Status:   lifeontology.StatusActive,
	}
}

func outcomeSourceLinks(outcome IntendedOutcome, limit int) []lifeontology.OperationalLinkRequest {
	if limit <= 0 {
		return nil
	}
	seen := make(map[string]struct{})
	result := make([]lifeontology.OperationalLinkRequest, 0, limit)
	for _, indicator := range outcome.Indicators {
		for _, source := range indicator.Baseline.Sources {
			id := strings.TrimSpace(source.ID)
			if id == "" || strings.TrimSpace(source.URI) == "" {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, lifeontology.OperationalLinkRequest{
				Type: lifeontology.EntitySource, RecordID: id,
				Name:        boundedProjectionText("Outcome evidence source "+id, 256),
				Summary:     boundedProjectionText(safety.RedactURL(source.URI), 1800),
				Relation:    lifeontology.RelationDerivedFrom,
				Status:      lifeontology.StatusActive,
				ExternalKey: &lifeontology.ExternalKey{Namespace: "outcome/source", Value: id},
				Attributes:  map[string]string{"source_status": string(source.Status)},
			})
			if len(result) == limit {
				return result
			}
		}
	}
	return result
}

func outcomeRevisionGraphID(workspaceID, outcomeID string, revision int64) string {
	return strings.TrimSpace(workspaceID) + "/" + strings.TrimSpace(outcomeID) + "/revision/" + strconv.FormatInt(revision, 10)
}

func evaluationGraphID(workspaceID, outcomeID, evaluationID string) string {
	return strings.TrimSpace(workspaceID) + "/" + strings.TrimSpace(outcomeID) + "/evaluation/" + strings.TrimSpace(evaluationID)
}

func evaluationProjectionPriority(state OutcomeState) int {
	switch state {
	case OutcomeRegression, OutcomeReviewRequired:
		return 90
	case OutcomeInsufficientEvidence:
		return 70
	case OutcomeOnTrack:
		return 50
	case OutcomeAchieved:
		return 30
	default:
		return 60
	}
}

func evaluationProjectionConfidence(state OutcomeState) float64 {
	switch state {
	case OutcomeAchieved, OutcomeOnTrack:
		return 0.8
	case OutcomeRegression, OutcomeReviewRequired:
		return 0.7
	default:
		return 0.5
	}
}

func evaluationProjectionVerification(state OutcomeState) lifeontology.VerificationStatus {
	switch state {
	case OutcomeAchieved, OutcomeOnTrack:
		return lifeontology.VerificationSourceSupported
	case OutcomeRegression, OutcomeReviewRequired:
		return lifeontology.VerificationNeedsReview
	default:
		return lifeontology.VerificationUncertain
	}
}

func validateProjectionBoundary(result lifeontology.OperationalProjectionResult) error {
	if !result.AdvisoryOnly || result.CanExecute || result.GrantsAuthority {
		return fmt.Errorf("whole-life graph projection crossed its advisory-only authority boundary")
	}
	return nil
}

func projectionWarning(err error) string {
	return boundedProjectionText(safety.RedactSecrets(err.Error()), 500)
}

func boundedProjectionText(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(safety.RedactSecrets(value))), " ")
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

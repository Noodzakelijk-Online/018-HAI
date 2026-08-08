package source

import (
	"automation-hub-backend/internal/lifeontology"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/safety"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

// LifeOntologyProjector is deliberately narrower than the complete ontology
// service. Connected-source ingestion may append advisory provenance records,
// but it cannot query private context or acquire execution authority.
type LifeOntologyProjector interface {
	ProjectOperationalRecord(context.Context, lifeontology.OperationalProjectionRequest) (lifeontology.OperationalProjectionResult, error)
}

// LifeGraphProjectionOutcome is returned with a sync so operators can verify
// which durable extraction reached the owner-scoped operational graph.
type LifeGraphProjectionOutcome struct {
	ExtractionID    string   `json:"extractionId"`
	DocumentID      string   `json:"documentId"`
	LinkedEntityIDs []string `json:"linkedEntityIds,omitempty"`
	RelationIDs     []string `json:"relationIds,omitempty"`
	AlreadyExisted  bool     `json:"alreadyExisted"`
	AdvisoryOnly    bool     `json:"advisoryOnly"`
	CanExecute      bool     `json:"canExecute"`
	GrantsAuthority bool     `json:"grantsAuthority"`
}

type lifeOntologyProjectionConfigurer interface {
	withLifeOntologyProjection(LifeOntologyProjector) Service
}

// WithLifeOntologyProjection adds immutable graph projection to an existing
// source service. It configures the underlying service in place so consumers
// that already hold the source service, such as verification, observe the same
// behavior rather than a parallel wrapper.
func WithLifeOntologyProjection(base Service, projector LifeOntologyProjector) (Service, error) {
	if base == nil {
		return nil, fmt.Errorf("connected-source service is required")
	}
	if projector == nil {
		return nil, fmt.Errorf("life ontology projector is required")
	}
	configurer, ok := base.(lifeOntologyProjectionConfigurer)
	if !ok {
		return nil, fmt.Errorf("connected-source service does not support life ontology projection")
	}
	return configurer.withLifeOntologyProjection(projector), nil
}

func (s *service) withLifeOntologyProjection(projector LifeOntologyProjector) Service {
	if s != nil {
		s.lifeOntologyProjector = projector
	}
	return s
}

func (s *service) projectExtractionToLifeGraph(
	ctx context.Context,
	source *models.ConnectedSource,
	extraction *models.SourceExtraction,
) (*LifeGraphProjectionOutcome, error) {
	if s == nil || s.lifeOntologyProjector == nil || source == nil || extraction == nil {
		return nil, nil
	}
	owner := strings.TrimSpace(source.OwnerIdentity)
	if owner == "" {
		// Legacy ownerless rows remain readable, but are never copied into a
		// private owner graph because ownership cannot be inferred safely.
		return nil, nil
	}

	// CreatedAt is stable across cached re-indexing. Content corrections still
	// produce a new immutable entity because the summary and digest change.
	observedAt := extraction.CreatedAt.UTC()
	if observedAt.IsZero() {
		observedAt = extraction.UpdatedAt.UTC()
	}
	if observedAt.IsZero() && extraction.LastIndexedAt != nil {
		observedAt = extraction.LastIndexedAt.UTC()
	}
	if observedAt.IsZero() {
		return nil, fmt.Errorf("extraction %s has no durable observation timestamp", extraction.ID)
	}

	verificationStatus := lifeontology.VerificationSourceSupported
	confidence := 0.82
	if extraction.Uncertain {
		verificationStatus = lifeontology.VerificationNeedsReview
		confidence = 0.45
	}
	sensitivity := lifeontology.SensitivityInternal
	if extraction.Sensitive {
		sensitivity = lifeontology.SensitivitySensitive
	}
	status := lifeontology.StatusActive
	if extraction.Archived {
		status = lifeontology.StatusArchived
	}

	digest := sourceProjectionDigest(source, extraction)
	provenance := []lifeontology.Provenance{{
		ReferenceID:   extraction.RawItemID.String(),
		URI:           safety.RedactURL(extraction.SourceURI),
		ContentDigest: digest,
		Authority:     compact(source.ConnectorKey, 80),
		CapturedAt:    observedAt,
		LocalOnly:     source.LocalOnly,
	}}
	links := []lifeontology.OperationalLinkRequest{{
		Type:       lifeontology.EntitySource,
		RecordID:   source.ID.String(),
		Name:       compact(firstNonEmpty(source.Name, source.ConnectorKey, "Connected source"), 256),
		Summary:    compact("Registered connected source using the "+source.ConnectorKey+" connector.", 600),
		Relation:   lifeontology.RelationDerivedFrom,
		Status:     sourceLifecycleStatus(source),
		Attributes: map[string]string{"connector_key": compact(source.ConnectorKey, 80), "category": compact(source.Category, 80)},
	}}
	if projectKey := strings.TrimSpace(extraction.ProjectKey); projectKey != "" {
		links = append(links, lifeontology.OperationalLinkRequest{
			Type:       lifeontology.EntityProject,
			RecordID:   projectKey,
			Name:       compact(projectKey, 256),
			Summary:    "Project linked by connected-source ingestion.",
			Relation:   lifeontology.RelationBelongsToProject,
			Status:     lifeontology.StatusActive,
			Attributes: map[string]string{"project_key": compact(projectKey, 256)},
		})
	}
	links = append(links, sourceContactCandidateLinks(source, extraction, 16-len(links))...)

	result, err := s.lifeOntologyProjector.ProjectOperationalRecord(ctx, lifeontology.OperationalProjectionRequest{
		OwnerIdentity:      owner,
		Type:               lifeontology.EntityDocument,
		RecordID:           extraction.ID.String(),
		Domain:             sourceProjectionDomain(source, extraction),
		Name:               compact(safety.RedactSecrets(firstNonEmpty(extraction.SourceLabel, extraction.ContentType, "Source extraction")), 256),
		Summary:            compact(safety.RedactSecrets(firstNonEmpty(extraction.Summary, extraction.Text, "Connected-source extraction")), 1800),
		Status:             status,
		Priority:           sourceProjectionPriority(extraction),
		ObservedAt:         observedAt,
		Confidence:         confidence,
		VerificationStatus: verificationStatus,
		Attributes: map[string]string{
			"source_id":      source.ID.String(),
			"raw_item_id":    extraction.RawItemID.String(),
			"connector_key":  compact(source.ConnectorKey, 80),
			"content_type":   compact(extraction.ContentType, 80),
			"content_digest": digest,
		},
		Provenance:  provenance,
		Sensitivity: sensitivity,
		LocalOnly:   source.LocalOnly,
		Links:       links,
	})
	if err != nil {
		return nil, err
	}
	outcome := &LifeGraphProjectionOutcome{
		ExtractionID: extraction.ID.String(), DocumentID: result.Primary.ID,
		AlreadyExisted: result.AlreadyExisted, AdvisoryOnly: result.AdvisoryOnly,
		CanExecute: result.CanExecute, GrantsAuthority: result.GrantsAuthority,
	}
	for _, entity := range result.LinkedEntities {
		outcome.LinkedEntityIDs = append(outcome.LinkedEntityIDs, entity.ID)
	}
	for _, relation := range result.Relations {
		outcome.RelationIDs = append(outcome.RelationIDs, relation.ID)
	}
	return outcome, nil
}

func sourceContactCandidateLinks(source *models.ConnectedSource, extraction *models.SourceExtraction, limit int) []lifeontology.OperationalLinkRequest {
	if source == nil || extraction == nil || limit <= 0 || !contactBearingSource(source, extraction) {
		return nil
	}
	if limit > 4 {
		limit = 4
	}
	result := make([]lifeontology.OperationalLinkRequest, 0, limit)
	seen := map[string]struct{}{}
	for _, raw := range strings.Split(extraction.Entities, ",") {
		name := compact(raw, 80)
		key := strings.ToLower(name)
		if !plausibleContactCandidate(name) {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		digest := sha256.Sum256([]byte(extraction.ID.String() + "\x00" + key))
		recordID := extraction.ID.String() + "/contact-candidate/" + hex.EncodeToString(digest[:8])
		result = append(result, lifeontology.OperationalLinkRequest{
			Type: lifeontology.EntityPerson, RecordID: recordID,
			Name:     name,
			Summary:  "Potential person name extracted from a connected-source record. Review before treating it as contact identity.",
			Relation: lifeontology.RelationDocuments, Status: lifeontology.StatusOpen,
			Attributes: map[string]string{
				"record_kind": "contact_candidate", "candidate": "true",
				"review_required": "true", "source_extraction_id": extraction.ID.String(),
			},
			ExternalKey: &lifeontology.ExternalKey{Namespace: "source/contact-candidate", Value: recordID},
			Confidence:  0.35, VerificationStatus: lifeontology.VerificationNeedsReview,
			Sensitivity: sourceCandidateSensitivity(extraction), LocalOnly: source.LocalOnly,
		})
		if len(result) == limit {
			break
		}
	}
	return result
}

func contactBearingSource(source *models.ConnectedSource, extraction *models.SourceExtraction) bool {
	value := strings.ToLower(strings.Join([]string{source.Category, source.ConnectorKey, extraction.ContentType}, " "))
	return containsAny(value, "email", "gmail", "message", "chat", "whatsapp", "contact")
}

func plausibleContactCandidate(value string) bool {
	if len([]rune(value)) < 3 || len([]rune(value)) > 80 || value == strings.ToUpper(value) {
		return false
	}
	first := true
	for _, character := range value {
		if first && !unicode.IsUpper(character) {
			return false
		}
		first = false
		if !unicode.IsLetter(character) && character != '-' && character != '\'' && character != ' ' {
			return false
		}
	}
	_, blocked := contactCandidateStopWords[strings.ToLower(value)]
	return !blocked
}

var contactCandidateStopWords = map[string]struct{}{
	"email": {}, "message": {}, "subject": {}, "decision": {}, "task": {},
	"today": {}, "tomorrow": {}, "please": {}, "thanks": {}, "hello": {},
	"dear": {}, "monday": {}, "tuesday": {}, "wednesday": {}, "thursday": {},
	"friday": {}, "saturday": {}, "sunday": {},
}

func sourceCandidateSensitivity(extraction *models.SourceExtraction) lifeontology.Sensitivity {
	if extraction != nil && extraction.Sensitive {
		return lifeontology.SensitivitySensitive
	}
	return lifeontology.SensitivityInternal
}

func sourceProjectionDigest(source *models.ConnectedSource, extraction *models.SourceExtraction) string {
	value := strings.Join([]string{
		source.ID.String(), extraction.ID.String(), extraction.RawItemID.String(),
		extraction.ContentHash, extraction.Text, extraction.Summary, extraction.SourceURI,
	}, "\x00")
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func sourceProjectionDomain(source *models.ConnectedSource, extraction *models.SourceExtraction) lifeontology.Domain {
	value := strings.ToLower(strings.Join([]string{source.Category, source.ConnectorKey, extraction.ContentType, extraction.ProjectKey}, " "))
	switch {
	case containsAny(value, "legal", "lawyer", "government", "court", "municipality", "juridisch", "overheid"):
		return lifeontology.DomainLegalGovernment
	case containsAny(value, "invoice", "receipt", "bank", "finance", "accounting", "factuur", "rekening"):
		return lifeontology.DomainFinancial
	case containsAny(value, "medical", "health", "wellbeing", "medisch", "gezondheid"):
		return lifeontology.DomainHealthWellbeing
	case containsAny(value, "github", "code", "software", "work", "trello", "odoo", "project"):
		return lifeontology.DomainWorkVenture
	default:
		return lifeontology.DomainPersonalAdmin
	}
}

func sourceProjectionPriority(extraction *models.SourceExtraction) int {
	if extraction == nil {
		return 25
	}
	if extraction.Uncertain {
		return 55
	}
	if strings.TrimSpace(extraction.Tasks) != "" || strings.TrimSpace(extraction.FollowUps) != "" {
		return 65
	}
	return 35
}

func sourceLifecycleStatus(source *models.ConnectedSource) lifeontology.LifecycleStatus {
	if source == nil {
		return lifeontology.StatusUnknown
	}
	switch source.Status {
	case "revoked":
		return lifeontology.StatusArchived
	case "paused":
		return lifeontology.StatusWaiting
	default:
		return lifeontology.StatusActive
	}
}

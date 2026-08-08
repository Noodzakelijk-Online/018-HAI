package lifeontology

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	ErrContactReviewConflict = errors.New("contact review subject already has a decision")
	contactReviewKeyPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,127}$`)
)

type canonicalContactReviewRequest struct {
	ContractVersion  string               `json:"contractVersion"`
	OwnerIdentity    string               `json:"ownerIdentity"`
	IdempotencyKey   string               `json:"idempotencyKey"`
	Subject          ContactReviewSubject `json:"subject"`
	SubjectID        string               `json:"subjectId"`
	Action           ContactReviewAction  `json:"action"`
	CandidateIDs     []string             `json:"candidateEntityIds"`
	CanonicalName    string               `json:"canonicalName"`
	CanonicalSummary string               `json:"canonicalSummary"`
	Reason           string               `json:"reason"`
	DecidedAt        string               `json:"decidedAt"`
}

type canonicalContactReviewRecord struct {
	RequestDigest     string `json:"requestDigest"`
	CanonicalEntityID string `json:"canonicalEntityId"`
	RecordedAt        string `json:"recordedAt"`
}

func (s *Service) DecideContactCandidate(ctx context.Context, request DecideContactCandidateRequest) (ContactReviewDecisionResult, error) {
	if s == nil || s.repo == nil || s.clock == nil {
		return ContactReviewDecisionResult{}, fmt.Errorf("life ontology service is unavailable")
	}
	owner := compact(request.OwnerIdentity)
	candidate, err := s.repo.GetEntity(ctx, owner, compact(request.CandidateID))
	if err != nil {
		return ContactReviewDecisionResult{}, err
	}
	if err := validateContactCandidate(candidate); err != nil {
		return ContactReviewDecisionResult{}, err
	}
	if request.Action != ContactReviewPromote && request.Action != ContactReviewCorrect && request.Action != ContactReviewReject {
		return ContactReviewDecisionResult{}, fmt.Errorf("candidate review action must be promote, correct, or reject")
	}
	return s.decideContactReview(ctx, contactReviewInput{
		owner: owner, idempotencyKey: request.IdempotencyKey, subject: ContactReviewCandidate,
		subjectID: candidate.ID, action: request.Action, candidates: []Entity{candidate},
		canonicalName: request.CanonicalName, canonicalSummary: request.CanonicalSummary,
		reason: request.Reason,
	})
}

func (s *Service) DecideContactMerge(ctx context.Context, request DecideContactMergeRequest) (ContactReviewDecisionResult, error) {
	if s == nil || s.repo == nil || s.clock == nil {
		return ContactReviewDecisionResult{}, fmt.Errorf("life ontology service is unavailable")
	}
	owner := compact(request.OwnerIdentity)
	proposal, err := s.repo.GetMergeProposal(ctx, owner, compact(request.ProposalID))
	if err != nil {
		return ContactReviewDecisionResult{}, err
	}
	if request.Action != ContactReviewMerge && request.Action != ContactReviewKeepDistinct && request.Action != ContactReviewReject {
		return ContactReviewDecisionResult{}, fmt.Errorf("merge review action must be merge, keep_distinct, or reject")
	}
	candidates := make([]Entity, 0, len(proposal.CandidateEntityIDs))
	for _, id := range proposal.CandidateEntityIDs {
		candidate, getErr := s.repo.GetEntity(ctx, owner, id)
		if getErr != nil {
			return ContactReviewDecisionResult{}, getErr
		}
		if err := validateContactCandidate(candidate); err != nil {
			return ContactReviewDecisionResult{}, err
		}
		candidates = append(candidates, candidate)
	}
	return s.decideContactReview(ctx, contactReviewInput{
		owner: owner, idempotencyKey: request.IdempotencyKey, subject: ContactReviewMergeProposal,
		subjectID: proposal.ID, action: request.Action, candidates: candidates,
		canonicalName: request.CanonicalName, canonicalSummary: request.CanonicalSummary,
		reason: request.Reason,
	})
}

func (s *Service) ListContactReviewDecisions(ctx context.Context, owner string, limit int) ([]ContactReviewDecision, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("life ontology service is unavailable")
	}
	if err := validateOwner(owner); err != nil {
		return nil, err
	}
	limit, err := normalizedLimit(limit)
	if err != nil {
		return nil, err
	}
	decisions, err := s.repo.ListContactReviewDecisions(ctx, compact(owner), limit)
	if err != nil {
		return nil, err
	}
	return decisions, nil
}

type contactReviewInput struct {
	owner, idempotencyKey, subjectID, canonicalName, canonicalSummary, reason string
	subject                                                                   ContactReviewSubject
	action                                                                    ContactReviewAction
	candidates                                                                []Entity
	decidedAt                                                                 time.Time
}

func (s *Service) decideContactReview(ctx context.Context, input contactReviewInput) (ContactReviewDecisionResult, error) {
	now := s.clock().UTC()
	input.idempotencyKey = compact(input.idempotencyKey)
	input.canonicalName = compact(input.canonicalName)
	input.canonicalSummary = compact(input.canonicalSummary)
	input.reason = compact(input.reason)
	input.decidedAt = now
	if err := validateContactReviewInput(input, now); err != nil {
		return ContactReviewDecisionResult{}, err
	}
	candidateIDs := make([]string, 0, len(input.candidates))
	for _, candidate := range input.candidates {
		candidateIDs = append(candidateIDs, candidate.ID)
	}
	sort.Strings(candidateIDs)
	requestDigest, err := hashCanonical(canonicalContactReviewRequest{
		ContractVersion: ContactReviewContractVersion, OwnerIdentity: input.owner,
		IdempotencyKey: input.idempotencyKey, Subject: input.subject, SubjectID: input.subjectID,
		Action: input.action, CandidateIDs: candidateIDs, CanonicalName: input.canonicalName,
		CanonicalSummary: input.canonicalSummary, Reason: input.reason,
		DecidedAt: input.decidedAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return ContactReviewDecisionResult{}, err
	}

	var canonical *Entity
	if input.action == ContactReviewPromote || input.action == ContactReviewCorrect || input.action == ContactReviewMerge {
		entity, buildErr := buildCanonicalContact(input, requestDigest, now)
		if buildErr != nil {
			return ContactReviewDecisionResult{}, buildErr
		}
		canonical = &entity
	}
	canonicalID := ""
	if canonical != nil {
		canonicalID = canonical.ID
	}
	recordDigest, err := hashCanonical(canonicalContactReviewRecord{
		RequestDigest: requestDigest, CanonicalEntityID: canonicalID,
		RecordedAt: now.Format(time.RFC3339Nano),
	})
	if err != nil {
		return ContactReviewDecisionResult{}, err
	}
	decision := ContactReviewDecision{
		ContractVersion: ContactReviewContractVersion, ID: "life-contact-review-" + recordDigest,
		OwnerIdentity: input.owner, IdempotencyKey: input.idempotencyKey, Subject: input.subject,
		SubjectID: input.subjectID, Action: input.action, CandidateEntityIDs: candidateIDs,
		CanonicalEntityID: canonicalID, CanonicalName: input.canonicalName,
		CanonicalSummary: input.canonicalSummary, Reason: input.reason,
		DecidedAt: input.decidedAt, RecordedAt: now, RequestDigest: requestDigest,
		RecordDigest: recordDigest, LocalOnly: true,
	}
	created, err := s.repo.AppendContactReviewDecision(ctx, decision, canonical)
	if errors.Is(err, ErrExists) {
		existing, getErr := s.repo.GetContactReviewDecisionByIdempotency(ctx, input.owner, input.idempotencyKey)
		if getErr != nil {
			return ContactReviewDecisionResult{}, ErrContactReviewConflict
		}
		if existing.RequestDigest != requestDigest {
			return ContactReviewDecisionResult{}, fmt.Errorf("%w: idempotency key was already used for a different review", ErrContactReviewConflict)
		}
		return s.contactReviewResult(ctx, existing, true)
	}
	if err != nil {
		return ContactReviewDecisionResult{}, err
	}
	return s.contactReviewResult(ctx, created, false)
}

func (s *Service) contactReviewResult(ctx context.Context, decision ContactReviewDecision, existed bool) (ContactReviewDecisionResult, error) {
	result := ContactReviewDecisionResult{Decision: decision, AlreadyExisted: existed}
	if decision.CanonicalEntityID != "" {
		entity, err := s.repo.GetEntity(ctx, decision.OwnerIdentity, decision.CanonicalEntityID)
		if err != nil {
			return ContactReviewDecisionResult{}, err
		}
		result.CanonicalEntity = &entity
	}
	return result, nil
}

func buildCanonicalContact(input contactReviewInput, requestDigest string, now time.Time) (Entity, error) {
	first := input.candidates[0]
	name := input.canonicalName
	if name == "" {
		name = first.Name
	}
	if (input.action == ContactReviewCorrect || input.action == ContactReviewMerge) && input.canonicalName == "" {
		return Entity{}, fmt.Errorf("corrected and merged contacts require a canonical name")
	}
	summary := input.canonicalSummary
	if summary == "" {
		summary = first.Summary
	}
	domain := first.Domain
	priority := first.Priority
	validFrom := first.ValidFrom
	sensitivity := first.Sensitivity
	externalKeys := make([]ExternalKey, 0)
	provenance := make([]Provenance, 0)
	for _, candidate := range input.candidates {
		if candidate.Domain != domain {
			return Entity{}, fmt.Errorf("contact candidates from different life domains cannot be merged")
		}
		if candidate.Priority > priority {
			priority = candidate.Priority
		}
		if candidate.ValidFrom.Before(validFrom) {
			validFrom = candidate.ValidFrom
		}
		if sensitivityRank(candidate.Sensitivity) > sensitivityRank(sensitivity) {
			sensitivity = candidate.Sensitivity
		}
		externalKeys = append(externalKeys, candidate.ExternalKeys...)
		provenance = append(provenance, candidate.Provenance...)
	}
	provenance = normalizeProvenance(provenance)
	if len(provenance) > 15 {
		provenance = provenance[:15]
	}
	provenance = append(provenance, Provenance{
		ReferenceID: "contact-review:" + input.idempotencyKey, ContentDigest: requestDigest,
		Authority: "authenticated_owner", CapturedAt: input.decidedAt, LocalOnly: true,
	})
	entity, err := normalizeEntityRequest(RecordEntityRequest{
		OwnerIdentity: input.owner, Type: EntityPerson, Domain: domain, Name: name, Summary: summary,
		ExternalKeys: externalKeys, Attributes: map[string]string{
			"canonical": "true", "review_action": string(input.action),
			"source_candidate_count": fmt.Sprint(len(input.candidates)),
			"review_request_digest":  requestDigest,
		},
		Status: StatusActive, Priority: priority, ValidFrom: validFrom, ObservedAt: input.decidedAt,
		Confidence: 1, VerificationStatus: VerificationHumanApproved, Provenance: provenance,
		Sensitivity: sensitivity, LocalOnly: true,
	}, now)
	if err != nil {
		return Entity{}, err
	}
	return entity, nil
}

func validateContactCandidate(candidate Entity) error {
	if candidate.Type != EntityPerson || candidate.Attributes["candidate"] != "true" || candidate.VerificationStatus != VerificationNeedsReview {
		return fmt.Errorf("contact review requires a source-derived person candidate awaiting review")
	}
	if !candidate.LocalOnly || (candidate.Sensitivity != SensitivitySensitive && candidate.Sensitivity != SensitivityRestricted) {
		return fmt.Errorf("contact candidate privacy boundary is invalid")
	}
	return nil
}

func validateContactReviewInput(input contactReviewInput, now time.Time) error {
	if err := validateOwner(input.owner); err != nil {
		return err
	}
	if !contactReviewKeyPattern.MatchString(input.idempotencyKey) {
		return fmt.Errorf("contact review idempotency key must be 8-128 safe characters")
	}
	if input.subject != ContactReviewCandidate && input.subject != ContactReviewMergeProposal {
		return fmt.Errorf("invalid contact review subject")
	}
	if input.subjectID == "" || len(input.subjectID) > 128 || len(input.candidates) < 1 || len(input.candidates) > 2 {
		return fmt.Errorf("contact review subject and candidates are required")
	}
	if len(input.reason) < 3 || len(input.reason) > 1024 || len(input.canonicalName) > 256 || len(input.canonicalSummary) > 2048 {
		return fmt.Errorf("contact review reason and canonical text must be bounded")
	}
	if input.decidedAt.IsZero() || input.decidedAt.After(now) {
		return fmt.Errorf("contact review decidedAt is required and cannot be in the future")
	}
	for label, value := range map[string]string{
		"contact review reason":     input.reason,
		"canonical contact name":    input.canonicalName,
		"canonical contact summary": input.canonicalSummary,
	} {
		if err := rejectSecret(label, value); err != nil {
			return err
		}
	}
	return nil
}

func validateContactReviewDecision(decision ContactReviewDecision) error {
	if decision.ContractVersion != ContactReviewContractVersion || !strings.HasPrefix(decision.ID, "life-contact-review-") {
		return fmt.Errorf("invalid contact review contract identity")
	}
	if err := validateOwner(decision.OwnerIdentity); err != nil {
		return err
	}
	if !contactReviewKeyPattern.MatchString(decision.IdempotencyKey) || !sha256Pattern.MatchString(decision.RequestDigest) || !sha256Pattern.MatchString(decision.RecordDigest) || decision.ID != "life-contact-review-"+decision.RecordDigest {
		return fmt.Errorf("invalid contact review digest identity")
	}
	if decision.Subject != ContactReviewCandidate && decision.Subject != ContactReviewMergeProposal {
		return fmt.Errorf("invalid contact review subject")
	}
	if (decision.Subject == ContactReviewCandidate && decision.Action != ContactReviewPromote && decision.Action != ContactReviewCorrect && decision.Action != ContactReviewReject) ||
		(decision.Subject == ContactReviewMergeProposal && decision.Action != ContactReviewMerge && decision.Action != ContactReviewKeepDistinct && decision.Action != ContactReviewReject) {
		return fmt.Errorf("contact review action does not match subject")
	}
	if (decision.Subject == ContactReviewCandidate && (!validEntityID(decision.SubjectID) || len(decision.CandidateEntityIDs) != 1 || decision.CandidateEntityIDs[0] != decision.SubjectID)) ||
		(decision.Subject == ContactReviewMergeProposal && (!validMergeProposalID(decision.SubjectID) || len(decision.CandidateEntityIDs) != 2)) {
		return fmt.Errorf("contact review subject does not match candidates")
	}
	if len(decision.CandidateEntityIDs) < 1 || len(decision.CandidateEntityIDs) > 2 || len(decision.Reason) < 3 || decision.DecidedAt.IsZero() || decision.RecordedAt.IsZero() || decision.RecordedAt.Before(decision.DecidedAt) {
		return fmt.Errorf("invalid contact review evidence")
	}
	if len(decision.Reason) > 1024 || len(decision.CanonicalName) > 256 || len(decision.CanonicalSummary) > 2048 ||
		((decision.Action == ContactReviewCorrect || decision.Action == ContactReviewMerge) && decision.CanonicalName == "") {
		return fmt.Errorf("contact review canonical text is invalid")
	}
	for _, id := range decision.CandidateEntityIDs {
		if !validEntityID(id) {
			return fmt.Errorf("invalid contact candidate identity")
		}
	}
	if !sort.StringsAreSorted(decision.CandidateEntityIDs) {
		return fmt.Errorf("contact candidate identities must be canonical")
	}
	createsCanonical := decision.Action == ContactReviewPromote || decision.Action == ContactReviewCorrect || decision.Action == ContactReviewMerge
	if createsCanonical != (decision.CanonicalEntityID != "") || (decision.CanonicalEntityID != "" && !validEntityID(decision.CanonicalEntityID)) {
		return fmt.Errorf("contact review canonical entity does not match action")
	}
	if !decision.LocalOnly || decision.CanExecute || decision.GrantsAuthority {
		return fmt.Errorf("contact review cannot leave the device or grant authority")
	}
	requestDigest, err := hashCanonical(canonicalContactReviewRequest{
		ContractVersion: decision.ContractVersion, OwnerIdentity: decision.OwnerIdentity,
		IdempotencyKey: decision.IdempotencyKey, Subject: decision.Subject, SubjectID: decision.SubjectID,
		Action: decision.Action, CandidateIDs: decision.CandidateEntityIDs,
		CanonicalName: decision.CanonicalName, CanonicalSummary: decision.CanonicalSummary,
		Reason: decision.Reason, DecidedAt: decision.DecidedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil || requestDigest != decision.RequestDigest {
		return fmt.Errorf("contact review request digest mismatch")
	}
	recordDigest, err := hashCanonical(canonicalContactReviewRecord{
		RequestDigest: decision.RequestDigest, CanonicalEntityID: decision.CanonicalEntityID,
		RecordedAt: decision.RecordedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil || recordDigest != decision.RecordDigest {
		return fmt.Errorf("contact review record digest mismatch")
	}
	return nil
}

func sensitivityRank(value Sensitivity) int {
	return map[Sensitivity]int{SensitivityPublic: 0, SensitivityInternal: 1, SensitivitySensitive: 2, SensitivityRestricted: 3}[value]
}

func validMergeProposalID(value string) bool {
	const prefix = "life-merge-"
	return strings.HasPrefix(value, prefix) && sha256Pattern.MatchString(strings.TrimPrefix(value, prefix))
}

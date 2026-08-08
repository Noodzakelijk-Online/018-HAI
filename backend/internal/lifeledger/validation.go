package lifeledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"automation-hub-backend/internal/lifeontology"
)

var (
	ErrNotFound            = errors.New("life ledger record not found")
	ErrRevisionConflict    = errors.New("life ledger revision conflict")
	ErrIdempotencyConflict = errors.New("life ledger idempotency conflict")
	ErrCorruptRecord       = errors.New("life ledger record failed integrity verification")
	currencyPattern        = regexp.MustCompile(`^[A-Z]{3}$`)
)

var commitmentStatuses = map[CommitmentStatus]struct{}{
	CommitmentProposed: {}, CommitmentActive: {}, CommitmentWaiting: {},
	CommitmentFulfilled: {}, CommitmentCancelled: {}, CommitmentBreached: {},
	CommitmentDisputed: {},
}

var costKinds = map[CostKind]struct{}{
	CostEstimate: {}, CostIncurred: {}, CostPaid: {}, CostRefund: {},
}

var verificationStatuses = map[VerificationStatus]struct{}{
	VerificationSourceSupported: {}, VerificationHumanConfirmed: {},
	VerificationVerified: {}, VerificationNeedsReview: {}, VerificationDisputed: {},
}

var lifeDomains = map[lifeontology.Domain]struct{}{
	lifeontology.DomainSafetySecurity: {}, lifeontology.DomainHealthWellbeing: {},
	lifeontology.DomainRelationships: {}, lifeontology.DomainHousingAssets: {},
	lifeontology.DomainFinancial: {}, lifeontology.DomainWorkVenture: {},
	lifeontology.DomainLearningGrowth: {}, lifeontology.DomainMeaningValues: {},
	lifeontology.DomainCommunityCivic: {}, lifeontology.DomainLegalGovernment: {},
	lifeontology.DomainPersonalAdmin: {},
}

func normalizeCommitmentRequest(request RecordCommitmentRequest, now time.Time) (RecordCommitmentRequest, error) {
	request.OwnerIdentity = clean(request.OwnerIdentity)
	request.CommitmentKey = clean(request.CommitmentKey)
	request.Title = clean(request.Title)
	request.Summary = clean(request.Summary)
	request.Counterparty = clean(request.Counterparty)
	request.ProjectKey = clean(request.ProjectKey)
	request.IdempotencyKey = clean(request.IdempotencyKey)
	request.Evidence = normalizeEvidence(request.Evidence)
	request.ObservedAt = normalizedObservedAt(request.ObservedAt, now)
	request.DueAt = normalizedTimePointer(request.DueAt)
	if err := validateCommon(request.OwnerIdentity, request.Domain, request.Title, request.Summary, request.Verification, request.Evidence, request.IdempotencyKey, request.ObservedAt, now); err != nil {
		return request, err
	}
	if err := bounded("commitment key", request.CommitmentKey, 256, true); err != nil {
		return request, err
	}
	if _, ok := commitmentStatuses[request.Status]; !ok {
		return request, fmt.Errorf("unsupported commitment status %q", request.Status)
	}
	if err := bounded("counterparty", request.Counterparty, 255, false); err != nil {
		return request, err
	}
	if err := bounded("project key", request.ProjectKey, 255, false); err != nil {
		return request, err
	}
	if request.ExpectedRevision == 0 && request.Status != CommitmentProposed && request.Status != CommitmentActive && request.Status != CommitmentWaiting {
		return request, fmt.Errorf("first commitment revision must be proposed, active, or waiting")
	}
	return request, nil
}

func normalizeCostRequest(request RecordCostRequest, now time.Time) (RecordCostRequest, error) {
	request.OwnerIdentity = clean(request.OwnerIdentity)
	request.Title = clean(request.Title)
	request.Summary = clean(request.Summary)
	request.Currency = strings.ToUpper(clean(request.Currency))
	request.CommitmentKey = clean(request.CommitmentKey)
	request.ProjectKey = clean(request.ProjectKey)
	request.IdempotencyKey = clean(request.IdempotencyKey)
	request.Evidence = normalizeEvidence(request.Evidence)
	request.ObservedAt = normalizedObservedAt(request.ObservedAt, now)
	if err := validateCommon(request.OwnerIdentity, request.Domain, request.Title, request.Summary, request.Verification, request.Evidence, request.IdempotencyKey, request.ObservedAt, now); err != nil {
		return request, err
	}
	if _, ok := costKinds[request.Kind]; !ok {
		return request, fmt.Errorf("unsupported cost kind %q", request.Kind)
	}
	if !validCostVerification(request.Kind, request.Verification) {
		return request, fmt.Errorf("verification %q is not sufficient for cost kind %q", request.Verification, request.Kind)
	}
	if request.AmountMinor <= 0 {
		return request, fmt.Errorf("amountMinor must be positive")
	}
	if !currencyPattern.MatchString(request.Currency) {
		return request, fmt.Errorf("currency must be an ISO-style three-letter code")
	}
	if err := bounded("commitment key", request.CommitmentKey, 256, false); err != nil {
		return request, err
	}
	if err := bounded("project key", request.ProjectKey, 255, false); err != nil {
		return request, err
	}
	return request, nil
}

func validCostVerification(kind CostKind, verification VerificationStatus) bool {
	switch kind {
	case CostEstimate:
		return verification == VerificationNeedsReview ||
			verification == VerificationSourceSupported ||
			verification == VerificationHumanConfirmed ||
			verification == VerificationVerified
	case CostIncurred:
		return verification == VerificationSourceSupported ||
			verification == VerificationHumanConfirmed ||
			verification == VerificationVerified
	case CostPaid, CostRefund:
		return verification == VerificationHumanConfirmed || verification == VerificationVerified
	default:
		return false
	}
}

func validateCommon(owner string, domain lifeontology.Domain, title, summary string, verification VerificationStatus, evidence []EvidenceReference, idempotency string, observedAt, now time.Time) error {
	if err := bounded("owner identity", owner, 255, true); err != nil {
		return err
	}
	if _, ok := lifeDomains[domain]; !ok {
		return fmt.Errorf("unsupported life domain %q", domain)
	}
	if err := bounded("title", title, 255, true); err != nil {
		return err
	}
	if err := bounded("summary", summary, 4000, false); err != nil {
		return err
	}
	if _, ok := verificationStatuses[verification]; !ok {
		return fmt.Errorf("unsupported verification status %q", verification)
	}
	if err := bounded("idempotency key", idempotency, 255, true); err != nil {
		return err
	}
	if len(evidence) == 0 || len(evidence) > 32 {
		return fmt.Errorf("between 1 and 32 evidence references are required")
	}
	for index, reference := range evidence {
		if err := validateEvidence(reference, observedAt); err != nil {
			return fmt.Errorf("evidence %d: %w", index, err)
		}
	}
	if observedAt.IsZero() || observedAt.After(now.Add(5*time.Minute)) {
		return fmt.Errorf("observedAt is required and cannot be in the future")
	}
	return nil
}

func validateEvidence(reference EvidenceReference, recordObservedAt time.Time) error {
	if err := bounded("id", reference.ID, 255, true); err != nil {
		return err
	}
	if err := bounded("URI", reference.URI, 2048, true); err != nil {
		return err
	}
	if !validDigest(reference.ContentDigest) {
		return fmt.Errorf("content digest must be a SHA-256 hex digest")
	}
	if err := bounded("authority", reference.Authority, 255, false); err != nil {
		return err
	}
	if reference.ObservedAt.IsZero() || reference.ObservedAt.After(recordObservedAt.Add(5*time.Minute)) {
		return fmt.Errorf("evidence observedAt must not follow the ledger observation")
	}
	if _, ok := verificationStatuses[reference.Verification]; !ok {
		return fmt.Errorf("unsupported evidence verification %q", reference.Verification)
	}
	return nil
}

func normalizeEvidence(values []EvidenceReference) []EvidenceReference {
	result := make([]EvidenceReference, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value.ID = clean(value.ID)
		value.URI = strings.TrimSpace(value.URI)
		value.ContentDigest = strings.ToLower(clean(value.ContentDigest))
		value.Authority = clean(value.Authority)
		value.ObservedAt = value.ObservedAt.UTC()
		value.LocalOnly = true
		key := value.ID + "\x00" + value.ContentDigest
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizedObservedAt(value, now time.Time) time.Time {
	if value.IsZero() {
		return now.UTC()
	}
	return value.UTC()
}

func normalizedTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func clean(value string) string { return strings.Join(strings.Fields(strings.TrimSpace(value)), " ") }

func bounded(label, value string, limit int, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > limit {
		return fmt.Errorf("%s exceeds %d characters", label, limit)
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func digest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func commitmentRequestDigest(request RecordCommitmentRequest) (string, error) {
	request.OwnerIdentity = ""
	return digest(request)
}

func costRequestDigest(request RecordCostRequest) (string, error) {
	request.OwnerIdentity = ""
	return digest(request)
}

func commitmentRecordDigest(record CommitmentRevision) (string, error) {
	record.RecordDigest = ""
	record.LifeGraph = nil
	record.LifeGraphWarning = ""
	return digest(record)
}

func costRecordDigest(record CostEntry) (string, error) {
	record.RecordDigest = ""
	record.LifeGraph = nil
	record.LifeGraphWarning = ""
	return digest(record)
}

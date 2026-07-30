package proactive

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	maxIdentifierLength = 200
	maxTextLength       = 4000
	maxRuleTypes        = 16
	maxSources          = 64
	maxRetryIntervals   = 12
	maxRetryAttempts    = 12
	maxEscalations      = 5
)

func validateIdentity(value string) error {
	return validateText("owner identity", value, 320)
}

func validateIdentifier(name, value string) error {
	return validateText(name, value, maxIdentifierLength)
}

func validateText(name, value string, max int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > max {
		return fmt.Errorf("%s exceeds %d characters", name, max)
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return fmt.Errorf("%s contains control characters", name)
		}
	}
	return nil
}

func validateUnit(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("%s must be between 0 and 1", name)
	}
	return nil
}

func validateWeights(weights ScoreWeights) error {
	values := []struct {
		name  string
		value float64
	}{
		{"relevance weight", weights.Relevance},
		{"urgency weight", weights.Urgency},
		{"importance weight", weights.Importance},
		{"risk weight", weights.Risk},
	}
	total := 0.0
	for _, item := range values {
		if item.value < 0.10 || item.value > 0.50 || math.IsNaN(item.value) || math.IsInf(item.value, 0) {
			return fmt.Errorf("%s must be between 0.10 and 0.50", item.name)
		}
		total += item.value
	}
	if math.Abs(total-1) > 0.000001 {
		return fmt.Errorf("score weights must sum to 1")
	}
	return nil
}

func validateSignal(signal Signal, now time.Time) error {
	if signal.ContractVersion != ContractVersion {
		return fmt.Errorf("unsupported signal contract version %d", signal.ContractVersion)
	}
	if err := validateIdentity(signal.OwnerIdentity); err != nil {
		return err
	}
	if err := validateIdentifier("signal id", signal.ID); err != nil {
		return err
	}
	if err := validateIdentifier("idempotency key", signal.IdempotencyKey); err != nil {
		return err
	}
	if err := validateIdentifier("open loop key", signal.OpenLoopKey); err != nil {
		return err
	}
	if err := validateText("signal title", signal.Title, 300); err != nil {
		return err
	}
	if err := validateText("signal summary", signal.Summary, maxTextLength); err != nil {
		return err
	}
	if !validSignalType(signal.Type) {
		return fmt.Errorf("unsupported signal type %q", signal.Type)
	}
	if !validResponsible(signal.Responsible) {
		return fmt.Errorf("unsupported responsible party %q", signal.Responsible)
	}
	if !validDomain(signal.Domain) {
		return fmt.Errorf("unsupported decision domain %q", signal.Domain)
	}
	if !validRisk(signal.Risk) {
		return fmt.Errorf("unsupported risk %q", signal.Risk)
	}
	if !validSensitivity(signal.Sensitivity) {
		return fmt.Errorf("unsupported sensitivity %q", signal.Sensitivity)
	}
	if err := validateUnit("confidence", signal.Confidence); err != nil {
		return err
	}
	if err := validateUnit("relevance", signal.Relevance); err != nil {
		return err
	}
	if err := validateUnit("importance", signal.Importance); err != nil {
		return err
	}
	if signal.OccurredAt.IsZero() || signal.OccurredAt.After(now.Add(time.Minute)) {
		return fmt.Errorf("signal occurrence must be present and not in the future")
	}
	if signal.DueAt != nil && signal.DueAt.IsZero() {
		return fmt.Errorf("signal due time cannot be zero")
	}
	if signal.ResolvedAt != nil && signal.ResolvedAt.IsZero() {
		return fmt.Errorf("signal resolution time cannot be zero")
	}
	if len(signal.Sources) == 0 || len(signal.Sources) > maxSources {
		return fmt.Errorf("signal must contain between 1 and %d source references", maxSources)
	}
	seenSources := make(map[string]struct{}, len(signal.Sources))
	for _, source := range signal.Sources {
		if err := validateSource(source, now); err != nil {
			return err
		}
		if _, duplicate := seenSources[source.ID]; duplicate {
			return fmt.Errorf("duplicate source id %q", source.ID)
		}
		seenSources[source.ID] = struct{}{}
	}
	return nil
}

func validateSource(source SourceReference, now time.Time) error {
	if err := validateIdentifier("source id", source.ID); err != nil {
		return err
	}
	if err := validateIdentifier("source kind", source.Kind); err != nil {
		return err
	}
	if err := validateText("source locator", source.Locator, 1000); err != nil {
		return err
	}
	if err := validateIdentifier("source content hash", source.ContentHash); err != nil {
		return err
	}
	if source.ObservedAt.IsZero() || source.ObservedAt.After(now.Add(time.Minute)) {
		return fmt.Errorf("source observed time must be present and not in the future")
	}
	if source.RetrievedAt.IsZero() || source.RetrievedAt.After(now.Add(time.Minute)) {
		return fmt.Errorf("source retrieval time must be present and not in the future")
	}
	if source.RetrievedAt.Before(source.ObservedAt) {
		return fmt.Errorf("source retrieval cannot precede observation")
	}
	switch source.Verification {
	case VerificationVerified, VerificationSourceSupported, VerificationUncertain, VerificationConflicting, VerificationUnsupported:
	default:
		return fmt.Errorf("unsupported source verification %q", source.Verification)
	}
	return nil
}

func normalizeRule(rule TriggerRule, now time.Time) (TriggerRule, error) {
	rule.ContractVersion = ContractVersion
	rule.ID = strings.TrimSpace(rule.ID)
	rule.OwnerIdentity = strings.TrimSpace(rule.OwnerIdentity)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Digest = ""
	if rule.Version == 0 {
		rule.Version = 1
	}
	if rule.Weights == (ScoreWeights{}) {
		rule.Weights = DefaultScoreWeights()
	}
	rule.SignalTypes = uniqueSortedSignalTypes(rule.SignalTypes)
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now
	if err := validateRule(rule); err != nil {
		return TriggerRule{}, err
	}
	digestInput := rule
	digestInput.Digest = ""
	digest, err := digestValue(digestInput)
	if err != nil {
		return TriggerRule{}, err
	}
	rule.Digest = digest
	return rule, nil
}

func validateRule(rule TriggerRule) error {
	if rule.ContractVersion != ContractVersion {
		return fmt.Errorf("unsupported rule contract version %d", rule.ContractVersion)
	}
	if err := validateIdentity(rule.OwnerIdentity); err != nil {
		return err
	}
	if err := validateIdentifier("rule id", rule.ID); err != nil {
		return err
	}
	if err := validateText("rule name", rule.Name, 300); err != nil {
		return err
	}
	if rule.Version == 0 {
		return fmt.Errorf("rule version must be positive")
	}
	if len(rule.SignalTypes) == 0 || len(rule.SignalTypes) > maxRuleTypes {
		return fmt.Errorf("rule must contain between 1 and %d signal types", maxRuleTypes)
	}
	for _, signalType := range rule.SignalTypes {
		if !validSignalType(signalType) {
			return fmt.Errorf("unsupported signal type %q", signalType)
		}
	}
	if err := validateUnit("minimum confidence", rule.MinimumConfidence); err != nil {
		return err
	}
	if rule.MaximumSourceAge <= 0 {
		return fmt.Errorf("maximum source age must be positive")
	}
	if rule.Cooldown < 0 {
		return fmt.Errorf("cooldown cannot be negative")
	}
	if rule.ProposalTTL <= 0 {
		return fmt.Errorf("proposal TTL must be positive")
	}
	if err := validateWeights(rule.Weights); err != nil {
		return err
	}
	if err := validateQuietHours(rule.QuietHours); err != nil {
		return err
	}
	if len(rule.Retry.Intervals) == 0 || len(rule.Retry.Intervals) > maxRetryIntervals {
		return fmt.Errorf("retry policy must contain between 1 and %d intervals", maxRetryIntervals)
	}
	if rule.Retry.MaxAttempts < 1 || rule.Retry.MaxAttempts > maxRetryAttempts {
		return fmt.Errorf("max retry attempts must be between 1 and %d", maxRetryAttempts)
	}
	if rule.Retry.MaxAttempts > len(rule.Retry.Intervals) {
		return fmt.Errorf("max retry attempts cannot exceed configured intervals")
	}
	if rule.Retry.MaxEscalations < 0 || rule.Retry.MaxEscalations > maxEscalations {
		return fmt.Errorf("max escalations must be between 0 and %d", maxEscalations)
	}
	for _, interval := range rule.Retry.Intervals {
		if interval <= 0 || interval > 30*24*time.Hour {
			return fmt.Errorf("retry intervals must be positive and no longer than 30 days")
		}
	}
	return nil
}

func validateQuietHours(value QuietHours) error {
	if !value.Enabled {
		return nil
	}
	if value.StartMinute < 0 || value.StartMinute >= 24*60 || value.EndMinute < 0 || value.EndMinute >= 24*60 {
		return fmt.Errorf("quiet-hour minutes must be within a day")
	}
	if value.StartMinute == value.EndMinute {
		return fmt.Errorf("quiet hours cannot cover an ambiguous full day")
	}
	if strings.TrimSpace(value.TimeZone) == "" {
		return fmt.Errorf("quiet-hour time zone is required")
	}
	if _, err := time.LoadLocation(value.TimeZone); err != nil {
		return fmt.Errorf("load quiet-hour time zone: %w", err)
	}
	return nil
}

func uniqueSortedSignalTypes(values []SignalType) []SignalType {
	seen := make(map[SignalType]struct{}, len(values))
	result := make([]SignalType, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func validSignalType(value SignalType) bool {
	switch value {
	case SignalDeadline, SignalCommitment, SignalWaitingState, SignalStaleWork,
		SignalSourceChange, SignalRecurringObligation, SignalCapacityConstraint, SignalReviewQueue:
		return true
	default:
		return false
	}
}

func validResponsible(value ResponsibleParty) bool {
	return value == ResponsibleRobert || value == ResponsibleHAI || value == ResponsibleExternal
}

func validDomain(value DecisionDomain) bool {
	return value == DomainGeneral || value == DomainLegal || value == DomainMedical || value == DomainFinancial
}

func validRisk(value RiskLevel) bool {
	return value == RiskLow || value == RiskMedium || value == RiskHigh || value == RiskCritical
}

func validSensitivity(value Sensitivity) bool {
	return value == SensitivityStandard || value == SensitivitySensitive || value == SensitivityRestricted
}

package ambientmonitor

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	maxIdentifierLength = 128
	maxFailureLength    = 512
	maxHistory          = 4096
	maxClaimLimit       = 100
	minCadence          = time.Minute
	maxCadence          = 30 * 24 * time.Hour
	minLeaseDuration    = 5 * time.Second
	maxLeaseDuration    = 30 * time.Minute
	maxScheduleHorizon  = 365 * 24 * time.Hour
	maxClockSkew        = 5 * time.Minute
	maxObservationAge   = 365 * 24 * time.Hour
	maxCountValue       = 1_000_000_000_000
)

var (
	identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$`)
	digestPattern     = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

func advisoryAuthority() AuthorityControl {
	return AuthorityControl{Label: AuthorityLabel}
}

func validateAuthority(value AuthorityControl) error {
	if value.Label != AuthorityLabel || value.CanExecute || value.CanDeliver || value.CanNotify ||
		value.CanWriteCalendar || value.CanMutateWorkflow || value.CanAuthorizeMandate || value.CanMutateLearning {
		return fmt.Errorf("%w: monitor authority must remain advisory-only", ErrInvalidInput)
	}
	return nil
}

func validateScope(scope Scope) (Scope, error) {
	scope.OwnerID = strings.TrimSpace(scope.OwnerID)
	scope.WorkspaceID = strings.TrimSpace(scope.WorkspaceID)
	if err := validateIdentifier("owner id", scope.OwnerID); err != nil {
		return Scope{}, err
	}
	if err := validateIdentifier("workspace id", scope.WorkspaceID); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

func validateIdentifier(name, value string) error {
	trimmed := strings.TrimSpace(value)
	if value != trimmed || len(value) == 0 || len(value) > maxIdentifierLength || !identifierPattern.MatchString(value) || containsSecret(value) {
		return fmt.Errorf("%w: %s is invalid", ErrInvalidInput, name)
	}
	return nil
}

func validateBoundedText(name, value string, maximum int, required bool) error {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalidInput, name)
	}
	if len(value) > maximum || containsControl(value) || containsSecret(value) {
		return fmt.Errorf("%w: %s is invalid", ErrInvalidInput, name)
	}
	return nil
}

func validateSourceKind(value SourceKind) error {
	switch value {
	case SourceWorkflowOpenLoopCount, SourceWorkflowVerifiedCompletionCount, SourceOverdueCommitmentCount:
		return nil
	default:
		return fmt.Errorf("%w: source kind is unsupported", ErrInvalidInput)
	}
}

func validateCadence(value time.Duration) error {
	if value < minCadence || value > maxCadence || value%time.Second != 0 {
		return fmt.Errorf("%w: cadence must be whole seconds between one minute and 30 days", ErrInvalidInput)
	}
	return nil
}

func validateLeaseDuration(value time.Duration) error {
	if value < minLeaseDuration || value > maxLeaseDuration || value%time.Second != 0 {
		return fmt.Errorf("%w: lease duration must be whole seconds between five seconds and 30 minutes", ErrInvalidInput)
	}
	return nil
}

func validateTime(name string, value time.Time) (time.Time, error) {
	if value.IsZero() || value.Year() < 2000 || value.Year() > 2200 {
		return time.Time{}, fmt.Errorf("%w: %s is outside the supported time range", ErrInvalidInput, name)
	}
	return value.UTC().Truncate(time.Microsecond), nil
}

func validateRequestTime(name string, value, now time.Time) (time.Time, error) {
	value, err := validateTime(name, value)
	if err != nil {
		return time.Time{}, err
	}
	now = now.UTC().Truncate(time.Microsecond)
	if value.Before(now.Add(-maxClockSkew)) || value.After(now.Add(maxClockSkew)) {
		return time.Time{}, fmt.Errorf("%w: %s is outside the allowed clock skew", ErrInvalidInput, name)
	}
	return value, nil
}

func validateDigest(name, value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !digestPattern.MatchString(value) {
		return "", fmt.Errorf("%w: %s must be a lowercase SHA-256 digest", ErrInvalidInput, name)
	}
	return value, nil
}

func validateCollected(value CollectedObservation, completedAt time.Time) (CollectedObservation, error) {
	if math.IsNaN(value.Value) || math.IsInf(value.Value, 0) || value.Value < 0 || value.Value > maxCountValue || math.Trunc(value.Value) != value.Value {
		return CollectedObservation{}, fmt.Errorf("%w: observation value must be a bounded non-negative integer", ErrInvalidInput)
	}
	observedAt, err := validateTime("observation time", value.ObservedAt)
	if err != nil {
		return CollectedObservation{}, err
	}
	if observedAt.Before(completedAt.Add(-maxObservationAge)) || observedAt.After(completedAt.Add(maxClockSkew)) {
		return CollectedObservation{}, fmt.Errorf("%w: observation time is outside the accepted window", ErrInvalidInput)
	}
	digest, err := validateDigest("source digest", value.SourceDigest)
	if err != nil {
		return CollectedObservation{}, err
	}
	value.ObservedAt = observedAt
	value.SourceDigest = digest
	return value, nil
}

func validateLease(value Lease) error {
	if !value.Active() {
		// A released lease retains its generation as a fencing token. Worker and
		// timestamps must still be empty so it cannot be mistaken for ownership.
		if value.WorkerID != "" || !value.ClaimedAt.IsZero() || !value.ExpiresAt.IsZero() {
			return fmt.Errorf("%w: lease must be either complete or empty", ErrInvalidInput)
		}
		return nil
	}
	if err := validateIdentifier("lease worker id", value.WorkerID); err != nil {
		return err
	}
	claimedAt, err := validateTime("lease claim time", value.ClaimedAt)
	if err != nil {
		return err
	}
	expiresAt, err := validateTime("lease expiry time", value.ExpiresAt)
	if err != nil {
		return err
	}
	duration := expiresAt.Sub(claimedAt)
	if duration < minLeaseDuration || duration > maxLeaseDuration || duration%time.Second != 0 {
		return fmt.Errorf("%w: lease expiry is invalid", ErrInvalidInput)
	}
	return nil
}

func validateTarget(value MonitorTarget) (MonitorTarget, error) {
	if value.ContractVersion != ContractVersion {
		return MonitorTarget{}, fmt.Errorf("%w: unsupported target contract version", ErrInvalidInput)
	}
	scope, err := validateScope(value.Scope)
	if err != nil {
		return MonitorTarget{}, err
	}
	value.Scope = scope
	for name, item := range map[string]string{
		"target id": value.ID, "outcome id": value.OutcomeID, "indicator id": value.IndicatorID,
	} {
		if err := validateIdentifier(name, item); err != nil {
			return MonitorTarget{}, err
		}
	}
	if err := validateSourceKind(value.SourceKind); err != nil {
		return MonitorTarget{}, err
	}
	if err := validateCadence(value.Cadence); err != nil {
		return MonitorTarget{}, err
	}
	if value.NextRunAt, err = validateTime("next run time", value.NextRunAt); err != nil {
		return MonitorTarget{}, err
	}
	if value.CreatedAt, err = validateTime("target creation time", value.CreatedAt); err != nil {
		return MonitorTarget{}, err
	}
	if value.UpdatedAt, err = validateTime("target update time", value.UpdatedAt); err != nil {
		return MonitorTarget{}, err
	}
	if value.UpdatedAt.Before(value.CreatedAt) {
		return MonitorTarget{}, fmt.Errorf("%w: target update precedes creation", ErrInvalidInput)
	}
	if err := validateLease(value.Lease); err != nil {
		return MonitorTarget{}, err
	}
	if err := validateAuthority(value.Authority); err != nil {
		return MonitorTarget{}, err
	}
	return value, nil
}

func validateIdempotency(key, digest string) error {
	if err := validateIdentifier("idempotency key", key); err != nil {
		return err
	}
	_, err := validateDigest("idempotency digest", digest)
	return err
}

func exactDigest(operation string, value any) (string, error) {
	payload, err := json.Marshal(struct {
		Operation string `json:"operation"`
		Value     any    `json:"value"`
	}{Operation: operation, Value: value})
	if err != nil {
		return "", fmt.Errorf("%w: encode idempotency input", ErrInvalidInput)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func newRecordID(prefix string) (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("create %s id: %w", prefix, err)
	}
	return prefix + "-" + hex.EncodeToString(buffer), nil
}

func containsControl(value string) bool {
	return strings.ContainsFunc(value, func(r rune) bool { return unicode.IsControl(r) })
}

func containsSecret(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"authorization:", "bearer ", "api_key", "api-key", "password", "passwd",
		"private_key", "client_secret", "access_token", "refresh_token", "sk-", "ghp_", "xoxb-",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func nextCadence(previous time.Time, cadence time.Duration, after time.Time) (time.Time, error) {
	previous = previous.UTC().Truncate(time.Microsecond)
	after = after.UTC().Truncate(time.Microsecond)
	if previous.After(after) {
		return previous, nil
	}
	steps := after.Sub(previous)/cadence + 1
	if steps <= 0 {
		return time.Time{}, fmt.Errorf("%w: cadence progression is out of range", ErrInvalidInput)
	}
	next := previous.Add(steps * cadence)
	if !next.After(after) || next.After(after.Add(cadence)) {
		return time.Time{}, fmt.Errorf("%w: next run time is invalid", ErrInvalidInput)
	}
	return next, nil
}

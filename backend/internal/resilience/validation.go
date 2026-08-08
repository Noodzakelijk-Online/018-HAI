package resilience

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	maxIDLength      = 200
	maxFailureLength = 2000
	maxLeaseTTL      = 24 * time.Hour
	maxHeartbeatAge  = 24 * time.Hour
	maxBackoff       = 30 * 24 * time.Hour
)

var (
	idPattern               = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,199}$`)
	hashPattern             = regexp.MustCompile(`^[a-f0-9]{64}$`)
	secretKeyPattern        = regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|authorization|private[_-]?key)`)
	keyValueSecretPattern   = regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|token|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|authorization)\b\s*[:=]\s*['"]?[^'"\s,;]+`)
	bearerSecretPattern     = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]{8,}`)
	privateKeyPattern       = regexp.MustCompile(`(?is)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	credentialPrefixPattern = regexp.MustCompile(`(?i)\b(sk-(?:proj-)?[a-z0-9_-]{12,}|gh[pousr]_[a-z0-9]{16,}|xox[baprs]-[a-z0-9-]{10,}|AKIA[A-Z0-9]{16})\b`)
	urlCredentialPattern    = regexp.MustCompile(`(?i)\b(https?://)[^/\s:@]+:[^/\s@]+@`)
)

func validateContract(version int) error {
	if version != ContractVersion {
		return fmt.Errorf("resilience: unsupported contract version")
	}
	return nil
}

func validateScope(scope Scope) error {
	if err := validateID("owner id", scope.OwnerID); err != nil {
		return err
	}
	if err := validateID("workspace id", scope.WorkspaceID); err != nil {
		return err
	}
	return nil
}

func requireSameScope(expected, actual Scope) error {
	if err := validateScope(expected); err != nil {
		return err
	}
	if err := validateScope(actual); err != nil {
		return err
	}
	if expected != actual {
		return fmt.Errorf("resilience: owner/workspace scope mismatch")
	}
	return nil
}

func validateID(name, value string) error {
	if len(value) > maxIDLength || !idPattern.MatchString(value) || containsControl(value) || containsSecret(value) {
		return fmt.Errorf("resilience: %s is invalid", name)
	}
	return nil
}

func validateHash(name, value string, optional bool) error {
	value = strings.TrimSpace(value)
	if optional && value == "" {
		return nil
	}
	if !hashPattern.MatchString(value) {
		return fmt.Errorf("resilience: %s must be a lowercase SHA-256 digest", name)
	}
	return nil
}

func validateTime(name string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("resilience: %s is required", name)
	}
	return nil
}

func validateLeaseTTL(ttl time.Duration) error {
	if ttl <= 0 || ttl > maxLeaseTTL {
		return fmt.Errorf("resilience: lease ttl must be positive and no longer than 24 hours")
	}
	return nil
}

func validateHeartbeatAge(age time.Duration) error {
	if age <= 0 || age > maxHeartbeatAge {
		return fmt.Errorf("resilience: heartbeat max age must be positive and no longer than 24 hours")
	}
	return nil
}

func validateRetryPolicy(policy RetryPolicy) error {
	if policy.MaxAttempts == 0 || policy.MaxAttempts > 1_000 {
		return fmt.Errorf("resilience: max attempts must be between 1 and 1000")
	}
	if policy.BaseDelay <= 0 || policy.BaseDelay > maxBackoff {
		return fmt.Errorf("resilience: base delay must be positive and no longer than 30 days")
	}
	if policy.Multiplier == 0 || policy.Multiplier > 100 {
		return fmt.Errorf("resilience: retry multiplier must be between 1 and 100")
	}
	if policy.MaxDelay < policy.BaseDelay || policy.MaxDelay > maxBackoff {
		return fmt.Errorf("resilience: max delay must be at least base delay and no longer than 30 days")
	}
	return nil
}

func validateCircuitPolicy(policy CircuitPolicy) error {
	if policy.FailureThreshold == 0 || policy.FailureThreshold > 1_000 {
		return fmt.Errorf("resilience: circuit failure threshold must be between 1 and 1000")
	}
	if policy.OpenDuration <= 0 || policy.OpenDuration > maxBackoff {
		return fmt.Errorf("resilience: circuit open duration must be positive and no longer than 30 days")
	}
	if policy.MaxHalfOpenProbes == 0 || policy.MaxHalfOpenProbes > 100 {
		return fmt.Errorf("resilience: half-open probe limit must be between 1 and 100")
	}
	return nil
}

func validateFailure(failure Failure) error {
	if err := validateID("failure code", failure.Code); err != nil {
		return err
	}
	if !validFailureClass(failure.Class) {
		return fmt.Errorf("resilience: failure class is unsupported")
	}
	if failure.Message == "" || len(failure.Message) > maxFailureLength || containsControl(failure.Message) || containsSecret(failure.Message) {
		return fmt.Errorf("resilience: failure message must be present, bounded, and redacted")
	}
	return nil
}

func validFailureClass(class FailureClass) bool {
	switch class {
	case FailureTransient, FailureRateLimited, FailurePermanent, FailureInvalidWork, FailureUnauthorized, FailureSecurity, FailureUnknown:
		return true
	default:
		return false
	}
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return true
		}
	}
	return false
}

func containsSecret(value string) bool {
	return RedactText(value) != value
}

// RedactText removes common secret forms from untrusted diagnostic text.
// Structured control-plane inputs are rejected when redaction would change
// them; diagnostic failures may be constructed safely with NewFailure.
func RedactText(value string) string {
	if value == "" {
		return ""
	}
	redacted := privateKeyPattern.ReplaceAllString(value, "[REDACTED_PRIVATE_KEY]")
	redacted = bearerSecretPattern.ReplaceAllString(redacted, "Bearer [REDACTED]")
	redacted = credentialPrefixPattern.ReplaceAllString(redacted, "[REDACTED_CREDENTIAL]")
	redacted = urlCredentialPattern.ReplaceAllString(redacted, "${1}[REDACTED]@")
	redacted = keyValueSecretPattern.ReplaceAllStringFunc(redacted, func(match string) string {
		separator := strings.IndexAny(match, ":=")
		if separator < 0 {
			return "[REDACTED_SECRET]"
		}
		return strings.TrimSpace(match[:separator+1]) + " [REDACTED]"
	})
	return redacted
}

// NewFailure creates a bounded, redacted failure suitable for durable state.
func NewFailure(code string, class FailureClass, untrustedMessage string) (Failure, error) {
	failure := Failure{
		Code:    strings.TrimSpace(code),
		Class:   class,
		Message: strings.TrimSpace(RedactText(untrustedMessage)),
	}
	if len(failure.Message) > maxFailureLength {
		failure.Message = failure.Message[:maxFailureLength]
	}
	if err := validateFailure(failure); err != nil {
		return Failure{}, err
	}
	return failure, nil
}

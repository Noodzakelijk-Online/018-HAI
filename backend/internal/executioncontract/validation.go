package executioncontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]*$`)

type ValidationPolicy struct {
	SchemaVersion              string
	MaximumAutonomy            int
	MaximumDuration            time.Duration
	ClockSkew                  time.Duration
	MaximumResources           int
	MaximumMetadataEntries     int
	MaximumMetadataValueLength int
	RequirePolicyReference     bool
	RequireEvidence            bool
	RequireProvenance          bool
	RequireDigest              bool
}

func DefaultValidationPolicy() ValidationPolicy {
	return ValidationPolicy{
		SchemaVersion:              CurrentSchemaVersion,
		MaximumAutonomy:            10,
		MaximumDuration:            30 * 24 * time.Hour,
		ClockSkew:                  5 * time.Minute,
		MaximumResources:           64,
		MaximumMetadataEntries:     32,
		MaximumMetadataValueLength: 256,
		RequirePolicyReference:     true,
		RequireEvidence:            true,
		RequireProvenance:          true,
		RequireDigest:              true,
	}
}

func Validate(policy ValidationPolicy, envelope Envelope, now time.Time) error {
	if err := validatePolicy(policy); err != nil {
		return err
	}
	if strings.TrimSpace(envelope.SchemaVersion) != policy.SchemaVersion {
		return fmt.Errorf("execution contract schema version is not accepted")
	}
	if err := validateIdentifier("owner ID", envelope.OwnerID, 1, 128); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"run ID":         envelope.RunID,
		"attempt ID":     envelope.AttemptID,
		"correlation ID": envelope.CorrelationID,
	} {
		if err := requireUUID(label, value); err != nil {
			return err
		}
	}
	if err := validateIdempotencyKey(envelope.IdempotencyKey); err != nil {
		return err
	}
	if err := validateTraceID(envelope.TraceID); err != nil {
		return err
	}
	if envelope.AttemptNumber == 0 {
		return fmt.Errorf("attempt number must be at least 1")
	}
	hasParentID := strings.TrimSpace(envelope.ParentAttemptID) != ""
	hasParentDigest := strings.TrimSpace(envelope.ParentContractDigest) != ""
	if hasParentID != hasParentDigest {
		return fmt.Errorf("parent attempt ID and parent contract digest must be supplied together")
	}
	if envelope.AttemptNumber == 1 && hasParentID {
		return fmt.Errorf("root attempt cannot reference a parent")
	}
	if envelope.AttemptNumber > 1 && !hasParentID {
		return fmt.Errorf("child attempt requires parent identity and digest")
	}
	if hasParentID {
		if err := requireUUID("parent attempt ID", envelope.ParentAttemptID); err != nil {
			return err
		}
		if envelope.ParentAttemptID == envelope.AttemptID {
			return fmt.Errorf("attempt cannot be its own parent")
		}
		if err := validateDigest("parent contract digest", envelope.ParentContractDigest); err != nil {
			return err
		}
	}
	if envelope.CreatedAt.IsZero() || envelope.Deadline.IsZero() {
		return fmt.Errorf("creation time and deadline are required")
	}
	now = now.UTC()
	createdAt := envelope.CreatedAt.UTC()
	deadline := envelope.Deadline.UTC()
	if createdAt.After(now.Add(policy.ClockSkew)) {
		return fmt.Errorf("creation time is too far in the future")
	}
	if !deadline.After(now) || !deadline.After(createdAt) {
		return fmt.Errorf("deadline must be after creation time and in the future")
	}
	if deadline.Sub(createdAt) > policy.MaximumDuration {
		return fmt.Errorf("execution contract exceeds the maximum duration")
	}
	if envelope.AutonomyCeiling < 0 || envelope.AutonomyCeiling > policy.MaximumAutonomy {
		return fmt.Errorf("autonomy ceiling must be between 0 and %d", policy.MaximumAutonomy)
	}
	if err := validateAction(envelope.Action); err != nil {
		return err
	}
	if len(envelope.Resources) > policy.MaximumResources {
		return fmt.Errorf("execution contract exceeds the resource limit")
	}
	if envelope.Action.Mode == ModeExecute && len(envelope.Resources) == 0 {
		return fmt.Errorf("execute mode requires at least one bounded resource")
	}
	seenResources := map[string]struct{}{}
	for index, resource := range envelope.Resources {
		if err := validateResource(resource); err != nil {
			return fmt.Errorf("resource %d: %w", index, err)
		}
		key := strings.TrimSpace(resource.Kind) + "\x00" + strings.TrimSpace(resource.Identifier)
		if _, duplicate := seenResources[key]; duplicate {
			return fmt.Errorf("resource %d duplicates an existing resource scope", index)
		}
		seenResources[key] = struct{}{}
	}
	if requiresApproval(envelope) && len(envelope.ApprovalReferences) == 0 {
		return fmt.Errorf("high-risk or sensitive execution requires an approval reference")
	}
	if policy.RequirePolicyReference && len(envelope.PolicyReferences) == 0 {
		return fmt.Errorf("at least one policy reference is required")
	}
	for index, reference := range envelope.PolicyReferences {
		if err := validatePolicyReference(reference); err != nil {
			return fmt.Errorf("policy reference %d: %w", index, err)
		}
	}
	for index, reference := range envelope.ApprovalReferences {
		if err := validateApprovalReference(reference, createdAt, deadline); err != nil {
			return fmt.Errorf("approval reference %d: %w", index, err)
		}
	}
	if policy.RequireEvidence && len(envelope.EvidenceRequirements) == 0 {
		return fmt.Errorf("at least one evidence requirement is required")
	}
	for index, requirement := range envelope.EvidenceRequirements {
		if err := validateEvidenceRequirement(requirement); err != nil {
			return fmt.Errorf("evidence requirement %d: %w", index, err)
		}
	}
	if policy.RequireProvenance && len(envelope.SourceProvenance) == 0 {
		return fmt.Errorf("at least one source provenance record is required")
	}
	for index, source := range envelope.SourceProvenance {
		if err := validateSourceProvenance(source, createdAt); err != nil {
			return fmt.Errorf("source provenance %d: %w", index, err)
		}
	}
	if len(envelope.RedactedMetadata) > policy.MaximumMetadataEntries {
		return fmt.Errorf("execution contract exceeds the metadata entry limit")
	}
	for key, value := range envelope.RedactedMetadata {
		if len(value) > policy.MaximumMetadataValueLength {
			return fmt.Errorf("metadata value for %q exceeds the configured length", key)
		}
		if err := validateMetadataEntry(key, value); err != nil {
			return err
		}
	}
	if policy.RequireDigest {
		if err := validateDigest("contract digest", envelope.ContractDigest); err != nil {
			return err
		}
		digest, err := ComputeDigest(envelope)
		if err != nil {
			return fmt.Errorf("compute execution contract digest: %w", err)
		}
		if !strings.EqualFold(digest, strings.TrimSpace(envelope.ContractDigest)) {
			return fmt.Errorf("execution contract digest does not match the envelope")
		}
	}
	return nil
}

func validatePolicy(policy ValidationPolicy) error {
	if strings.TrimSpace(policy.SchemaVersion) == "" {
		return fmt.Errorf("validation policy schema version is required")
	}
	if policy.MaximumAutonomy < 0 || policy.MaximumAutonomy > 10 {
		return fmt.Errorf("validation policy maximum autonomy must be between 0 and 10")
	}
	if policy.MaximumDuration <= 0 || policy.ClockSkew < 0 {
		return fmt.Errorf("validation policy duration and clock skew are invalid")
	}
	if policy.MaximumResources <= 0 ||
		policy.MaximumMetadataEntries < 0 ||
		policy.MaximumMetadataValueLength <= 0 {
		return fmt.Errorf("validation policy limits are invalid")
	}
	return nil
}

func validateAction(action ActionScope) error {
	if err := validateIdentifier("action name", action.Name, 1, 128); err != nil {
		return err
	}
	if strings.TrimSpace(action.Purpose) == "" || len(action.Purpose) > 512 {
		return fmt.Errorf("action purpose must be between 1 and 512 characters")
	}
	switch action.Mode {
	case ModePlanOnly, ModeRecommend, ModeDraft, ModeExecute:
	default:
		return fmt.Errorf("execution mode %q is invalid", action.Mode)
	}
	switch action.Risk {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
	default:
		return fmt.Errorf("risk level %q is invalid", action.Risk)
	}
	if len(normalizeStrings(action.ProhibitedActions)) == 0 {
		return fmt.Errorf("at least one prohibited action is required")
	}
	for _, tool := range action.AllowedTools {
		if err := validateBoundedValue("allowed tool", tool); err != nil {
			return err
		}
	}
	for _, item := range append(append([]string{}, action.ProhibitedActions...), action.ExpectedSideEffects...) {
		if strings.TrimSpace(item) == "" || len(item) > 256 {
			return fmt.Errorf("action constraints must be between 1 and 256 characters")
		}
	}
	return nil
}

func validateResource(resource ResourceScope) error {
	if err := validateIdentifier("resource kind", resource.Kind, 1, 64); err != nil {
		return err
	}
	if err := validateBoundedValue("resource identifier", resource.Identifier); err != nil {
		return err
	}
	if len(resource.Operations) == 0 {
		return fmt.Errorf("resource operations are required")
	}
	seen := map[ResourceOperation]struct{}{}
	for _, operation := range resource.Operations {
		if !validResourceOperation(operation) {
			return fmt.Errorf("resource operation %q is invalid", operation)
		}
		if _, duplicate := seen[operation]; duplicate {
			return fmt.Errorf("resource operation %q is duplicated", operation)
		}
		seen[operation] = struct{}{}
	}
	return nil
}

func validatePolicyReference(reference PolicyReference) error {
	if err := validateIdentifier("policy ID", reference.ID, 1, 128); err != nil {
		return err
	}
	if err := validateIdentifier("policy version", reference.Version, 1, 64); err != nil {
		return err
	}
	if err := validateIdentifier("policy decision ID", reference.DecisionID, 1, 128); err != nil {
		return err
	}
	return validateDigest("policy decision digest", reference.DecisionDigest)
}

func validateApprovalReference(reference ApprovalReference, createdAt, deadline time.Time) error {
	if err := validateIdentifier("approval ID", reference.ID, 1, 128); err != nil {
		return err
	}
	if err := validateIdentifier("approval actor", reference.GrantedBy, 1, 128); err != nil {
		return err
	}
	if err := validateDigest("approval scope digest", reference.ScopeDigest); err != nil {
		return err
	}
	grantedAt := reference.GrantedAt.UTC()
	expiresAt := reference.ExpiresAt.UTC()
	if grantedAt.IsZero() || expiresAt.IsZero() || !expiresAt.After(grantedAt) {
		return fmt.Errorf("approval grant and expiry times are invalid")
	}
	if grantedAt.After(createdAt) {
		return fmt.Errorf("approval must exist before the execution contract is created")
	}
	if expiresAt.Before(deadline) {
		return fmt.Errorf("approval expires before the execution deadline")
	}
	return nil
}

func validateEvidenceRequirement(requirement EvidenceRequirement) error {
	if err := validateIdentifier("evidence requirement ID", requirement.ID, 1, 128); err != nil {
		return err
	}
	switch requirement.Kind {
	case EvidenceSource,
		EvidenceSchema,
		EvidenceTest,
		EvidenceCalculation,
		EvidenceHumanReview,
		EvidenceExecution,
		EvidenceVerification:
	default:
		return fmt.Errorf("evidence kind %q is invalid", requirement.Kind)
	}
	if strings.TrimSpace(requirement.Description) == "" || len(requirement.Description) > 512 {
		return fmt.Errorf("evidence description must be between 1 and 512 characters")
	}
	if requirement.MinimumCount < 1 {
		return fmt.Errorf("evidence minimum count must be at least 1")
	}
	return validateIdentifier("evidence verifier", requirement.Verifier, 1, 128)
}

func validateSourceProvenance(source SourceProvenance, createdAt time.Time) error {
	if err := validateIdentifier("source ID", source.SourceID, 1, 128); err != nil {
		return err
	}
	if err := validateIdentifier("source type", source.SourceType, 1, 64); err != nil {
		return err
	}
	if err := validateIdentifier("source version", source.SourceVersion, 1, 128); err != nil {
		return err
	}
	if err := validateDigest("source content digest", source.ContentDigest); err != nil {
		return err
	}
	if source.RetrievedAt.IsZero() || source.RetrievedAt.After(createdAt) {
		return fmt.Errorf("source retrieval time must exist and cannot follow contract creation")
	}
	if strings.TrimSpace(source.Authority) == "" || len(source.Authority) > 128 {
		return fmt.Errorf("source authority must be between 1 and 128 characters")
	}
	if source.URI != "" {
		parsed, err := url.ParseRequestURI(source.URI)
		if err != nil || parsed.Scheme == "" {
			return fmt.Errorf("source URI must be absolute when provided")
		}
	}
	return nil
}

func requiresApproval(envelope Envelope) bool {
	if envelope.Action.RequiresApproval ||
		envelope.Action.Risk == RiskHigh ||
		envelope.Action.Risk == RiskCritical {
		return true
	}
	for _, resource := range envelope.Resources {
		for _, operation := range resource.Operations {
			switch operation {
			case OperationDelete, OperationSend, OperationPublish, OperationTransact, OperationAccountChange:
				return true
			}
		}
	}
	return false
}

func validResourceOperation(value ResourceOperation) bool {
	switch value {
	case OperationRead,
		OperationCreate,
		OperationUpdate,
		OperationDelete,
		OperationExecute,
		OperationSend,
		OperationPublish,
		OperationTransact,
		OperationAccountChange:
		return true
	default:
		return false
	}
}

func requireUUID(label, value string) error {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
		return fmt.Errorf("%s must be a UUID", label)
	}
	return nil
}

func validateIdempotencyKey(value string) error {
	value = strings.TrimSpace(value)
	if len(value) < 16 || len(value) > 200 || strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("idempotency key must be 16 to 200 non-whitespace characters")
	}
	if value == "*" {
		return fmt.Errorf("idempotency key cannot be a wildcard")
	}
	return nil
}

func validateTraceID(value string) error {
	value = strings.TrimSpace(value)
	if len(value) != 32 {
		return fmt.Errorf("trace ID must be 32 hexadecimal characters")
	}
	raw, err := hex.DecodeString(value)
	if err != nil {
		return fmt.Errorf("trace ID must be hexadecimal")
	}
	allZero := true
	for _, item := range raw {
		if item != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return fmt.Errorf("trace ID cannot be all zeroes")
	}
	return nil
}

func validateIdentifier(label, value string, minimum, maximum int) error {
	value = strings.TrimSpace(value)
	if len(value) < minimum || len(value) > maximum || !identifierPattern.MatchString(value) {
		return fmt.Errorf("%s must be %d to %d safe identifier characters", label, minimum, maximum)
	}
	return nil
}

func validateBoundedValue(label, value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 512 {
		return fmt.Errorf("%s must be between 1 and 512 characters", label)
	}
	if value == "*" || strings.Contains(value, "://*") {
		return fmt.Errorf("%s cannot contain an unbounded wildcard", label)
	}
	return nil
}

func validateDigest(label, value string) error {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("%s must be a SHA-256 digest", label)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("%s must be a SHA-256 digest", label)
	}
	return nil
}

func validateMetadataEntry(key, value string) error {
	if err := validateIdentifier("metadata key", key, 1, 64); err != nil {
		return err
	}
	if isSensitiveKey(key) && value != RedactedValue {
		return fmt.Errorf("sensitive metadata %q must be redacted", key)
	}
	if value != RedactedValue && containsSecretText(value) {
		return fmt.Errorf("metadata %q contains secret material", key)
	}
	return nil
}

func isSensitiveKey(value string) bool {
	value = strings.ToLower(strings.ReplaceAll(value, "-", "_"))
	for _, marker := range []string{
		"password",
		"passwd",
		"token",
		"secret",
		"api_key",
		"apikey",
		"authorization",
		"credential",
		"private_key",
		"cookie",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func containsSecretText(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"bearer ",
		"-----begin private key-----",
		"-----begin rsa private key-----",
		"ghp_",
		"github_pat_",
		"sk-proj-",
		"sk_live_",
		"aws_secret_access_key",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func normalizeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.Join(strings.Fields(value), " ")
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:8])
}

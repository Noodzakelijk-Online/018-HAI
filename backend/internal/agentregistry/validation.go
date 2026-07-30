package agentregistry

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxIdentityLength = 256
	maxTextLength     = 2048
)

var (
	identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/@-]{0,255}$`)
	secretPattern     = regexp.MustCompile(`(?i)(password|passwd|secret|api[_-]?key|access[_-]?token|refresh[_-]?token|authorization)\s*[:=]\s*\S+|bearer\s+\S+|sk-[a-z0-9_-]{8,}`)
)

func ValidateAgent(agent Agent, now time.Time) error {
	if agent.ContractVersion != ContractVersion {
		return fmt.Errorf("unsupported agent contract version %d", agent.ContractVersion)
	}
	if err := validateIdentifier("agent id", agent.ID); err != nil {
		return err
	}
	if err := validateIdentity(agent.OwnerIdentity); err != nil {
		return err
	}
	if err := validateText("agent name", agent.Name, true); err != nil {
		return err
	}
	if !validAgentType(agent.Type) {
		return fmt.Errorf("invalid agent type %q", agent.Type)
	}
	if err := validateIdentifier("runtime adapter id", agent.Runtime.ID); err != nil {
		return err
	}
	if err := validateIdentifier("runtime adapter type", agent.Runtime.Type); err != nil {
		return err
	}
	if _, err := parseVersion(agent.Runtime.ProtocolVersion); err != nil {
		return fmt.Errorf("runtime protocol version: %w", err)
	}
	if agent.AuthorityCeiling < 0 || agent.AuthorityCeiling > 10 {
		return fmt.Errorf("authority ceiling must be between 0 and 10")
	}
	if agent.AutonomyCeiling < 0 || agent.AutonomyCeiling > 10 {
		return fmt.Errorf("autonomy ceiling must be between 0 and 10")
	}
	if !validLifecycle(agent.State) {
		return fmt.Errorf("invalid lifecycle state %q", agent.State)
	}
	if err := validateCapabilities(agent.Capabilities); err != nil {
		return err
	}
	if err := validateAllowlist("tool allowlist", agent.ToolAllowlist, false); err != nil {
		return err
	}
	if err := validateAllowlist("data allowlist", agent.DataAllowlist, false); err != nil {
		return err
	}
	if err := validateAllowlist("folder allowlist", agent.FolderAllowlist, true); err != nil {
		return err
	}
	if !validHealth(agent.Health.Status) {
		return fmt.Errorf("invalid health status %q", agent.Health.Status)
	}
	if err := validateText("health reason", agent.Health.Reason, false); err != nil {
		return err
	}
	if agent.Health.FreshFor < 0 {
		return fmt.Errorf("health freshness cannot be negative")
	}
	if agent.Availability.ActiveAssignments < 0 || agent.Availability.MaxConcurrent < 1 ||
		agent.Availability.ActiveAssignments > agent.Availability.MaxConcurrent {
		return fmt.Errorf("invalid agent availability")
	}
	if agent.Performance.EstimatedCostEUR < 0 || agent.Performance.P95LatencyMs < 0 {
		return fmt.Errorf("performance values cannot be negative")
	}
	if !validLocality(agent.Performance.Locality) {
		return fmt.Errorf("invalid locality %q", agent.Performance.Locality)
	}
	if agent.Reliability.MeanLatencyMs < 0 {
		return fmt.Errorf("reliability latency cannot be negative")
	}
	if agent.Reliability.ConsecutiveFailures > agent.Reliability.Failures {
		return fmt.Errorf("consecutive failures cannot exceed total failures")
	}
	if agent.Revision == 0 {
		return fmt.Errorf("agent revision must be positive")
	}
	if agent.CreatedAt.IsZero() || agent.UpdatedAt.IsZero() || agent.UpdatedAt.Before(agent.CreatedAt) {
		return fmt.Errorf("invalid agent timestamps")
	}
	if agent.CreatedAt.After(now.Add(time.Minute)) || agent.UpdatedAt.After(now.Add(time.Minute)) {
		return fmt.Errorf("agent timestamps cannot be in the future")
	}
	return nil
}

func ValidateAssignmentRequest(request AssignmentRequest) error {
	if err := validateIdentity(request.OwnerIdentity); err != nil {
		return err
	}
	if err := validateIdentifier("task id", request.TaskID); err != nil {
		return err
	}
	if len(request.Capabilities) == 0 {
		return fmt.Errorf("at least one capability is required")
	}
	seen := map[string]struct{}{}
	for _, required := range request.Capabilities {
		if err := validateIdentifier("capability id", required.ID); err != nil {
			return err
		}
		key := strings.ToLower(required.ID)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate capability %q", required.ID)
		}
		seen[key] = struct{}{}
		if err := validateVersionRange(required.MinVersion, required.MaxVersion); err != nil {
			return fmt.Errorf("capability %q: %w", required.ID, err)
		}
		if err := validateAllowlist("required operations", required.Operations, false); err != nil {
			return err
		}
	}
	if err := validateCompatibility(request.Compatibility); err != nil {
		return err
	}
	if request.RequiredAuthority < 0 || request.RequiredAuthority > 10 ||
		request.RequiredAutonomy < 0 || request.RequiredAutonomy > 10 ||
		request.PolicyMaxAuthority < 0 || request.PolicyMaxAuthority > 10 ||
		request.PolicyMaxAutonomy < 0 || request.PolicyMaxAutonomy > 10 {
		return fmt.Errorf("authority and autonomy values must be between 0 and 10")
	}
	if request.RequiredAuthority > request.PolicyMaxAuthority {
		return fmt.Errorf("required authority exceeds policy maximum")
	}
	if request.RequiredAutonomy > request.PolicyMaxAutonomy {
		return fmt.Errorf("required autonomy exceeds policy maximum")
	}
	for _, agentType := range request.AllowedAgentTypes {
		if !validAgentType(agentType) {
			return fmt.Errorf("invalid allowed agent type %q", agentType)
		}
	}
	if request.MaxEstimatedCostEUR != nil && *request.MaxEstimatedCostEUR < 0 {
		return fmt.Errorf("maximum estimated cost cannot be negative")
	}
	if err := validateAllowlist("required tools", request.RequiredTools, false); err != nil {
		return err
	}
	if err := validateAllowlist("required data", request.RequiredData, false); err != nil {
		return err
	}
	return validateAllowlist("required folders", request.RequiredFolders, true)
}

func validateCapabilities(values []CapabilityDeclaration) error {
	if len(values) == 0 {
		return fmt.Errorf("at least one capability declaration is required")
	}
	seen := map[string]struct{}{}
	for _, value := range values {
		if err := validateIdentifier("capability id", value.ID); err != nil {
			return err
		}
		key := strings.ToLower(value.ID)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate capability %q", value.ID)
		}
		seen[key] = struct{}{}
		if _, err := parseVersion(value.Version); err != nil {
			return fmt.Errorf("capability %q version: %w", value.ID, err)
		}
		if err := validateAllowlist("capability operations", value.Operations, false); err != nil {
			return err
		}
		if err := validateText("capability description", value.Description, false); err != nil {
			return err
		}
	}
	return nil
}

func validateCompatibility(value CompatibilityRequirement) error {
	if value.RuntimeAdapterID != "" {
		if err := validateIdentifier("runtime adapter requirement", value.RuntimeAdapterID); err != nil {
			return err
		}
	}
	if value.RuntimeType != "" {
		if err := validateIdentifier("runtime type requirement", value.RuntimeType); err != nil {
			return err
		}
	}
	return validateVersionRange(value.MinProtocolVersion, value.MaxProtocolVersion)
}

func validateVersionRange(minimum, maximum string) error {
	if minimum == "" && maximum == "" {
		return nil
	}
	var min, max semanticVersion
	var err error
	if minimum != "" {
		min, err = parseVersion(minimum)
		if err != nil {
			return fmt.Errorf("invalid minimum version: %w", err)
		}
	}
	if maximum != "" {
		max, err = parseVersion(maximum)
		if err != nil {
			return fmt.Errorf("invalid maximum version: %w", err)
		}
	}
	if minimum != "" && maximum != "" && compareVersions(min, max) > 0 {
		return fmt.Errorf("minimum version exceeds maximum version")
	}
	return nil
}

func validateIdentity(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxIdentityLength || secretPattern.MatchString(value) {
		return fmt.Errorf("invalid owner identity")
	}
	return nil
}

func validateIdentifier(name, value string) error {
	if !identifierPattern.MatchString(strings.TrimSpace(value)) || secretPattern.MatchString(value) {
		return fmt.Errorf("invalid %s", name)
	}
	return nil
}

func validateText(name, value string, required bool) error {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > maxTextLength {
		return fmt.Errorf("%s exceeds maximum length", name)
	}
	if secretPattern.MatchString(value) {
		return fmt.Errorf("%s contains secret material", name)
	}
	return nil
}

func validateAllowlist(name string, values []string, paths bool) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > maxTextLength || secretPattern.MatchString(value) {
			return fmt.Errorf("invalid %s entry", name)
		}
		if parsed, err := url.Parse(value); err == nil && parsed.User != nil {
			return fmt.Errorf("%s entry contains credentials", name)
		}
		key := strings.ToLower(value)
		if paths {
			key = strings.ReplaceAll(key, "\\", "/")
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate %s entry %q", name, value)
		}
		seen[key] = struct{}{}
	}
	return nil
}

type semanticVersion struct {
	major int
	minor int
	patch int
}

func parseVersion(value string) (semanticVersion, error) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "v"))
	parts := strings.Split(value, ".")
	if len(parts) < 1 || len(parts) > 3 || value == "" {
		return semanticVersion{}, fmt.Errorf("version must be numeric major[.minor[.patch]]")
	}
	numbers := [3]int{}
	for index, part := range parts {
		if part == "" {
			return semanticVersion{}, fmt.Errorf("version contains an empty component")
		}
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return semanticVersion{}, fmt.Errorf("version component %q is invalid", part)
		}
		numbers[index] = number
	}
	return semanticVersion{major: numbers[0], minor: numbers[1], patch: numbers[2]}, nil
}

func compareVersions(left, right semanticVersion) int {
	leftParts := []int{left.major, left.minor, left.patch}
	rightParts := []int{right.major, right.minor, right.patch}
	for index := range leftParts {
		if leftParts[index] < rightParts[index] {
			return -1
		}
		if leftParts[index] > rightParts[index] {
			return 1
		}
	}
	return 0
}

func normalizeUnique(values []string) []string {
	seen := map[string]string{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; !exists && trimmed != "" {
			seen[key] = trimmed
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, seen[key])
	}
	return result
}

func validAgentType(value AgentType) bool {
	switch value {
	case AgentTypePlanner, AgentTypeResearcher, AgentTypeExecutor, AgentTypeReviewer, AgentTypeSpecialist, AgentTypeOrchestrator:
		return true
	default:
		return false
	}
}

func validLifecycle(value LifecycleState) bool {
	switch value {
	case StateRegistered, StateEnabled, StateDraining, StateDisabled, StateQuarantined:
		return true
	default:
		return false
	}
}

func validHealth(value HealthStatus) bool {
	switch value {
	case HealthUnknown, HealthHealthy, HealthDegraded, HealthUnhealthy:
		return true
	default:
		return false
	}
}

func validLocality(value Locality) bool {
	return value == LocalityLocal || value == LocalityLAN || value == LocalityCloud
}

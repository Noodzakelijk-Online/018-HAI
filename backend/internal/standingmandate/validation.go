package standingmandate

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	minimumApprovalEvidenceAgeSeconds = 1
	maximumApprovalEvidenceAgeSeconds = 24 * 60 * 60
)

func validateCreateRequest(request CreateRequest, now time.Time) error {
	if strings.TrimSpace(request.OwnerIdentity) == "" {
		return fmt.Errorf("owner identity is required")
	}
	if strings.TrimSpace(request.Name) == "" {
		return fmt.Errorf("mandate name is required")
	}
	if strings.TrimSpace(request.Purpose) == "" {
		return fmt.Errorf("mandate purpose is required")
	}
	if strings.TrimSpace(request.CreatedBy) == "" {
		return fmt.Errorf("creator identity is required")
	}
	if request.AutonomyCeiling < 0 || request.AutonomyCeiling > 10 {
		return fmt.Errorf("autonomy ceiling must be between 0 and 10")
	}
	if len(request.Scopes) == 0 {
		return fmt.Errorf("at least one bounded scope is required")
	}
	scopeIDs := make(map[string]struct{}, len(request.Scopes))
	for index, scope := range request.Scopes {
		if err := validateScope(scope); err != nil {
			return fmt.Errorf("scope %d: %w", index, err)
		}
		normalizedID := normalize(scope.ID)
		if _, exists := scopeIDs[normalizedID]; exists {
			return fmt.Errorf("scope IDs must be unique")
		}
		scopeIDs[normalizedID] = struct{}{}
	}
	if err := validateApprovalPolicy(request.ApprovalPolicy); err != nil {
		return err
	}
	stopIDs := make(map[string]struct{}, len(request.StopConditions))
	for index, condition := range request.StopConditions {
		if err := validateStopCondition(condition); err != nil {
			return fmt.Errorf("stop condition %d: %w", index, err)
		}
		normalizedID := normalize(condition.ID)
		if _, exists := stopIDs[normalizedID]; exists {
			return fmt.Errorf("stop condition IDs must be unique")
		}
		stopIDs[normalizedID] = struct{}{}
	}
	if request.ExpiresAt != nil && !request.ExpiresAt.After(now) {
		return fmt.Errorf("mandate expiry must be in the future")
	}
	return nil
}

func validateScope(scope Scope) error {
	if strings.TrimSpace(scope.ID) == "" {
		return fmt.Errorf("scope ID is required")
	}
	if len(scope.Actions) == 0 {
		return fmt.Errorf("at least one action is required")
	}
	for _, action := range scope.Actions {
		if err := validateBoundedValue("action", action); err != nil {
			return err
		}
	}
	if len(scope.Resources) == 0 && len(scope.Projects) == 0 && len(scope.Domains) == 0 && len(scope.Tools) == 0 {
		return fmt.Errorf("scope must include a resource, project, domain, or tool boundary")
	}
	for _, resource := range scope.Resources {
		if err := validateBoundedValue("resource type", resource.Type); err != nil {
			return err
		}
		for _, id := range resource.IDs {
			if err := validateBoundedValue("resource ID", id); err != nil {
				return err
			}
		}
	}
	for _, values := range [][]string{scope.Projects, scope.Domains, scope.Tools} {
		for _, value := range values {
			if err := validateBoundedValue("scope value", value); err != nil {
				return err
			}
		}
	}
	if scope.MaximumRisk != "" && !scope.MaximumRisk.valid() {
		return fmt.Errorf("scope maximum risk is invalid")
	}
	return nil
}

func validateApprovalPolicy(policy ApprovalPolicy) error {
	if !policy.Mode.valid() {
		return fmt.Errorf("approval policy mode is invalid")
	}
	if policy.AutonomyThreshold < 0 || policy.AutonomyThreshold > 10 {
		return fmt.Errorf("approval autonomy threshold must be between 0 and 10")
	}
	if policy.Mode == ApprovalAtOrAboveAutonomy && policy.AutonomyThreshold == 0 {
		return fmt.Errorf("approval autonomy threshold must be greater than zero")
	}
	if policy.Mode == ApprovalForRiskOrAction && len(policy.RiskLevels) == 0 && len(policy.Actions) == 0 {
		return fmt.Errorf("risk-or-action approval policy needs at least one trigger")
	}
	for _, risk := range policy.RiskLevels {
		if !risk.valid() {
			return fmt.Errorf("approval policy risk level is invalid")
		}
	}
	for _, action := range policy.Actions {
		if err := validateBoundedValue("approval action", action); err != nil {
			return err
		}
	}
	for _, role := range policy.ApproverRoles {
		if err := validateBoundedValue("approver role", role); err != nil {
			return err
		}
	}
	if policy.MaximumEvidenceAgeSeconds < 0 ||
		policy.MaximumEvidenceAgeSeconds > maximumApprovalEvidenceAgeSeconds {
		return fmt.Errorf("maximum approval evidence age must be between 0 and %d seconds", maximumApprovalEvidenceAgeSeconds)
	}
	return nil
}

func validateStopCondition(condition StopCondition) error {
	if strings.TrimSpace(condition.ID) == "" || strings.TrimSpace(condition.Description) == "" {
		return fmt.Errorf("ID and description are required")
	}
	if strings.TrimSpace(condition.FactKey) == "" {
		return fmt.Errorf("fact key is required")
	}
	if !condition.Operator.valid() {
		return fmt.Errorf("operator is invalid")
	}
	if condition.Effect != StopDeny && condition.Effect != StopRequireApproval {
		return fmt.Errorf("effect is invalid")
	}
	if condition.Operator != StopPresent && condition.Operator != StopAbsent &&
		strings.TrimSpace(condition.ExpectedValue) == "" {
		return fmt.Errorf("expected value is required for %s", condition.Operator)
	}
	if condition.Operator == StopGreaterOrEqual || condition.Operator == StopLessOrEqual {
		if _, err := strconv.ParseFloat(strings.TrimSpace(condition.ExpectedValue), 64); err != nil {
			return fmt.Errorf("expected value must be numeric for %s", condition.Operator)
		}
	}
	return nil
}

func validateActionRequest(request ActionRequest) error {
	if strings.TrimSpace(request.OwnerIdentity) == "" ||
		strings.TrimSpace(request.ActorIdentity) == "" {
		return fmt.Errorf("owner and actor identities are required")
	}
	if err := validateBoundedValue("action", request.Action); err != nil {
		return err
	}
	if err := validateBoundedValue("resource type", request.ResourceType); err != nil {
		return err
	}
	if request.RequestedAutonomy < 0 || request.RequestedAutonomy > 10 {
		return fmt.Errorf("requested autonomy must be between 0 and 10")
	}
	if !request.Risk.valid() {
		return fmt.Errorf("risk level is invalid")
	}
	return nil
}

func validateBoundedValue(label, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if value == "*" || strings.ContainsAny(value, "\r\n\x00") || len(value) > 256 {
		return fmt.Errorf("%s must be an exact bounded value", label)
	}
	return nil
}

func (value RiskLevel) valid() bool {
	switch value {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
		return true
	default:
		return false
	}
}

func (value ApprovalMode) valid() bool {
	switch value {
	case ApprovalNever, ApprovalAlways, ApprovalAtOrAboveAutonomy, ApprovalForRiskOrAction:
		return true
	default:
		return false
	}
}

func (value StopOperator) valid() bool {
	switch value {
	case StopEquals, StopNotEquals, StopPresent, StopAbsent, StopGreaterOrEqual, StopLessOrEqual:
		return true
	default:
		return false
	}
}

func riskRank(value RiskLevel) int {
	switch value {
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	case RiskCritical:
		return 4
	default:
		return 0
	}
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

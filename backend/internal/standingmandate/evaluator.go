package standingmandate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Service) Authorize(
	ctx context.Context,
	mandateID uuid.UUID,
	request ActionRequest,
) (*AuthorizationDecision, error) {
	now := s.now().UTC()
	if request.RequestedAt.IsZero() {
		request.RequestedAt = now
	} else {
		request.RequestedAt = request.RequestedAt.UTC()
	}
	if err := validateActionRequest(request); err != nil {
		return nil, err
	}
	mandate, err := s.repository.Get(ctx, strings.TrimSpace(request.OwnerIdentity), mandateID)
	if err != nil {
		return nil, err
	}
	decision, err := evaluate(*mandate, request, now)
	if err != nil {
		return nil, err
	}
	if err := s.repository.CreateDecision(ctx, *decision); err != nil {
		return nil, fmt.Errorf("persist authorization decision: %w", err)
	}
	return decision, nil
}

func evaluate(mandate StandingMandate, request ActionRequest, now time.Time) (*AuthorizationDecision, error) {
	requestDigest, err := digest(normalizedActionRequest(request))
	if err != nil {
		return nil, fmt.Errorf("digest action request: %w", err)
	}
	mandateDigest, err := digest(normalizedMandate(mandate))
	if err != nil {
		return nil, fmt.Errorf("digest standing mandate: %w", err)
	}
	decision := &AuthorizationDecision{
		ID:                uuid.New(),
		MandateID:         mandate.ID,
		OwnerIdentity:     request.OwnerIdentity,
		ActorIdentity:     request.ActorIdentity,
		Action:            request.Action,
		Outcome:           DecisionDenied,
		EffectiveAutonomy: min(mandate.AutonomyCeiling, request.RequestedAutonomy),
		EvaluatedAt:       now,
		Evidence: DecisionEvidence{
			RequestDigest:   requestDigest,
			MandateDigest:   mandateDigest,
			MandateRevision: mandate.Revision,
			SourceReferences: mergeValues(
				mandate.SourceReferences,
				request.SourceReferences,
			),
			Trace: []DecisionTrace{},
		},
	}

	if mandate.Status != StatusActive {
		return finishDecision(decision, DecisionDenied, "mandate is not active", "mandate.inactive")
	}
	if mandate.ExpiresAt != nil && !now.Before(mandate.ExpiresAt.UTC()) {
		return finishDecision(decision, DecisionDenied, "mandate has expired", "mandate.expired")
	}
	if request.RequestedAt.After(now.Add(5 * time.Minute)) {
		return finishDecision(decision, DecisionDenied, "action request time is in the future", "request.future")
	}
	if request.RequestedAutonomy > mandate.AutonomyCeiling {
		return finishDecision(decision, DecisionDenied, "requested autonomy exceeds mandate ceiling", "autonomy.exceeds_ceiling")
	}

	matched := matchingScopeIDs(mandate.Scopes, request)
	if len(matched) == 0 {
		return finishDecision(decision, DecisionDenied, "action is outside every mandate scope", "scope.no_match")
	}
	decision.Evidence.MatchedScopeIDs = matched
	decision.Evidence.Trace = append(decision.Evidence.Trace, DecisionTrace{
		Code:    "scope.matched",
		Message: fmt.Sprintf("matched %d bounded scope(s)", len(matched)),
	})

	triggered := evaluateStops(mandate.StopConditions, request.Facts)
	decision.Evidence.TriggeredStops = triggered
	for _, stop := range triggered {
		decision.Evidence.Trace = append(decision.Evidence.Trace, DecisionTrace{
			Code:    "stop." + string(stop.Effect),
			Message: stop.ConditionID + ": " + stop.Reason,
		})
		if stop.Effect == StopDeny {
			return finishDecision(decision, DecisionDenied, "a mandate stop condition blocks this action", "stop.denied")
		}
	}

	approvalRequired := request.UpstreamApprovalRequired ||
		approvalRequiredByPolicy(mandate.ApprovalPolicy, request) ||
		containsApprovalStop(triggered)
	decision.ApprovalRequired = approvalRequired
	if approvalRequired {
		approved, reason := approvalSatisfies(mandate.ApprovalPolicy, request, requestDigest, now)
		decision.ApprovalSatisfied = approved
		if request.Approval != nil {
			decision.Evidence.ApprovalEvidenceID = strings.TrimSpace(request.Approval.ID)
		}
		if !approved {
			decision.Evidence.Trace = append(decision.Evidence.Trace, DecisionTrace{
				Code:    "approval.unsatisfied",
				Message: reason,
			})
			return finishDecision(decision, DecisionRequiresApproval, reason, "approval.required")
		}
		decision.Evidence.Trace = append(decision.Evidence.Trace, DecisionTrace{
			Code:    "approval.satisfied",
			Message: "action-bound approval evidence satisfies the mandate policy",
		})
	}

	return finishDecision(decision, DecisionAuthorized, "action is authorized by the active standing mandate", "decision.authorized")
}

func finishDecision(
	decision *AuthorizationDecision,
	outcome DecisionOutcome,
	reason string,
	code string,
) (*AuthorizationDecision, error) {
	decision.Outcome = outcome
	decision.Reason = reason
	decision.Evidence.Trace = append(decision.Evidence.Trace, DecisionTrace{
		Code:    code,
		Message: reason,
	})
	payload := struct {
		ID                string           `json:"id"`
		MandateID         string           `json:"mandateId"`
		Outcome           DecisionOutcome  `json:"outcome"`
		Reason            string           `json:"reason"`
		EffectiveAutonomy int              `json:"effectiveAutonomy"`
		ApprovalRequired  bool             `json:"approvalRequired"`
		ApprovalSatisfied bool             `json:"approvalSatisfied"`
		EvaluatedAt       time.Time        `json:"evaluatedAt"`
		Evidence          DecisionEvidence `json:"evidence"`
	}{
		ID:                decision.ID.String(),
		MandateID:         decision.MandateID.String(),
		Outcome:           decision.Outcome,
		Reason:            decision.Reason,
		EffectiveAutonomy: decision.EffectiveAutonomy,
		ApprovalRequired:  decision.ApprovalRequired,
		ApprovalSatisfied: decision.ApprovalSatisfied,
		EvaluatedAt:       decision.EvaluatedAt,
		Evidence:          decision.Evidence,
	}
	payload.Evidence.DecisionDigest = ""
	value, err := digest(payload)
	if err != nil {
		return nil, fmt.Errorf("digest authorization decision: %w", err)
	}
	decision.Evidence.DecisionDigest = value
	return decision, nil
}

func matchingScopeIDs(scopes []Scope, request ActionRequest) []string {
	matched := make([]string, 0)
	for _, scope := range scopes {
		if scopeMatches(scope, request) {
			matched = append(matched, scope.ID)
		}
	}
	sort.Strings(matched)
	return matched
}

func scopeMatches(scope Scope, request ActionRequest) bool {
	if !containsNormalized(scope.Actions, request.Action) {
		return false
	}
	if scope.MaximumRisk != "" && riskRank(request.Risk) > riskRank(scope.MaximumRisk) {
		return false
	}
	if len(scope.Projects) > 0 && !containsNormalized(scope.Projects, request.ProjectKey) {
		return false
	}
	if len(scope.Domains) > 0 && !containsNormalized(scope.Domains, request.Domain) {
		return false
	}
	if len(scope.Tools) > 0 && !containsNormalized(scope.Tools, request.ToolID) {
		return false
	}
	if len(scope.Resources) == 0 {
		return true
	}
	for _, resource := range scope.Resources {
		if normalize(resource.Type) != normalize(request.ResourceType) {
			continue
		}
		if len(resource.IDs) == 0 || containsNormalized(resource.IDs, request.ResourceID) {
			return true
		}
	}
	return false
}

func evaluateStops(conditions []StopCondition, facts map[string]string) []TriggeredStop {
	triggered := make([]TriggeredStop, 0)
	for _, condition := range conditions {
		actual, exists := factValue(facts, condition.FactKey)
		if !exists && condition.Required {
			triggered = append(triggered, TriggeredStop{
				ConditionID: condition.ID,
				Effect:      condition.Effect,
				Reason:      "required fact is unavailable",
			})
			continue
		}
		if stopMatches(condition, actual, exists) {
			triggered = append(triggered, TriggeredStop{
				ConditionID: condition.ID,
				Effect:      condition.Effect,
				Reason:      condition.Description,
			})
		}
	}
	sort.Slice(triggered, func(i, j int) bool {
		return triggered[i].ConditionID < triggered[j].ConditionID
	})
	return triggered
}

func stopMatches(condition StopCondition, actual string, exists bool) bool {
	switch condition.Operator {
	case StopPresent:
		return exists
	case StopAbsent:
		return !exists
	case StopEquals:
		return exists && normalize(actual) == normalize(condition.ExpectedValue)
	case StopNotEquals:
		return exists && normalize(actual) != normalize(condition.ExpectedValue)
	case StopGreaterOrEqual, StopLessOrEqual:
		if !exists {
			return false
		}
		actualNumber, actualErr := strconv.ParseFloat(strings.TrimSpace(actual), 64)
		expectedNumber, expectedErr := strconv.ParseFloat(strings.TrimSpace(condition.ExpectedValue), 64)
		if actualErr != nil || expectedErr != nil {
			return condition.Required
		}
		if condition.Operator == StopGreaterOrEqual {
			return actualNumber >= expectedNumber
		}
		return actualNumber <= expectedNumber
	default:
		return true
	}
}

func approvalRequiredByPolicy(policy ApprovalPolicy, request ActionRequest) bool {
	switch policy.Mode {
	case ApprovalAlways:
		return true
	case ApprovalAtOrAboveAutonomy:
		return request.RequestedAutonomy >= policy.AutonomyThreshold
	case ApprovalForRiskOrAction:
		return containsRisk(policy.RiskLevels, request.Risk) ||
			containsNormalized(policy.Actions, request.Action)
	default:
		return false
	}
}

func approvalSatisfies(
	policy ApprovalPolicy,
	request ActionRequest,
	actionDigest string,
	now time.Time,
) (bool, string) {
	evidence := request.Approval
	if evidence == nil {
		return false, "action-bound approval evidence is required"
	}
	if evidence.Revoked {
		return false, "approval evidence has been revoked"
	}
	if strings.TrimSpace(evidence.ID) == "" ||
		strings.TrimSpace(evidence.ApprovedBy) == "" ||
		strings.TrimSpace(evidence.Source) == "" {
		return false, "approval evidence identity, approver, and source are required"
	}
	if !strings.EqualFold(strings.TrimSpace(evidence.ActionDigest), actionDigest) {
		return false, "approval evidence is bound to a different action"
	}
	if evidence.ApprovedAt.IsZero() || evidence.ExpiresAt.IsZero() ||
		evidence.ApprovedAt.After(now.Add(5*time.Second)) ||
		!now.Before(evidence.ExpiresAt) {
		return false, "approval evidence is expired or has an invalid time window"
	}
	if policy.MaximumEvidenceAgeSeconds > 0 &&
		now.Sub(evidence.ApprovedAt) > time.Duration(policy.MaximumEvidenceAgeSeconds)*time.Second {
		return false, "approval evidence is older than the mandate policy permits"
	}
	if len(policy.ApproverRoles) > 0 && !intersectsNormalized(policy.ApproverRoles, evidence.ApproverRoles) {
		return false, "approval evidence does not contain a permitted approver role"
	}
	return true, ""
}

func containsApprovalStop(stops []TriggeredStop) bool {
	for _, stop := range stops {
		if stop.Effect == StopRequireApproval {
			return true
		}
	}
	return false
}

func factValue(facts map[string]string, key string) (string, bool) {
	for candidate, value := range facts {
		if normalize(candidate) == normalize(key) {
			return value, true
		}
	}
	return "", false
}

func containsNormalized(values []string, candidate string) bool {
	candidate = normalize(candidate)
	if candidate == "" {
		return false
	}
	for _, value := range values {
		if normalize(value) == candidate {
			return true
		}
	}
	return false
}

func containsRisk(values []RiskLevel, candidate RiskLevel) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func intersectsNormalized(left, right []string) bool {
	for _, value := range left {
		if containsNormalized(right, value) {
			return true
		}
	}
	return false
}

func digest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func normalizedActionRequest(request ActionRequest) ActionRequest {
	cloned := request
	cloned.OwnerIdentity = strings.TrimSpace(cloned.OwnerIdentity)
	cloned.ActorIdentity = strings.TrimSpace(cloned.ActorIdentity)
	cloned.Action = normalize(cloned.Action)
	cloned.ResourceType = normalize(cloned.ResourceType)
	cloned.ResourceID = strings.TrimSpace(cloned.ResourceID)
	cloned.ProjectKey = normalize(cloned.ProjectKey)
	cloned.Domain = normalize(cloned.Domain)
	cloned.ToolID = normalize(cloned.ToolID)
	cloned.RequestedAt = cloned.RequestedAt.UTC()
	cloned.SourceReferences = cleanValues(cloned.SourceReferences)
	cloned.Facts = cloneNormalizedMap(cloned.Facts)
	cloned.Approval = nil
	return cloned
}

func normalizedMandate(mandate StandingMandate) StandingMandate {
	cloned := cloneMandate(mandate)
	sort.Slice(cloned.Scopes, func(i, j int) bool {
		return normalize(cloned.Scopes[i].ID) < normalize(cloned.Scopes[j].ID)
	})
	for index := range cloned.Scopes {
		cloned.Scopes[index].ID = normalize(cloned.Scopes[index].ID)
		cloned.Scopes[index].Actions = cleanValues(cloned.Scopes[index].Actions)
		cloned.Scopes[index].Projects = cleanValues(cloned.Scopes[index].Projects)
		cloned.Scopes[index].Domains = cleanValues(cloned.Scopes[index].Domains)
		cloned.Scopes[index].Tools = cleanValues(cloned.Scopes[index].Tools)
	}
	cloned.SourceReferences = cleanValues(cloned.SourceReferences)
	return cloned
}

func cloneMandate(value StandingMandate) StandingMandate {
	cloned := value
	cloned.Scopes = cloneScopes(value.Scopes)
	cloned.ApprovalPolicy = cloneApprovalPolicy(value.ApprovalPolicy)
	cloned.StopConditions = append([]StopCondition(nil), value.StopConditions...)
	cloned.SourceReferences = append([]string(nil), value.SourceReferences...)
	cloned.ActivatedAt = cloneTime(value.ActivatedAt)
	cloned.ExpiresAt = cloneTime(value.ExpiresAt)
	cloned.RevokedAt = cloneTime(value.RevokedAt)
	return cloned
}

func cloneMandatePointer(value *StandingMandate) *StandingMandate {
	if value == nil {
		return nil
	}
	cloned := cloneMandate(*value)
	return &cloned
}

func cloneScopes(values []Scope) []Scope {
	cloned := make([]Scope, len(values))
	for index, value := range values {
		cloned[index] = value
		cloned[index].Actions = append([]string(nil), value.Actions...)
		cloned[index].Projects = append([]string(nil), value.Projects...)
		cloned[index].Domains = append([]string(nil), value.Domains...)
		cloned[index].Tools = append([]string(nil), value.Tools...)
		cloned[index].Resources = make([]ResourceScope, len(value.Resources))
		for resourceIndex, resource := range value.Resources {
			cloned[index].Resources[resourceIndex] = resource
			cloned[index].Resources[resourceIndex].IDs = append([]string(nil), resource.IDs...)
		}
	}
	return cloned
}

func cloneApprovalPolicy(value ApprovalPolicy) ApprovalPolicy {
	cloned := value
	cloned.RiskLevels = append([]RiskLevel(nil), value.RiskLevels...)
	cloned.Actions = append([]string(nil), value.Actions...)
	cloned.ApproverRoles = append([]string(nil), value.ApproverRoles...)
	return cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func cloneNormalizedMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	cloned := make(map[string]string, len(value))
	for key, entry := range value {
		cloned[normalize(key)] = strings.TrimSpace(entry)
	}
	return cloned
}

func cleanValues(values []string) []string {
	unique := make(map[string]string, len(values))
	for _, value := range values {
		if cleaned := strings.TrimSpace(value); cleaned != "" {
			unique[normalize(cleaned)] = cleaned
		}
	}
	result := make([]string, 0, len(unique))
	for _, value := range unique {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		return normalize(result[i]) < normalize(result[j])
	})
	return result
}

func mergeValues(groups ...[]string) []string {
	merged := make([]string, 0)
	for _, values := range groups {
		merged = append(merged, values...)
	}
	return cleanValues(merged)
}

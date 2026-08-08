package resourceplanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"automation-hub-backend/internal/safety"
)

const (
	maxTasks        = 500
	maxDependencies = 100
	maxResources    = 100
	maxWindows      = 5000
	maxMinutes      = int64(10 * 365 * 24 * 60)
	maxCapacity     = int64(1_000_000_000)
)

func normalizeAndValidate(request Request) (Request, error) {
	request = cloneRequest(request)
	request.OwnerIdentity = strings.TrimSpace(request.OwnerIdentity)
	if request.OwnerIdentity == "" || len(request.OwnerIdentity) > 320 || hasUnsafeText(request.OwnerIdentity) {
		return Request{}, fmt.Errorf("owner identity is required and must not contain secret material")
	}
	ownerDigest := sha256.Sum256([]byte(request.OwnerIdentity))
	request.OwnerScopeDigest = hex.EncodeToString(ownerDigest[:])
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID != "" && !validOpaqueID(request.WorkspaceID) {
		return Request{}, fmt.Errorf("workspace id must be an opaque identifier")
	}
	request.PlanID = strings.TrimSpace(request.PlanID)
	if !validOpaqueID(request.PlanID) {
		return Request{}, fmt.Errorf("plan id must be an opaque identifier")
	}
	if request.AsOf.IsZero() {
		return Request{}, fmt.Errorf("asOf is required for deterministic audit output")
	}
	request.AsOf = minuteUTC(request.AsOf)
	request.HorizonStart = minuteUTC(request.HorizonStart)
	request.HorizonEnd = minuteUTC(request.HorizonEnd)
	if request.HorizonStart.IsZero() || !request.HorizonEnd.After(request.HorizonStart) {
		return Request{}, fmt.Errorf("planning horizon must have a start before its end")
	}
	if minutesBetween(request.HorizonStart, request.HorizonEnd) > maxMinutes {
		return Request{}, fmt.Errorf("planning horizon exceeds %d minutes", maxMinutes)
	}
	if request.AsOf.After(request.HorizonEnd) {
		return Request{}, fmt.Errorf("asOf must not be after the planning horizon")
	}
	if request.DurationMode == "" {
		request.DurationMode = ExpectedDuration
	}
	if request.DurationMode != ExpectedDuration && request.DurationMode != ConservativeDuration {
		return Request{}, fmt.Errorf("duration mode must be expected or conservative")
	}
	if len(request.Tasks) == 0 || len(request.Tasks) > maxTasks {
		return Request{}, fmt.Errorf("tasks must contain 1 to %d items", maxTasks)
	}
	if len(request.Availability) > maxWindows {
		return Request{}, fmt.Errorf("availability may contain at most %d windows", maxWindows)
	}
	if err := validateBudget(request.Budget, "budget"); err != nil {
		return Request{}, err
	}
	if err := validateApprovalPolicy(request.ApprovalPolicy); err != nil {
		return Request{}, err
	}

	seen := make(map[string]struct{}, len(request.Tasks))
	for index := range request.Tasks {
		task := &request.Tasks[index]
		task.ID = strings.TrimSpace(task.ID)
		if !validOpaqueID(task.ID) {
			return Request{}, fmt.Errorf("task %d has an invalid opaque id", index)
		}
		if _, exists := seen[task.ID]; exists {
			return Request{}, fmt.Errorf("task id %s is duplicated", task.ID)
		}
		seen[task.ID] = struct{}{}
		if err := validateDuration(task.ID, task.Duration); err != nil {
			return Request{}, err
		}
		if task.EarliestStart != nil {
			value := minuteUTC(*task.EarliestStart)
			task.EarliestStart = &value
			if value.Before(request.HorizonStart) || !value.Before(request.HorizonEnd) {
				return Request{}, fmt.Errorf("task %s earliest start is outside the planning horizon", task.ID)
			}
		}
		if task.Deadline != nil {
			value := minuteUTC(*task.Deadline)
			task.Deadline = &value
			if !value.After(request.HorizonStart) || value.After(request.HorizonEnd) {
				return Request{}, fmt.Errorf("task %s deadline is outside the planning horizon", task.ID)
			}
			if task.EarliestStart != nil && !value.After(*task.EarliestStart) {
				return Request{}, fmt.Errorf("task %s deadline must follow its earliest start", task.ID)
			}
		}
		if task.DeadlineKind == "" {
			task.DeadlineKind = HardDeadline
		}
		if task.DeadlineKind != HardDeadline && task.DeadlineKind != SoftDeadline {
			return Request{}, fmt.Errorf("task %s has an invalid deadline kind", task.ID)
		}
		if task.Priority < 0 || task.Priority > 100 {
			return Request{}, fmt.Errorf("task %s priority must be between 0 and 100", task.ID)
		}
		if len(task.Dependencies) > maxDependencies {
			return Request{}, fmt.Errorf("task %s has too many dependencies", task.ID)
		}
		if err := normalizeUniqueIDs(task.ID, "dependency", task.Dependencies); err != nil {
			return Request{}, err
		}
		if len(task.Resources) > maxResources {
			return Request{}, fmt.Errorf("task %s has too many resource requirements", task.ID)
		}
		resourceSeen := map[string]struct{}{}
		for resourceIndex := range task.Resources {
			requirement := &task.Resources[resourceIndex]
			requirement.ResourceID = strings.TrimSpace(requirement.ResourceID)
			if !validOpaqueID(requirement.ResourceID) || requirement.CapacityUnits <= 0 || requirement.CapacityUnits > maxCapacity {
				return Request{}, fmt.Errorf("task %s has an invalid resource requirement", task.ID)
			}
			if _, exists := resourceSeen[requirement.ResourceID]; exists {
				return Request{}, fmt.Errorf("task %s repeats resource %s", task.ID, requirement.ResourceID)
			}
			resourceSeen[requirement.ResourceID] = struct{}{}
		}
		sort.Slice(task.Resources, func(i, j int) bool { return task.Resources[i].ResourceID < task.Resources[j].ResourceID })
		if err := validateUsage(task.ID, task.EstimatedUsage); err != nil {
			return Request{}, err
		}
		if task.UncertaintyReviewPct < 0 || task.UncertaintyReviewPct > 10000 {
			return Request{}, fmt.Errorf("task %s uncertainty review percentage is invalid", task.ID)
		}
		task.Approval.Reasons = normalizeStrings(task.Approval.Reasons)
		if hasUnsafeText(task.Duration.Basis) || containsUnsafeStrings(task.Approval.Reasons) {
			return Request{}, fmt.Errorf("task %s planning explanation contains secret material", task.ID)
		}
		if task.Approval.Required && len(task.Approval.Reasons) == 0 {
			return Request{}, fmt.Errorf("task %s requires an approval reason", task.ID)
		}
	}

	for _, task := range request.Tasks {
		for _, dependency := range task.Dependencies {
			if dependency == task.ID {
				return Request{}, fmt.Errorf("task %s cannot depend on itself", task.ID)
			}
		}
	}

	for index := range request.Availability {
		window := &request.Availability[index]
		window.ResourceID = strings.TrimSpace(window.ResourceID)
		window.Start = minuteUTC(window.Start)
		window.End = minuteUTC(window.End)
		if !validOpaqueID(window.ResourceID) || window.CapacityUnits <= 0 || window.CapacityUnits > maxCapacity {
			return Request{}, fmt.Errorf("availability window %d is invalid", index)
		}
		if window.Start.Before(request.HorizonStart) || window.End.After(request.HorizonEnd) || !window.End.After(window.Start) {
			return Request{}, fmt.Errorf("availability window %d is outside the planning horizon", index)
		}
	}
	sort.Slice(request.Availability, func(i, j int) bool {
		left, right := request.Availability[i], request.Availability[j]
		if left.ResourceID != right.ResourceID {
			return left.ResourceID < right.ResourceID
		}
		if !left.Start.Equal(right.Start) {
			return left.Start.Before(right.Start)
		}
		if !left.End.Equal(right.End) {
			return left.End.Before(right.End)
		}
		return left.CapacityUnits < right.CapacityUnits
	})
	sort.Slice(request.Tasks, func(i, j int) bool { return request.Tasks[i].ID < request.Tasks[j].ID })
	return request, nil
}

func cloneRequest(request Request) Request {
	copyRequest := request
	copyRequest.Tasks = make([]Task, len(request.Tasks))
	for index, task := range request.Tasks {
		copyTask := task
		copyTask.Dependencies = append([]string(nil), task.Dependencies...)
		copyTask.Resources = append([]ResourceRequirement(nil), task.Resources...)
		copyTask.Approval.Reasons = append([]string(nil), task.Approval.Reasons...)
		if task.EarliestStart != nil {
			value := *task.EarliestStart
			copyTask.EarliestStart = &value
		}
		if task.Deadline != nil {
			value := *task.Deadline
			copyTask.Deadline = &value
		}
		copyRequest.Tasks[index] = copyTask
	}
	copyRequest.Availability = append([]CapacityWindow(nil), request.Availability...)
	copyRequest.Budget = cloneBudget(request.Budget)
	copyRequest.ApprovalPolicy.CostThresholdMicros = cloneInt64(request.ApprovalPolicy.CostThresholdMicros)
	copyRequest.ApprovalPolicy.InputTokenThreshold = cloneInt64(request.ApprovalPolicy.InputTokenThreshold)
	copyRequest.ApprovalPolicy.OutputTokenThreshold = cloneInt64(request.ApprovalPolicy.OutputTokenThreshold)
	copyRequest.ApprovalPolicy.ToolCallThreshold = cloneInt64(request.ApprovalPolicy.ToolCallThreshold)
	return copyRequest
}

func cloneBudget(budget Budget) Budget {
	return Budget{
		MaxCostMicros: cloneInt64(budget.MaxCostMicros), MaxInputTokens: cloneInt64(budget.MaxInputTokens),
		MaxOutputTokens: cloneInt64(budget.MaxOutputTokens), MaxToolCalls: cloneInt64(budget.MaxToolCalls),
	}
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func validateDuration(taskID string, estimate DurationEstimate) error {
	if estimate.OptimisticMinutes <= 0 || estimate.ExpectedMinutes <= 0 || estimate.PessimisticMinutes <= 0 ||
		estimate.OptimisticMinutes > estimate.ExpectedMinutes || estimate.ExpectedMinutes > estimate.PessimisticMinutes ||
		estimate.PessimisticMinutes > maxMinutes {
		return fmt.Errorf("task %s has an invalid duration estimate", taskID)
	}
	if len(estimate.Basis) > 500 {
		return fmt.Errorf("task %s duration basis is too long", taskID)
	}
	return nil
}

func hasUnsafeText(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && safety.RedactSecrets(value) != value
}

func containsUnsafeStrings(values []string) bool {
	for _, value := range values {
		if hasUnsafeText(value) {
			return true
		}
	}
	return false
}

func validateUsage(taskID string, usage Usage) error {
	if usage.CostMicros < 0 || usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.ToolCalls < 0 {
		return fmt.Errorf("task %s has a negative usage estimate", taskID)
	}
	return nil
}

func validateBudget(budget Budget, label string) error {
	values := []struct {
		name  string
		value *int64
	}{{"cost", budget.MaxCostMicros}, {"input tokens", budget.MaxInputTokens}, {"output tokens", budget.MaxOutputTokens}, {"tool calls", budget.MaxToolCalls}}
	for _, item := range values {
		if item.value != nil && *item.value < 0 {
			return fmt.Errorf("%s %s limit cannot be negative", label, item.name)
		}
	}
	return nil
}

func validateApprovalPolicy(policy ApprovalPolicy) error {
	budget := Budget{
		MaxCostMicros: policy.CostThresholdMicros, MaxInputTokens: policy.InputTokenThreshold,
		MaxOutputTokens: policy.OutputTokenThreshold, MaxToolCalls: policy.ToolCallThreshold,
	}
	if err := validateBudget(budget, "approval"); err != nil {
		return err
	}
	if policy.UncertaintyThreshold < 0 || policy.UncertaintyThreshold > 10000 {
		return fmt.Errorf("approval uncertainty threshold is invalid")
	}
	return nil
}

func normalizeUniqueIDs(taskID, label string, values []string) error {
	seen := map[string]struct{}{}
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
		if !validOpaqueID(values[index]) {
			return fmt.Errorf("task %s has an invalid %s id", taskID, label)
		}
		if _, exists := seen[values[index]]; exists {
			return fmt.Errorf("task %s repeats %s %s", taskID, label, values[index])
		}
		seen[values[index]] = struct{}{}
	}
	sort.Strings(values)
	return nil
}

func normalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 500 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func validOpaqueID(value string) bool {
	if len(value) == 0 || len(value) > 96 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.') {
			return false
		}
	}
	return true
}

func minuteUTC(value time.Time) time.Time { return value.UTC().Truncate(time.Minute) }

func minutesBetween(start, end time.Time) int64 { return int64(end.Sub(start) / time.Minute) }

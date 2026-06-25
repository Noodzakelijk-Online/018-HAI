package workflow

import (
	"automation-hub-backend/internal/models"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

func (r *GormRepository) startAutonomyAttempt(item *models.WorkflowItem, now time.Time) {
	if item == nil {
		return
	}
	attempt := item.RetryCount + 1
	confidence := math.Max(0, math.Min(1, item.Confidence))
	snapshot, err := json.Marshal(map[string]any{
		"workflowId":       item.ID,
		"state":            item.CurrentState,
		"taskType":         item.TaskType,
		"riskLevel":        item.RiskLevel,
		"priorityScore":    item.PriorityScore,
		"nextAction":       item.NextAction,
		"sourceType":       item.SourceType,
		"sourceId":         item.SourceID,
		"sourceRevision":   item.SourceRevision,
		"requiresApproval": item.RequiresApproval,
		"approvalStatus":   item.ApprovalStatus,
	})
	if err != nil {
		return
	}
	state := &models.AutonomyWorldState{
		WorkflowID:        item.ID,
		Attempt:           attempt,
		ObservationType:   "workflow_state",
		State:             item.CurrentState,
		Snapshot:          string(snapshot),
		Confidence:        confidence,
		Uncertainty:       1 - confidence,
		SourceRevision:    item.SourceRevision,
		ObservedAt:        now,
		StaleAfter:        now.Add(worldStateTTL()),
		Partial:           item.SourceID == "" && item.SourceURI == "",
		RequiresReobserve: false,
	}
	if err := r.DB.Create(state).Error; err != nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"request":      item.Description,
		"projectKey":   item.ProjectKey,
		"automationId": item.AutomationID,
		"nextAction":   item.NextAction,
	})
	if err != nil {
		return
	}
	approvalRecorded := !item.RequiresApproval || item.ApprovalStatus == "approved"
	action := &models.AutonomyActionTrace{
		WorkflowID:       item.ID,
		WorldStateID:     &state.ID,
		Attempt:          attempt,
		InterfaceType:    "skill_call",
		ActionType:       "run_workflow_task",
		ActionPayload:    string(payload),
		Status:           "running",
		PolicyDecision:   "allowed",
		PolicyReason:     "workflow worker claim passed state, retry, schedule, and approval guards",
		RequiresApproval: item.RequiresApproval,
		ApprovalRecorded: approvalRecorded,
		StartedAt:        now,
	}
	_ = r.DB.Create(action).Error
}

func (r *GormRepository) finishAutonomyAttempt(item *models.WorkflowItem, now time.Time) {
	if item == nil {
		return
	}
	var action models.AutonomyActionTrace
	err := r.DB.
		Where("workflow_id = ? AND status = ?", item.ID, "running").
		Order("started_at desc").
		First(&action).Error
	if err != nil {
		return
	}
	policyCompliant := !item.RequiresApproval || item.ApprovalStatus == "approved"
	verified := acceptsVerifiedExecution(item.VerificationStatus)
	rawCompletion := item.CurrentState == StateCompleted
	completionUnderPolicy := rawCompletion && policyCompliant && verified
	action.Status = autonomyActionStatus(item)
	action.PolicyDecision = "allowed"
	action.PolicyReason = "execution remained within workflow policy"
	if !policyCompliant {
		action.PolicyDecision = "blocked"
		action.PolicyReason = "mandatory approval was not recorded"
	}
	action.ApprovalRecorded = policyCompliant
	action.ExecutionVerified = verified
	action.VerificationStatus = item.VerificationStatus
	action.CompletedAt = &now
	action.LatencyMilliseconds = now.Sub(action.StartedAt).Milliseconds()
	action.ResultSummary = firstNonEmpty(item.LastWorkerError, item.BlockedReason, item.NextAction)
	if err := r.DB.Save(&action).Error; err != nil {
		return
	}
	evaluation := &models.AutonomyEvaluation{
		WorkflowID:                item.ID,
		ActionTraceID:             action.ID,
		Attempt:                   action.Attempt,
		RawCompletion:             rawCompletion,
		ExecutionBasedCorrectness: verified,
		CompletionUnderPolicy:     completionUnderPolicy,
		PartialCompletion:         !rawCompletion && item.RetryCount > 0,
		PolicyCompliant:           policyCompliant,
		RiskViolation:             !policyCompliant,
		InvalidAction:             false,
		HumanIntervention:         item.CurrentState == StateNeedsApproval || item.CurrentState == StateBlocked || item.RecoveryStatus == RecoveryNeedsReview,
		RecoveryAttempt:           item.RetryCount > 0,
		Recovered:                 item.RecoveryStatus == RecoveryCompletedAfterRetry,
		RetryCount:                item.RetryCount,
		LatencyMilliseconds:       action.LatencyMilliseconds,
		FailureMode:               autonomyFailureMode(item, verified, policyCompliant),
	}
	_ = r.DB.Create(evaluation).Error
}

func acceptsVerifiedExecution(status string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	for _, accepted := range []string{"verified", "source_supported", "schema_validated", "test_passed", "human_approved"} {
		if normalized == accepted || strings.Contains(normalized, accepted) {
			return true
		}
	}
	return false
}

func autonomyActionStatus(item *models.WorkflowItem) string {
	switch item.CurrentState {
	case StateCompleted:
		return "completed"
	case StateReady:
		return "retry_scheduled"
	case StateNeedsApproval:
		return "blocked_approval"
	case StateBlocked:
		return "needs_review"
	default:
		return "stopped"
	}
}

func autonomyFailureMode(item *models.WorkflowItem, verified, policyCompliant bool) string {
	switch {
	case !policyCompliant:
		return "policy_violation"
	case item.RecoveryStatus == RecoveryNeedsReview:
		return "interrupted_execution"
	case item.CurrentState == StateReady:
		return "validation_retry"
	case item.CurrentState == StateBlocked && !verified:
		return "verification_failure"
	case item.CurrentState == StateBlocked:
		return "human_review"
	default:
		return ""
	}
}

func worldStateTTL() time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(os.Getenv("AUTONOMY_WORLD_STATE_TTL_SECONDS")))
	if err != nil || seconds < 15 || seconds > 3600 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

// Package operations implements the HAI Phase 2 Operation Ledger — the root
// aggregate for autonomous back-office work. Every source item, decision,
// approval, execution, verification, and audit event links to an Operation.
//
// This file defines the typed enums for the ledger (§10.6). Each enum has
// IsValid, String, and Parse so no ad-hoc string literals leak through services
// and handlers.
package operations

import (
	"fmt"
	"strings"
)

// OperationStatus is the lifecycle state of an Operation (§7 / §8).
type OperationStatus string

const (
	StatusNew             OperationStatus = "new"
	StatusClassified      OperationStatus = "classified"
	StatusReady           OperationStatus = "ready"
	StatusDrafting        OperationStatus = "drafting"
	StatusDraftReady      OperationStatus = "draft_ready"
	StatusAwaitingApproval OperationStatus = "awaiting_approval"
	StatusApproved        OperationStatus = "approved"
	StatusRunning         OperationStatus = "running"
	StatusVerifying       OperationStatus = "verifying"
	StatusCompleted       OperationStatus = "completed"
	StatusBlocked         OperationStatus = "blocked"
	StatusFailed          OperationStatus = "failed"
	StatusWaitingExternal OperationStatus = "waiting_external"
	StatusInterrupted     OperationStatus = "interrupted"
	StatusDismissed       OperationStatus = "dismissed"
	StatusArchived        OperationStatus = "archived"
)

func (s OperationStatus) String() string { return string(s) }

func (s OperationStatus) IsValid() bool {
	switch s {
	case StatusNew, StatusClassified, StatusReady, StatusDrafting, StatusDraftReady,
		StatusAwaitingApproval, StatusApproved, StatusRunning, StatusVerifying,
		StatusCompleted, StatusBlocked, StatusFailed, StatusWaitingExternal,
		StatusInterrupted, StatusDismissed, StatusArchived:
		return true
	}
	return false
}

// IsTerminal reports whether the status has no further transitions.
func (s OperationStatus) IsTerminal() bool {
	return s == StatusArchived
}

func ParseOperationStatus(v string) (OperationStatus, error) {
	s := OperationStatus(strings.ToLower(strings.TrimSpace(v)))
	if !s.IsValid() {
		return "", fmt.Errorf("invalid operation status: %q", v)
	}
	return s, nil
}

// RiskLevel is the Robert-taxonomy risk of an Operation (§25).
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

func (r RiskLevel) String() string { return string(r) }

func (r RiskLevel) IsValid() bool {
	switch r {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	}
	return false
}

func ParseRiskLevel(v string) (RiskLevel, error) {
	r := RiskLevel(strings.ToLower(strings.TrimSpace(v)))
	if !r.IsValid() {
		return "", fmt.Errorf("invalid risk level: %q", v)
	}
	return r, nil
}

// AutonomyLevel is how autonomously HAI may act on an Operation.
type AutonomyLevel string

const (
	AutonomyObserve  AutonomyLevel = "observe"   // read/classify/summarize only
	AutonomyDraft    AutonomyLevel = "draft"     // may prepare internal drafts
	AutonomyAuto     AutonomyLevel = "automatic" // low-risk reversible auto-execution
	AutonomyApproval AutonomyLevel = "approval"  // needs Robert approval
	AutonomyBlocked  AutonomyLevel = "blocked"   // must not proceed
)

func (a AutonomyLevel) String() string { return string(a) }

func (a AutonomyLevel) IsValid() bool {
	switch a {
	case AutonomyObserve, AutonomyDraft, AutonomyAuto, AutonomyApproval, AutonomyBlocked:
		return true
	}
	return false
}

func ParseAutonomyLevel(v string) (AutonomyLevel, error) {
	a := AutonomyLevel(strings.ToLower(strings.TrimSpace(v)))
	if !a.IsValid() {
		return "", fmt.Errorf("invalid autonomy level: %q", v)
	}
	return a, nil
}

// OwnerType is who owns an Operation (§7).
type OwnerType string

const (
	OwnerHAI      OwnerType = "hai"
	OwnerRobert   OwnerType = "robert"
	OwnerVA       OwnerType = "va"
	OwnerExternal OwnerType = "external"
	OwnerRuntime  OwnerType = "runtime"
)

func (o OwnerType) String() string { return string(o) }

func (o OwnerType) IsValid() bool {
	switch o {
	case OwnerHAI, OwnerRobert, OwnerVA, OwnerExternal, OwnerRuntime:
		return true
	}
	return false
}

func ParseOwnerType(v string) (OwnerType, error) {
	o := OwnerType(strings.ToLower(strings.TrimSpace(v)))
	if !o.IsValid() {
		return "", fmt.Errorf("invalid owner type: %q", v)
	}
	return o, nil
}

// CurrentDecision is the single current autonomy decision for an Operation (§9).
type CurrentDecision string

const (
	DecisionIgnore               CurrentDecision = "ignore"
	DecisionObserveOnly          CurrentDecision = "observe_only"
	DecisionSummarize            CurrentDecision = "summarize"
	DecisionExtractTask          CurrentDecision = "extract_task"
	DecisionCreateInternalTask   CurrentDecision = "create_internal_task"
	DecisionCreateWorkflow       CurrentDecision = "create_workflow"
	DecisionCreateDraft          CurrentDecision = "create_draft"
	DecisionAskRobert            CurrentDecision = "ask_robert"
	DecisionAskVA                CurrentDecision = "ask_va"
	DecisionWaitExternal         CurrentDecision = "wait_external"
	DecisionRunSafeLocalWorker   CurrentDecision = "run_safe_local_worker"
	DecisionRunModelPipeline     CurrentDecision = "run_model_pipeline"
	DecisionRunRuntimeAfterApprv CurrentDecision = "run_runtime_after_approval"
	DecisionExecuteAPIAfterApprv CurrentDecision = "execute_api_after_approval"
	DecisionStageBrowserAction   CurrentDecision = "stage_browser_action"
	DecisionBlock                CurrentDecision = "block"
	DecisionForbidden            CurrentDecision = "forbidden"
)

func (d CurrentDecision) String() string { return string(d) }

func (d CurrentDecision) IsValid() bool {
	switch d {
	case DecisionIgnore, DecisionObserveOnly, DecisionSummarize, DecisionExtractTask,
		DecisionCreateInternalTask, DecisionCreateWorkflow, DecisionCreateDraft,
		DecisionAskRobert, DecisionAskVA, DecisionWaitExternal, DecisionRunSafeLocalWorker,
		DecisionRunModelPipeline, DecisionRunRuntimeAfterApprv, DecisionExecuteAPIAfterApprv,
		DecisionStageBrowserAction, DecisionBlock, DecisionForbidden:
		return true
	}
	return false
}

// RequiresApproval reports whether a decision inherently needs Robert's approval
// before any external side effect.
func (d CurrentDecision) RequiresApproval() bool {
	switch d {
	case DecisionAskRobert, DecisionRunRuntimeAfterApprv, DecisionExecuteAPIAfterApprv,
		DecisionStageBrowserAction:
		return true
	}
	return false
}

// IsAutoExecutable reports whether a decision may run without Robert (only the
// safe local worker and internal model pipeline in Phase 2A).
func (d CurrentDecision) IsAutoExecutable() bool {
	switch d {
	case DecisionRunSafeLocalWorker, DecisionRunModelPipeline:
		return true
	}
	return false
}

func ParseCurrentDecision(v string) (CurrentDecision, error) {
	d := CurrentDecision(strings.ToLower(strings.TrimSpace(v)))
	if !d.IsValid() {
		return "", fmt.Errorf("invalid current decision: %q", v)
	}
	return d, nil
}

// VerificationStatus tracks the verification outcome of an Operation.
type VerificationStatus string

const (
	VerificationNotRequired VerificationStatus = "not_required"
	VerificationPending     VerificationStatus = "pending"
	VerificationPassed      VerificationStatus = "passed"
	VerificationFailed      VerificationStatus = "failed"
	VerificationInconclusive VerificationStatus = "inconclusive"
)

func (v VerificationStatus) String() string { return string(v) }

func (v VerificationStatus) IsValid() bool {
	switch v {
	case VerificationNotRequired, VerificationPending, VerificationPassed,
		VerificationFailed, VerificationInconclusive:
		return true
	}
	return false
}

func ParseVerificationStatus(v string) (VerificationStatus, error) {
	s := VerificationStatus(strings.ToLower(strings.TrimSpace(v)))
	if !s.IsValid() {
		return "", fmt.Errorf("invalid verification status: %q", v)
	}
	return s, nil
}

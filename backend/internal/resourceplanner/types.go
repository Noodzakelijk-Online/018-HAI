// Package resourceplanner provides deterministic, advisory-only resource and
// time planning. It does not execute work, consume approvals, or grant any
// authority to another subsystem.
package resourceplanner

import "time"

const AlgorithmVersion = "resourceplanner/v1"

type Feasibility string

const (
	Feasible              Feasibility = "feasible"
	FeasibleWithApprovals Feasibility = "feasible_with_approvals"
	Infeasible            Feasibility = "infeasible"
)

type DeadlineKind string

const (
	HardDeadline DeadlineKind = "hard"
	SoftDeadline DeadlineKind = "soft"
)

type DurationMode string

const (
	ExpectedDuration     DurationMode = "expected"
	ConservativeDuration DurationMode = "conservative"
)

// DurationEstimate records the bounded estimate used for planning. Expected
// must lie between optimistic and pessimistic.
type DurationEstimate struct {
	OptimisticMinutes  int64  `json:"optimisticMinutes"`
	ExpectedMinutes    int64  `json:"expectedMinutes"`
	PessimisticMinutes int64  `json:"pessimisticMinutes"`
	Basis              string `json:"basis,omitempty"`
}

type ResourceRequirement struct {
	ResourceID    string `json:"resourceId"`
	CapacityUnits int64  `json:"capacityUnits"`
}

// CapacityWindow describes capacity available for one resource during a
// half-open interval [start, end). Overlapping windows contribute the maximum
// declared capacity, not their sum, preventing duplicate calendars from
// inflating capacity accidentally.
type CapacityWindow struct {
	ResourceID    string    `json:"resourceId"`
	Start         time.Time `json:"start"`
	End           time.Time `json:"end"`
	CapacityUnits int64     `json:"capacityUnits"`
}

// Usage is an estimate only. Cost is represented in millionths of one euro to
// avoid floating-point decisions.
type Usage struct {
	CostMicros   int64 `json:"costMicros"`
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	ToolCalls    int64 `json:"toolCalls"`
}

// Budget uses pointers so nil means unlimited while a pointer to zero is a
// real zero budget.
type Budget struct {
	MaxCostMicros   *int64 `json:"maxCostMicros,omitempty"`
	MaxInputTokens  *int64 `json:"maxInputTokens,omitempty"`
	MaxOutputTokens *int64 `json:"maxOutputTokens,omitempty"`
	MaxToolCalls    *int64 `json:"maxToolCalls,omitempty"`
}

type TaskApproval struct {
	Required bool     `json:"required"`
	Reasons  []string `json:"reasons,omitempty"`
}

type Task struct {
	ID                   string                `json:"id"`
	Duration             DurationEstimate      `json:"duration"`
	EarliestStart        *time.Time            `json:"earliestStart,omitempty"`
	Deadline             *time.Time            `json:"deadline,omitempty"`
	DeadlineKind         DeadlineKind          `json:"deadlineKind,omitempty"`
	Dependencies         []string              `json:"dependencies,omitempty"`
	Resources            []ResourceRequirement `json:"resources,omitempty"`
	EstimatedUsage       Usage                 `json:"estimatedUsage"`
	Priority             int                   `json:"priority"`
	Optional             bool                  `json:"optional,omitempty"`
	Approval             TaskApproval          `json:"approval"`
	UncertaintyReviewPct int64                 `json:"uncertaintyReviewPct,omitempty"`
}

// ApprovalPolicy adds review flags. Thresholds are advisory flags only and do
// not relax hard Budget limits.
type ApprovalPolicy struct {
	CostThresholdMicros  *int64 `json:"costThresholdMicros,omitempty"`
	InputTokenThreshold  *int64 `json:"inputTokenThreshold,omitempty"`
	OutputTokenThreshold *int64 `json:"outputTokenThreshold,omitempty"`
	ToolCallThreshold    *int64 `json:"toolCallThreshold,omitempty"`
	UncertaintyThreshold int64  `json:"uncertaintyThresholdPct,omitempty"`
	SoftDeadlineMiss     bool   `json:"softDeadlineMiss"`
}

type Request struct {
	OwnerIdentity    string           `json:"-"`
	OwnerScopeDigest string           `json:"ownerScopeDigest"`
	WorkspaceID      string           `json:"workspaceId,omitempty"`
	PlanID           string           `json:"planId"`
	AsOf             time.Time        `json:"asOf"`
	HorizonStart     time.Time        `json:"horizonStart"`
	HorizonEnd       time.Time        `json:"horizonEnd"`
	DurationMode     DurationMode     `json:"durationMode,omitempty"`
	Tasks            []Task           `json:"tasks"`
	Availability     []CapacityWindow `json:"availability,omitempty"`
	Budget           Budget           `json:"budget"`
	ApprovalPolicy   ApprovalPolicy   `json:"approvalPolicy"`
}

type Allocation struct {
	ResourceID    string `json:"resourceId"`
	CapacityUnits int64  `json:"capacityUnits"`
}

type ScheduledTask struct {
	TaskID                 string       `json:"taskId"`
	Start                  time.Time    `json:"start"`
	End                    time.Time    `json:"end"`
	PlannedDurationMinutes int64        `json:"plannedDurationMinutes"`
	DeadlineSlackMinutes   *int64       `json:"deadlineSlackMinutes,omitempty"`
	DependencySlackMinutes int64        `json:"dependencySlackMinutes"`
	NetworkSlackMinutes    int64        `json:"networkSlackMinutes"`
	Critical               bool         `json:"critical"`
	Allocations            []Allocation `json:"allocations,omitempty"`
	Dependencies           []string     `json:"dependencies,omitempty"`
	DurationEstimateBasis  string       `json:"durationEstimateBasis"`
	DurationUncertaintyPct int64        `json:"durationUncertaintyPct"`
}

type Blocker struct {
	Code              string `json:"code"`
	TaskID            string `json:"taskId,omitempty"`
	ResourceID        string `json:"resourceId,omitempty"`
	Detail            string `json:"detail"`
	BlocksFeasibility bool   `json:"blocksFeasibility"`
}

type ApprovalFlag struct {
	Code      string `json:"code"`
	TaskID    string `json:"taskId,omitempty"`
	Reason    string `json:"reason"`
	Mandatory bool   `json:"mandatory"`
}

type ReplanReason struct {
	Code            string   `json:"code"`
	TaskIDs         []string `json:"taskIds,omitempty"`
	ResourceIDs     []string `json:"resourceIds,omitempty"`
	Detail          string   `json:"detail"`
	SuggestedChange string   `json:"suggestedChange"`
}

type FallbackStage struct {
	Stage            int      `json:"stage"`
	Code             string   `json:"code"`
	Description      string   `json:"description"`
	TriggeredBy      []string `json:"triggeredBy"`
	AffectedTaskIDs  []string `json:"affectedTaskIds,omitempty"`
	RequiresApproval bool     `json:"requiresApproval"`
}

type BudgetAssessment struct {
	Estimated              Usage  `json:"estimated"`
	Limits                 Budget `json:"limits"`
	WithinCostLimit        bool   `json:"withinCostLimit"`
	WithinInputTokenLimit  bool   `json:"withinInputTokenLimit"`
	WithinOutputTokenLimit bool   `json:"withinOutputTokenLimit"`
	WithinToolCallLimit    bool   `json:"withinToolCallLimit"`
}

type AuditEntry struct {
	Sequence int    `json:"sequence"`
	Code     string `json:"code"`
	Subject  string `json:"subject,omitempty"`
	Detail   string `json:"detail"`
}

type Decision struct {
	PlanID             string           `json:"planId"`
	OwnerScopeDigest   string           `json:"ownerScopeDigest"`
	WorkspaceID        string           `json:"workspaceId,omitempty"`
	AlgorithmVersion   string           `json:"algorithmVersion"`
	AsOf               time.Time        `json:"asOf"`
	InputDigest        string           `json:"inputDigest"`
	DecisionDigest     string           `json:"decisionDigest"`
	Feasibility        Feasibility      `json:"feasibility"`
	Scheduled          []ScheduledTask  `json:"scheduled"`
	UnscheduledTaskIDs []string         `json:"unscheduledTaskIds,omitempty"`
	Budget             BudgetAssessment `json:"budget"`
	CriticalBlockers   []Blocker        `json:"criticalBlockers,omitempty"`
	Advisories         []Blocker        `json:"advisories,omitempty"`
	ApprovalFlags      []ApprovalFlag   `json:"approvalFlags,omitempty"`
	ReplanReasons      []ReplanReason   `json:"replanReasons,omitempty"`
	FallbackStages     []FallbackStage  `json:"fallbackStages,omitempty"`
	Audit              []AuditEntry     `json:"audit"`
	Authority          string           `json:"authority"`
	CanExecute         bool             `json:"canExecute"`
	GrantsAuthority    bool             `json:"grantsAuthority"`
}

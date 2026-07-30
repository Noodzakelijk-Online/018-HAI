package workflowgraph

import "time"

const (
	CurrentDefinitionSchemaVersion uint16 = 1
	CurrentRunSchemaVersion        uint16 = 1
)

type NodeType string

const (
	NodeAction        NodeType = "action"
	NodeCondition     NodeType = "condition"
	NodeHumanApproval NodeType = "human_approval"
	NodeWait          NodeType = "wait"
	NodeTimer         NodeType = "timer"
	NodeParallelSplit NodeType = "parallel_split"
	NodeParallelJoin  NodeType = "parallel_join"
	NodeVerification  NodeType = "verification"
	NodeCompensation  NodeType = "compensation"
	NodeTerminal      NodeType = "terminal"
)

type ConditionOperator string

const (
	ConditionEqual     ConditionOperator = "equal"
	ConditionNotEqual  ConditionOperator = "not_equal"
	ConditionExists    ConditionOperator = "exists"
	ConditionNotExists ConditionOperator = "not_exists"
)

type JoinMode string

const (
	JoinAll JoinMode = "all"
	JoinAny JoinMode = "any"
)

type TerminalResult string

const (
	TerminalCompleted TerminalResult = "completed"
	TerminalFailed    TerminalResult = "failed"
	TerminalCancelled TerminalResult = "cancelled"
)

const (
	OutcomeDefault  = "default"
	OutcomeTrue     = "true"
	OutcomeFalse    = "false"
	OutcomeApproved = "approved"
	OutcomeRejected = "rejected"
	OutcomeElapsed  = "elapsed"
)

// Definition is immutable once a (ID, Version) pair has been stored.
type Definition struct {
	SchemaVersion uint16    `json:"schemaVersion"`
	ID            string    `json:"id"`
	Version       uint64    `json:"version"`
	Name          string    `json:"name"`
	EntryNodeID   string    `json:"entryNodeId"`
	MaxRunSteps   uint32    `json:"maxRunSteps"`
	Nodes         []Node    `json:"nodes"`
	Edges         []Edge    `json:"edges"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Node struct {
	ID                 string           `json:"id"`
	Type               NodeType         `json:"type"`
	Name               string           `json:"name,omitempty"`
	Condition          *ConditionConfig `json:"condition,omitempty"`
	Timer              *TimerConfig     `json:"timer,omitempty"`
	Join               *JoinConfig      `json:"join,omitempty"`
	Terminal           *TerminalConfig  `json:"terminal,omitempty"`
	CompensationNodeID string           `json:"compensationNodeId,omitempty"`
}

type ConditionConfig struct {
	Field    string            `json:"field"`
	Operator ConditionOperator `json:"operator"`
	Value    string            `json:"value,omitempty"`
}

type TimerConfig struct {
	After time.Duration `json:"after"`
}

type JoinConfig struct {
	SplitNodeID string   `json:"splitNodeId"`
	Mode        JoinMode `json:"mode"`
}

type TerminalConfig struct {
	Result TerminalResult `json:"result"`
}

// MaxTraversals is zero for an unbounded edge. Validation rejects any cycle
// that can be traversed without crossing at least one explicitly bounded edge.
type Edge struct {
	ID            string `json:"id"`
	From          string `json:"from"`
	To            string `json:"to"`
	Outcome       string `json:"outcome,omitempty"`
	Order         uint16 `json:"order"`
	MaxTraversals uint32 `json:"maxTraversals,omitempty"`
}

type RunStatus string

const (
	RunPending      RunStatus = "pending"
	RunRunning      RunStatus = "running"
	RunWaiting      RunStatus = "waiting"
	RunCancelling   RunStatus = "cancelling"
	RunCompensating RunStatus = "compensating"
	RunCompleted    RunStatus = "completed"
	RunFailed       RunStatus = "failed"
	RunCancelled    RunStatus = "cancelled"
)

func (s RunStatus) Terminal() bool {
	return s == RunCompleted || s == RunFailed || s == RunCancelled
}

type NodeStatus string

const (
	NodePending     NodeStatus = "pending"
	NodeActive      NodeStatus = "active"
	NodeWaiting     NodeStatus = "waiting"
	NodeSucceeded   NodeStatus = "succeeded"
	NodeFailed      NodeStatus = "failed"
	NodeCancelled   NodeStatus = "cancelled"
	NodeSkipped     NodeStatus = "skipped"
	NodeCompensated NodeStatus = "compensated"
)

type Run struct {
	SchemaVersion     uint16                  `json:"schemaVersion"`
	ID                string                  `json:"id"`
	DefinitionID      string                  `json:"definitionId"`
	DefinitionVersion uint64                  `json:"definitionVersion"`
	Revision          uint64                  `json:"revision"`
	Status            RunStatus               `json:"status"`
	ActiveNodeIDs     []string                `json:"activeNodeIds"`
	NodeStates        map[string]NodeRunState `json:"nodeStates"`
	EdgeTraversals    map[string]uint32       `json:"edgeTraversals"`
	Steps             uint32                  `json:"steps"`
	StartedAt         time.Time               `json:"startedAt"`
	UpdatedAt         time.Time               `json:"updatedAt"`
	EndedAt           *time.Time              `json:"endedAt,omitempty"`
}

type NodeRunState struct {
	Status      NodeStatus `json:"status"`
	Attempts    uint32     `json:"attempts"`
	EnteredAt   time.Time  `json:"enteredAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Outcome     string     `json:"outcome,omitempty"`
}

type ApprovalDecision string

const (
	ApprovalPending  ApprovalDecision = ""
	ApprovalApproved ApprovalDecision = "approved"
	ApprovalRejected ApprovalDecision = "rejected"
)

// Evaluation contains only caller-supplied facts. The evaluator never reads a
// clock, executes an action, or evaluates arbitrary expressions.
type Evaluation struct {
	NodeID    string
	Outcome   string
	Approval  ApprovalDecision
	Signal    string
	Variables map[string]string
	Now       time.Time
}

type Disposition string

const (
	DispositionAdvance  Disposition = "advance"
	DispositionWait     Disposition = "wait"
	DispositionComplete Disposition = "complete"
	DispositionFail     Disposition = "fail"
	DispositionCancel   Disposition = "cancel"
)

type NextNode struct {
	NodeID string `json:"nodeId"`
	EdgeID string `json:"edgeId"`
}

type Decision struct {
	Disposition   Disposition     `json:"disposition"`
	FromNodeID    string          `json:"fromNodeId"`
	Next          []NextNode      `json:"next,omitempty"`
	ResultingRun  RunStatus       `json:"resultingRunStatus"`
	TerminalState *TerminalResult `json:"terminalResult,omitempty"`
	Reason        string          `json:"reason"`
}

type RunFilter struct {
	DefinitionID string
	Statuses     []RunStatus
	Limit        int
}

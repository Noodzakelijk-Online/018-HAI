package agentregistry

import "time"

const ContractVersion = 1

type AgentType string

const (
	AgentTypePlanner      AgentType = "planner"
	AgentTypeResearcher   AgentType = "researcher"
	AgentTypeExecutor     AgentType = "executor"
	AgentTypeReviewer     AgentType = "reviewer"
	AgentTypeSpecialist   AgentType = "specialist"
	AgentTypeOrchestrator AgentType = "orchestrator"
)

type LifecycleState string

const (
	StateRegistered  LifecycleState = "registered"
	StateEnabled     LifecycleState = "enabled"
	StateDraining    LifecycleState = "draining"
	StateDisabled    LifecycleState = "disabled"
	StateQuarantined LifecycleState = "quarantined"
)

type HealthStatus string

const (
	HealthUnknown   HealthStatus = "unknown"
	HealthHealthy   HealthStatus = "healthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
)

type Locality string

const (
	LocalityLocal Locality = "local"
	LocalityLAN   Locality = "lan"
	LocalityCloud Locality = "cloud"
)

type CapabilityDeclaration struct {
	ID          string   `json:"id"`
	Version     string   `json:"version"`
	Operations  []string `json:"operations,omitempty"`
	Description string   `json:"description,omitempty"`
}

type RuntimeAdapter struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	ProtocolVersion string `json:"protocolVersion"`
}

type HealthEvidence struct {
	Status    HealthStatus  `json:"status"`
	Ready     bool          `json:"ready"`
	Reason    string        `json:"reason,omitempty"`
	CheckedAt time.Time     `json:"checkedAt"`
	FreshFor  time.Duration `json:"freshFor"`
}

type Availability struct {
	Available         bool `json:"available"`
	ActiveAssignments int  `json:"activeAssignments"`
	MaxConcurrent     int  `json:"maxConcurrent"`
}

type ReliabilityEvidence struct {
	Successes           uint64    `json:"successes"`
	Failures            uint64    `json:"failures"`
	ConsecutiveFailures uint64    `json:"consecutiveFailures"`
	MeanLatencyMs       float64   `json:"meanLatencyMs"`
	LastOutcomeAt       time.Time `json:"lastOutcomeAt,omitempty"`
}

func (e ReliabilityEvidence) Score() float64 {
	// A bounded beta prior avoids presenting an untried agent as perfectly
	// reliable while still allowing evidence to move the score over time.
	return float64(e.Successes+1) / float64(e.Successes+e.Failures+2)
}

type PerformanceProfile struct {
	EstimatedCostEUR float64  `json:"estimatedCostEur"`
	P95LatencyMs     int64    `json:"p95LatencyMs"`
	Locality         Locality `json:"locality"`
}

type Agent struct {
	ContractVersion  int                     `json:"contractVersion"`
	ID               string                  `json:"id"`
	OwnerIdentity    string                  `json:"ownerIdentity"`
	Name             string                  `json:"name"`
	Type             AgentType               `json:"type"`
	Runtime          RuntimeAdapter          `json:"runtime"`
	Capabilities     []CapabilityDeclaration `json:"capabilities"`
	AuthorityCeiling int                     `json:"authorityCeiling"`
	AutonomyCeiling  int                     `json:"autonomyCeiling"`
	ToolAllowlist    []string                `json:"toolAllowlist,omitempty"`
	DataAllowlist    []string                `json:"dataAllowlist,omitempty"`
	FolderAllowlist  []string                `json:"folderAllowlist,omitempty"`
	Health           HealthEvidence          `json:"health"`
	State            LifecycleState          `json:"state"`
	Availability     Availability            `json:"availability"`
	Performance      PerformanceProfile      `json:"performance"`
	Reliability      ReliabilityEvidence     `json:"reliability"`
	Revision         uint64                  `json:"revision"`
	CreatedAt        time.Time               `json:"createdAt"`
	UpdatedAt        time.Time               `json:"updatedAt"`
}

type CapabilityRequirement struct {
	ID         string   `json:"id"`
	MinVersion string   `json:"minVersion,omitempty"`
	MaxVersion string   `json:"maxVersion,omitempty"`
	Operations []string `json:"operations,omitempty"`
}

type CompatibilityRequirement struct {
	RuntimeAdapterID   string `json:"runtimeAdapterId,omitempty"`
	RuntimeType        string `json:"runtimeType,omitempty"`
	MinProtocolVersion string `json:"minProtocolVersion,omitempty"`
	MaxProtocolVersion string `json:"maxProtocolVersion,omitempty"`
}

type AssignmentRequest struct {
	OwnerIdentity       string                   `json:"ownerIdentity"`
	TaskID              string                   `json:"taskId"`
	Capabilities        []CapabilityRequirement  `json:"capabilities"`
	Compatibility       CompatibilityRequirement `json:"compatibility"`
	RequiredAuthority   int                      `json:"requiredAuthority"`
	RequiredAutonomy    int                      `json:"requiredAutonomy"`
	PolicyMaxAuthority  int                      `json:"policyMaxAuthority"`
	PolicyMaxAutonomy   int                      `json:"policyMaxAutonomy"`
	RequiredTools       []string                 `json:"requiredTools,omitempty"`
	RequiredData        []string                 `json:"requiredData,omitempty"`
	RequiredFolders     []string                 `json:"requiredFolders,omitempty"`
	AllowedAgentTypes   []AgentType              `json:"allowedAgentTypes,omitempty"`
	MaxEstimatedCostEUR *float64                 `json:"maxEstimatedCostEur,omitempty"`
	RequireLocal        bool                     `json:"requireLocal"`
	AllowDegraded       bool                     `json:"allowDegraded"`
}

type ScoreComponent struct {
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

type AssignmentExplanation struct {
	Eligible       bool             `json:"eligible"`
	Components     []ScoreComponent `json:"components"`
	Constraints    []string         `json:"constraints"`
	RejectedReason string           `json:"rejectedReason,omitempty"`
}

type Assignment struct {
	ID               string                `json:"id"`
	OwnerIdentity    string                `json:"ownerIdentity"`
	TaskID           string                `json:"taskId"`
	AgentID          string                `json:"agentId"`
	AgentRevision    uint64                `json:"agentRevision"`
	GrantedAuthority int                   `json:"grantedAuthority"`
	GrantedAutonomy  int                   `json:"grantedAutonomy"`
	Score            float64               `json:"score"`
	Explanation      AssignmentExplanation `json:"explanation"`
	RequestDigest    string                `json:"requestDigest"`
	AssignedAt       time.Time             `json:"assignedAt"`
}

type Outcome struct {
	Success    bool          `json:"success"`
	Latency    time.Duration `json:"latency"`
	RecordedAt time.Time     `json:"recordedAt"`
}

type AssignmentOutcome struct {
	AssignmentID  string        `json:"assignmentId"`
	OwnerIdentity string        `json:"ownerIdentity"`
	AgentID       string        `json:"agentId"`
	Success       bool          `json:"success"`
	Latency       time.Duration `json:"latency"`
	RecordedAt    time.Time     `json:"recordedAt"`
}

type Transition struct {
	From       LifecycleState `json:"from"`
	To         LifecycleState `json:"to"`
	Reason     string         `json:"reason"`
	OccurredAt time.Time      `json:"occurredAt"`
	Revision   uint64         `json:"revision"`
}

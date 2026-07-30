package frameworkregistry

import "time"

const (
	StatusActive       = "active"
	StatusExperimental = "experimental"
	StatusDeprecated   = "deprecated"

	PreferenceDefault  = "default"
	PreferenceEnabled  = "enabled"
	PreferenceDisabled = "disabled"

	ConstitutionDraft      = "draft"
	ConstitutionActive     = "active"
	ConstitutionSuperseded = "superseded"
)

// Framework is the versioned, inspectable contract used by the chief-of-staff
// selector. It describes a decision discipline; it does not execute tools.
type Framework struct {
	ID                       string   `json:"id"`
	Version                  string   `json:"version"`
	Name                     string   `json:"name"`
	Family                   string   `json:"family"`
	Purpose                  string   `json:"purpose"`
	SuitableProblemTypes     []string `json:"suitableProblemTypes"`
	TriggerConditions        []string `json:"triggerConditions"`
	RequiredInputs           []string `json:"requiredInputs"`
	ProducedOutputs          []string `json:"producedOutputs"`
	RequiredAgents           []string `json:"requiredAgents"`
	WorkflowTemplate         []string `json:"workflowTemplate"`
	DecisionRules            []string `json:"decisionRules"`
	SafetyInvariants         []string `json:"safetyInvariants"`
	AuthorityRequirement     string   `json:"authorityRequirement"`
	MaximumAutonomyLevel     int      `json:"maximumAutonomyLevel"`
	RiskCeiling              string   `json:"riskCeiling"`
	EvidenceRequirements     []string `json:"evidenceRequirements"`
	EvaluationMethod         []string `json:"evaluationMethod"`
	ConflictsWith            []string `json:"conflictsWith"`
	UserSpecificAdaptations  []string `json:"userSpecificAdaptations"`
	CandidateImplementations []string `json:"candidateImplementations,omitempty"`
	Source                   string   `json:"source"`
	Provenance               string   `json:"provenance"`
	Status                   string   `json:"status"`
}

type Preference struct {
	FrameworkID          string    `json:"frameworkId"`
	State                string    `json:"state"`
	Pinned               bool      `json:"pinned"`
	MaximumAutonomyLevel *int      `json:"maximumAutonomyLevel,omitempty"`
	Adaptations          []string  `json:"adaptations"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

type FrameworkView struct {
	Framework
	EffectiveStatus        string     `json:"effectiveStatus"`
	Enabled                bool       `json:"enabled"`
	Pinned                 bool       `json:"pinned"`
	EffectiveAutonomyLevel int        `json:"effectiveAutonomyLevel"`
	Adaptations            []string   `json:"adaptations"`
	PreferenceUpdatedAt    *time.Time `json:"preferenceUpdatedAt,omitempty"`
}

type PreferencePatch struct {
	State                 string   `json:"state"`
	Pinned                *bool    `json:"pinned,omitempty"`
	MaximumAutonomyLevel  *int     `json:"maximumAutonomyLevel,omitempty"`
	ClearAutonomyOverride bool     `json:"clearAutonomyOverride,omitempty"`
	Adaptations           []string `json:"adaptations,omitempty"`
}

type SelectionRequest struct {
	OwnerIdentity       string   `json:"-"`
	TaskPlanID          string   `json:"taskPlanId,omitempty"`
	Request             string   `json:"request"`
	ProjectKey          string   `json:"projectKey,omitempty"`
	PursuitID           string   `json:"pursuitId,omitempty"`
	TaskType            string   `json:"taskType,omitempty"`
	RiskLevel           string   `json:"riskLevel,omitempty"`
	Difficulty          int      `json:"difficulty,omitempty"`
	RequiredReasoning   string   `json:"requiredReasoning,omitempty"`
	SuccessCriteria     []string `json:"successCriteria,omitempty"`
	NeedsMemory         bool     `json:"needsMemory,omitempty"`
	NeedsTools          bool     `json:"needsTools,omitempty"`
	NeedsDocuments      bool     `json:"needsDocuments,omitempty"`
	NeedsWebAccess      bool     `json:"needsWebAccess,omitempty"`
	NeedsLocalExecution bool     `json:"needsLocalExecution,omitempty"`
	NeedsApproval       bool     `json:"needsApproval,omitempty"`
	ExecuteRequested    bool     `json:"executeRequested,omitempty"`
	HumanApproved       bool     `json:"humanApproved,omitempty"`
}

type SelectedFramework struct {
	ID                   string   `json:"id"`
	Version              string   `json:"version"`
	Name                 string   `json:"name"`
	Family               string   `json:"family"`
	Score                float64  `json:"score"`
	Reasons              []string `json:"reasons"`
	MaximumAutonomyLevel int      `json:"maximumAutonomyLevel"`
	AuthorityRequirement string   `json:"authorityRequirement"`
	EvidenceRequirements []string `json:"evidenceRequirements"`
	EvaluationMethod     []string `json:"evaluationMethod"`
}

type FrameworkConflict struct {
	SelectedID string `json:"selectedId"`
	SkippedID  string `json:"skippedId"`
	Reason     string `json:"reason"`
}

// SelectionDecision is an auditable chief-of-staff recommendation. Authority
// is a ceiling and cannot grant execution permission by itself.
type SelectionDecision struct {
	ID                        string              `json:"id"`
	TaskPlanID                string              `json:"taskPlanId,omitempty"`
	CreatedAt                 time.Time           `json:"createdAt"`
	CatalogVersion            string              `json:"catalogVersion"`
	CatalogDigest             string              `json:"catalogDigest"`
	SelectorAlgorithmVersion  string              `json:"selectorAlgorithmVersion"`
	EffectivePreferenceDigest string              `json:"effectivePreferenceDigest"`
	ConstitutionDigest        string              `json:"constitutionDigest"`
	LifeDomain                string              `json:"lifeDomain"`
	NeedOrCommitment          string              `json:"needOrCommitment"`
	Selected                  []SelectedFramework `json:"selected"`
	Conflicts                 []FrameworkConflict `json:"conflicts"`
	RequiredAgents            []string            `json:"requiredAgents"`
	MaximumAutonomyLevel      int                 `json:"maximumAutonomyLevel"`
	AuthoritySummary          string              `json:"authoritySummary"`
	RequiresApproval          bool                `json:"requiresApproval"`
	ApprovalReasons           []string            `json:"approvalReasons"`
	EvidenceRequirements      []string            `json:"evidenceRequirements"`
	CompletionCriteria        []string            `json:"completionCriteria"`
	LearningPlan              []string            `json:"learningPlan"`
	ContextRequirements       []string            `json:"contextRequirements"`
	SelectionReason           string              `json:"selectionReason"`
	ConstitutionVersion       int                 `json:"constitutionVersion"`
	ConstitutionSource        string              `json:"constitutionSource"`
}

type Constitution struct {
	ID                  string     `json:"id"`
	Version             int        `json:"version"`
	BaseVersion         int        `json:"baseVersion"`
	Status              string     `json:"status"`
	Values              []string   `json:"values"`
	Prohibitions        []string   `json:"prohibitions"`
	StandingPermissions []string   `json:"standingPermissions"`
	Preferences         []string   `json:"preferences"`
	RelationshipRules   []string   `json:"relationshipRules"`
	FinancialBoundaries []string   `json:"financialBoundaries"`
	CommunicationRules  []string   `json:"communicationRules"`
	EscalationRules     []string   `json:"escalationRules"`
	ProtectedRules      []string   `json:"protectedRules"`
	ChangeSummary       string     `json:"changeSummary,omitempty"`
	ApprovedBy          string     `json:"approvedBy,omitempty"`
	ApprovedAt          *time.Time `json:"approvedAt,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
}

// ConstitutionHistoryEntry exposes immutable version metadata without
// returning the full governance contract or the private approval note.
type ConstitutionHistoryEntry struct {
	ID            string     `json:"id"`
	Version       int        `json:"version"`
	BaseVersion   int        `json:"baseVersion"`
	Status        string     `json:"status"`
	ChangeSummary string     `json:"changeSummary"`
	ApprovedBy    string     `json:"approvedBy,omitempty"`
	ApprovedAt    *time.Time `json:"approvedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	Digest        string     `json:"digest"`
}

type ConstitutionHistoryPage struct {
	History   []ConstitutionHistoryEntry `json:"history"`
	Limit     int                        `json:"limit"`
	Truncated bool                       `json:"truncated"`
}

type ConstitutionDraftRequest struct {
	BaseVersion         int      `json:"baseVersion,omitempty"`
	Values              []string `json:"values"`
	Prohibitions        []string `json:"prohibitions"`
	StandingPermissions []string `json:"standingPermissions"`
	Preferences         []string `json:"preferences"`
	RelationshipRules   []string `json:"relationshipRules"`
	FinancialBoundaries []string `json:"financialBoundaries"`
	CommunicationRules  []string `json:"communicationRules"`
	EscalationRules     []string `json:"escalationRules"`
	ChangeSummary       string   `json:"changeSummary"`
}

type ActivateConstitutionRequest struct {
	Confirmation string `json:"confirmation"`
	ApprovalNote string `json:"approvalNote"`
}

type Overview struct {
	GeneratedAt         time.Time      `json:"generatedAt"`
	Total               int            `json:"total"`
	Enabled             int            `json:"enabled"`
	Experimental        int            `json:"experimental"`
	Deprecated          int            `json:"deprecated"`
	Pinned              int            `json:"pinned"`
	Families            map[string]int `json:"families"`
	ConstitutionVersion int            `json:"constitutionVersion"`
	ConstitutionSource  string         `json:"constitutionSource"`
	RecentSelections    int            `json:"recentSelections"`
	SelectionContract   []string       `json:"selectionContract"`
}

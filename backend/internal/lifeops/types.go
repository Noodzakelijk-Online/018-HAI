package lifeops

import (
	"time"

	"github.com/google/uuid"
)

type DomainID string

func (id DomainID) String() string { return string(id) }

const (
	DomainLegalGovernment           DomainID = "legal_government"
	DomainEmergencyContinuity       DomainID = "emergency_continuity"
	DomainHealthWellbeing           DomainID = "health_wellbeing"
	DomainFinancial                 DomainID = "financial"
	DomainWorkVenture               DomainID = "work_venture"
	DomainHomeAssets                DomainID = "home_assets"
	DomainRelationshipsCare         DomainID = "relationships_care"
	DomainLearningGrowth            DomainID = "learning_growth"
	DomainTravelMobility            DomainID = "travel_mobility"
	DomainPersonalProductivity      DomainID = "personal_productivity"
	DomainIdentityRoles             DomainID = "identity_roles"
	DomainFamilyHousehold           DomainID = "family_household"
	DomainFoodNutrition             DomainID = "food_nutrition"
	DomainCommunication             DomainID = "communication_correspondence"
	DomainDigitalAccounts           DomainID = "digital_accounts"
	DomainPossessionsInventory      DomainID = "possessions_inventory"
	DomainAnimalsDependants         DomainID = "animals_dependants"
	DomainCommunityCivic            DomainID = "community_civic"
	DomainLeisureRecreation         DomainID = "leisure_recreation"
	DomainCreativityExpression      DomainID = "creativity_expression"
	DomainMeaningValues             DomainID = "meaning_values"
	DomainEnvironmentSustainability DomainID = "environment_sustainability"
	DomainLegacyLongTerm            DomainID = "legacy_long_term"
	DomainSafetySecurity            DomainID = "safety_security"
)

type LifeDomain struct {
	ID          DomainID `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	NeedClass   string   `json:"needClass"`
	Sensitive   bool     `json:"sensitive"`
}

type EntityDomainLink struct {
	ID                 uuid.UUID `json:"id"`
	OwnerIdentity      string    `json:"ownerIdentity"`
	EntityType         string    `json:"entityType"`
	EntityID           string    `json:"entityId"`
	DomainID           DomainID  `json:"domainId"`
	Primary            bool      `json:"primary"`
	Confidence         float64   `json:"confidence"`
	SourceLabel        string    `json:"sourceLabel"`
	SourceURI          string    `json:"sourceUri,omitempty"`
	Evidence           []string  `json:"evidence"`
	VerificationStatus string    `json:"verificationStatus"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type LinkEntityRequest struct {
	OwnerIdentity      string   `json:"-"`
	EntityType         string   `json:"entityType"`
	EntityID           string   `json:"entityId"`
	DomainID           DomainID `json:"domainId"`
	Primary            bool     `json:"primary"`
	Confidence         float64  `json:"confidence"`
	SourceLabel        string   `json:"sourceLabel"`
	SourceURI          string   `json:"sourceUri,omitempty"`
	Evidence           []string `json:"evidence,omitempty"`
	VerificationStatus string   `json:"verificationStatus,omitempty"`
}

type NeedObservation struct {
	ID            uuid.UUID  `json:"id"`
	OwnerIdentity string     `json:"ownerIdentity"`
	DomainID      DomainID   `json:"domainId"`
	NeedLevel     string     `json:"needLevel"`
	State         string     `json:"state"`
	CurrentLevel  int        `json:"currentLevel"`
	TargetLevel   int        `json:"targetLevel"`
	Gap           int        `json:"gap"`
	Priority      int        `json:"priority"`
	Confidence    float64    `json:"confidence"`
	Evidence      []string   `json:"evidence"`
	SourceLabel   string     `json:"sourceLabel"`
	SourceURI     string     `json:"sourceUri,omitempty"`
	ObservedAt    time.Time  `json:"observedAt"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	NeedsReview   bool       `json:"needsReview"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type RecordNeedRequest struct {
	OwnerIdentity string     `json:"-"`
	DomainID      DomainID   `json:"domainId"`
	NeedLevel     string     `json:"needLevel"`
	State         string     `json:"state"`
	CurrentLevel  int        `json:"currentLevel"`
	TargetLevel   int        `json:"targetLevel"`
	Priority      int        `json:"priority"`
	Confidence    float64    `json:"confidence"`
	Evidence      []string   `json:"evidence,omitempty"`
	SourceLabel   string     `json:"sourceLabel"`
	SourceURI     string     `json:"sourceUri,omitempty"`
	ObservedAt    time.Time  `json:"observedAt"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	NeedsReview   bool       `json:"needsReview"`
}

const (
	CapacityUnknown     = "unknown"
	CapacityAvailable   = "available"
	CapacityConstrained = "constrained"
	CapacityOverloaded  = "overloaded"
	CapacityUnavailable = "unavailable"
	CapacityRecovering  = "recovering"
)

type CapacitySignals struct {
	Energy                  int      `json:"energy"`
	AttentionQuality        int      `json:"attentionQuality"`
	PainIllnessLoad         int      `json:"painIllnessLoad"`
	SleepQuality            int      `json:"sleepQuality"`
	StressLoad              int      `json:"stressLoad"`
	Mobility                int      `json:"mobility"`
	FinancialLiquidity      int      `json:"financialLiquidity"`
	DeadlinePressure        int      `json:"deadlinePressure"`
	InterruptionSensitivity int      `json:"interruptionSensitivity"`
	RecoveryRequirement     int      `json:"recoveryRequirement"`
	TaskSwitchingCost       int      `json:"taskSwitchingCost"`
	SensoryLoad             int      `json:"sensoryLoad"`
	DecisionFatigue         int      `json:"decisionFatigue"`
	RiskTolerance           int      `json:"riskTolerance"`
	ConfidenceReadiness     int      `json:"confidenceReadiness"`
	Location                string   `json:"location,omitempty"`
	AvailableTools          []string `json:"availableTools,omitempty"`
	AvailableHelpers        []string `json:"availableHelpers,omitempty"`
	WeatherConditions       string   `json:"weatherConditions,omitempty"`
	EnvironmentalConditions string   `json:"environmentalConditions,omitempty"`
	SocialAppropriateness   string   `json:"socialAppropriateness,omitempty"`
}

type CapacitySnapshot struct {
	ID                   uuid.UUID       `json:"id"`
	OwnerIdentity        string          `json:"ownerIdentity"`
	Status               string          `json:"status"`
	Signals              CapacitySignals `json:"signals"`
	TimeAvailableMinutes int             `json:"timeAvailableMinutes"`
	ConcurrentWorkLimit  int             `json:"concurrentWorkLimit"`
	CurrentLoad          int             `json:"currentLoad"`
	PlanningStepLimit    int             `json:"planningStepLimit"`
	Constraints          []string        `json:"constraints"`
	SourceLabel          string          `json:"sourceLabel"`
	SourceURI            string          `json:"sourceUri,omitempty"`
	CapturedAt           time.Time       `json:"capturedAt"`
	Confidence           float64         `json:"confidence"`
	Fresh                bool            `json:"fresh"`
	NeedsReview          bool            `json:"needsReview"`
	CreatedAt            time.Time       `json:"createdAt"`
}

type RecordCapacityRequest struct {
	OwnerIdentity        string          `json:"-"`
	Status               string          `json:"status"`
	Signals              CapacitySignals `json:"signals"`
	TimeAvailableMinutes int             `json:"timeAvailableMinutes"`
	ConcurrentWorkLimit  int             `json:"concurrentWorkLimit"`
	CurrentLoad          int             `json:"currentLoad"`
	PlanningStepLimit    int             `json:"planningStepLimit,omitempty"`
	Constraints          []string        `json:"constraints,omitempty"`
	SourceLabel          string          `json:"sourceLabel"`
	SourceURI            string          `json:"sourceUri,omitempty"`
	CapturedAt           time.Time       `json:"capturedAt"`
	Confidence           float64         `json:"confidence"`
	NeedsReview          bool            `json:"needsReview"`
}

type GoalLevel string

const (
	GoalLevelValues           GoalLevel = "values_principles"
	GoalLevelNeeds            GoalLevel = "needs_responsibilities"
	GoalLevelVision           GoalLevel = "vision_future_state"
	GoalLevelStrategicOutcome GoalLevel = "strategic_outcome"
	GoalLevelPursuit          GoalLevel = "pursuit"
	GoalLevelProgrammeCase    GoalLevel = "programme_case"
	GoalLevelProject          GoalLevel = "project"
	GoalLevelWorkflow         GoalLevel = "workflow"
	GoalLevelTask             GoalLevel = "task"
	GoalLevelAtomicAction     GoalLevel = "atomic_action"
	GoalLevelVerification     GoalLevel = "verification_condition"
	GoalLevelMeasuredOutcome  GoalLevel = "measured_outcome"
)

type GoalNode struct {
	ID              uuid.UUID  `json:"id"`
	OwnerIdentity   string     `json:"ownerIdentity"`
	ParentID        *uuid.UUID `json:"parentId,omitempty"`
	Level           GoalLevel  `json:"level"`
	DomainIDs       []DomainID `json:"domainIds"`
	Title           string     `json:"title"`
	Description     string     `json:"description,omitempty"`
	SuccessCriteria []string   `json:"successCriteria"`
	StopConditions  []string   `json:"stopConditions"`
	Status          string     `json:"status"`
	Confidence      float64    `json:"confidence"`
	SourceLabel     string     `json:"sourceLabel"`
	SourceURI       string     `json:"sourceUri,omitempty"`
	TargetAt        *time.Time `json:"targetAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type CreateGoalRequest struct {
	OwnerIdentity   string     `json:"-"`
	ParentID        *uuid.UUID `json:"parentId,omitempty"`
	Level           GoalLevel  `json:"level"`
	DomainIDs       []DomainID `json:"domainIds"`
	Title           string     `json:"title"`
	Description     string     `json:"description,omitempty"`
	SuccessCriteria []string   `json:"successCriteria,omitempty"`
	StopConditions  []string   `json:"stopConditions,omitempty"`
	Status          string     `json:"status,omitempty"`
	Confidence      float64    `json:"confidence"`
	SourceLabel     string     `json:"sourceLabel"`
	SourceURI       string     `json:"sourceUri,omitempty"`
	TargetAt        *time.Time `json:"targetAt,omitempty"`
}

type UpdateGoalRequest struct {
	ParentID        *uuid.UUID `json:"parentId,omitempty"`
	ClearParent     bool       `json:"clearParent,omitempty"`
	Level           *GoalLevel `json:"level,omitempty"`
	DomainIDs       []DomainID `json:"domainIds,omitempty"`
	Title           *string    `json:"title,omitempty"`
	Description     *string    `json:"description,omitempty"`
	SuccessCriteria []string   `json:"successCriteria,omitempty"`
	StopConditions  []string   `json:"stopConditions,omitempty"`
	Status          *string    `json:"status,omitempty"`
	Confidence      *float64   `json:"confidence,omitempty"`
	SourceLabel     *string    `json:"sourceLabel,omitempty"`
	SourceURI       *string    `json:"sourceUri,omitempty"`
	TargetAt        *time.Time `json:"targetAt,omitempty"`
	ClearTarget     bool       `json:"clearTarget,omitempty"`
}

type GoalTreeNode struct {
	Goal     GoalNode       `json:"goal"`
	Children []GoalTreeNode `json:"children"`
}

type PriorityFactors struct {
	Importance               int `json:"importance"`
	Urgency                  int `json:"urgency"`
	HumanNeedAffected        int `json:"humanNeedAffected"`
	DeadlinePressure         int `json:"deadlinePressure"`
	CostOfDelay              int `json:"costOfDelay"`
	ExpectedValue            int `json:"expectedValue"`
	HarmAvoided              int `json:"harmAvoided"`
	ProbabilityOfSuccess     int `json:"probabilityOfSuccess"`
	Effort                   int `json:"effort"`
	Duration                 int `json:"duration"`
	Dependencies             int `json:"dependencies"`
	Reversibility            int `json:"reversibility"`
	Risk                     int `json:"risk"`
	LegalObligation          int `json:"legalObligation"`
	RelationshipConsequences int `json:"relationshipConsequences"`
	AvailableCapacity        int `json:"availableCapacity"`
	EnergyFit                int `json:"energyFit"`
	OpportunityCost          int `json:"opportunityCost"`
	StrategicAlignment       int `json:"strategicAlignment"`
	LearningValue            int `json:"learningValue"`
	CompoundingValue         int `json:"compoundingValue"`
	Staleness                int `json:"staleness"`
	CommitmentAge            int `json:"commitmentAge"`
	PeopleBlocked            int `json:"peopleBlocked"`
	Delegability             int `json:"delegability"`
}

type PriorityAssessmentRequest struct {
	OwnerIdentity string            `json:"-"`
	EntityType    string            `json:"entityType"`
	EntityID      string            `json:"entityId"`
	Title         string            `json:"title"`
	Deadline      *time.Time        `json:"deadline,omitempty"`
	Factors       PriorityFactors   `json:"factors"`
	Capacity      *CapacitySnapshot `json:"capacity,omitempty"`
	SourceLabel   string            `json:"sourceLabel,omitempty"`
	SourceURI     string            `json:"sourceUri,omitempty"`
}

type FactorContribution struct {
	Factor         string  `json:"factor"`
	Input          int     `json:"input"`
	EffectiveInput int     `json:"effectiveInput"`
	Weight         float64 `json:"weight"`
	Contribution   float64 `json:"contribution"`
	CostFactor     bool    `json:"costFactor"`
	Reason         string  `json:"reason"`
}

type PriorityAssessment struct {
	ID               uuid.UUID            `json:"id"`
	OwnerIdentity    string               `json:"ownerIdentity"`
	EntityType       string               `json:"entityType"`
	EntityID         string               `json:"entityId"`
	Title            string               `json:"title"`
	Score            int                  `json:"score"`
	Band             string               `json:"band"`
	Factors          PriorityFactors      `json:"factors"`
	Contributions    []FactorContribution `json:"contributions"`
	Reasons          []string             `json:"reasons"`
	CapacityApplied  bool                 `json:"capacityApplied"`
	AlgorithmVersion string               `json:"algorithmVersion"`
	SourceLabel      string               `json:"sourceLabel"`
	SourceURI        string               `json:"sourceUri,omitempty"`
	AssessedAt       time.Time            `json:"assessedAt"`
}

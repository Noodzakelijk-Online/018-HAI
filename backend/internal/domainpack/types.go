package domainpack

import "time"

const CatalogVersion = "1.0.0"

type PackID string

const (
	PackLegalGovernment           PackID = "legal_government"
	PackEmergencyContinuity       PackID = "emergency_continuity"
	PackHealthWellbeing           PackID = "health_wellbeing"
	PackFinancial                 PackID = "financial"
	PackWorkVenture               PackID = "work_venture"
	PackHomeAssets                PackID = "home_assets"
	PackRelationshipsCare         PackID = "relationships_care"
	PackLearningGrowth            PackID = "learning_growth"
	PackTravelMobility            PackID = "travel_mobility"
	PackPersonalProductivity      PackID = "personal_productivity"
	PackFamilyHousehold           PackID = "family_household"
	PackFoodNutrition             PackID = "food_nutrition"
	PackCommunication             PackID = "communication_correspondence"
	PackDigitalAccounts           PackID = "digital_accounts"
	PackPossessionsInventory      PackID = "possessions_inventory"
	PackAnimalsDependants         PackID = "animals_dependants"
	PackCommunityCivic            PackID = "community_civic"
	PackLeisure                   PackID = "leisure"
	PackCreativity                PackID = "creativity"
	PackMeaningValues             PackID = "meaning_values"
	PackEnvironmentSustainability PackID = "environment_sustainability"
	PackLegacyLongTerm            PackID = "legacy_long_term"
	PackSafetySecurity            PackID = "safety_security"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type SignalStrength int

const (
	SignalWeak SignalStrength = iota + 1
	SignalModerate
	SignalStrong
)

type ClassificationSignal struct {
	Phrase   string         `json:"phrase"`
	Strength SignalStrength `json:"strength"`
	Reason   string         `json:"reason"`
}

type IntakeQuestion struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Required bool   `json:"required"`
}

type RiskTrigger struct {
	ID          string    `json:"id"`
	Signal      string    `json:"signal"`
	Level       RiskLevel `json:"level"`
	Explanation string    `json:"explanation"`
}

type ApprovalRule struct {
	Action      string    `json:"action"`
	Required    bool      `json:"required"`
	MinimumRisk RiskLevel `json:"minimumRisk"`
	Reason      string    `json:"reason"`
}

type ProhibitedAction struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

type SourceAuthorityRule struct {
	ClaimType       string   `json:"claimType"`
	AcceptedSources []string `json:"acceptedSources"`
	MinimumSources  int      `json:"minimumSources"`
	Reason          string   `json:"reason"`
}

type EvidenceRequirement struct {
	ID                  string   `json:"id"`
	Description         string   `json:"description"`
	RequiredForActions  []string `json:"requiredForActions"`
	MinimumVerification string   `json:"minimumVerification"`
}

type DeterministicValidator struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

type SuccessCriteriaTemplate struct {
	ID       string   `json:"id"`
	Criteria []string `json:"criteria"`
}

type StopCondition struct {
	ID         string    `json:"id"`
	Condition  string    `json:"condition"`
	EscalateTo string    `json:"escalateTo"`
	Level      RiskLevel `json:"level"`
}

type RetentionPolicy struct {
	DefaultDays       int  `json:"defaultDays"`
	LocalOnly         bool `json:"localOnly"`
	DeletionReview    bool `json:"deletionReview"`
	ArchiveProvenance bool `json:"archiveProvenance"`
}

type DomainPack struct {
	ID                          PackID                    `json:"id"`
	Version                     string                    `json:"version"`
	Name                        string                    `json:"name"`
	Description                 string                    `json:"description"`
	Sensitive                   bool                      `json:"sensitive"`
	DefaultEnabled              bool                      `json:"defaultEnabled"`
	ClassificationSignals       []ClassificationSignal    `json:"classificationSignals"`
	IntakeQuestions             []IntakeQuestion          `json:"intakeQuestions"`
	CommonEntities              []string                  `json:"commonEntities"`
	RiskTriggers                []RiskTrigger             `json:"riskTriggers"`
	ApprovalRules               []ApprovalRule            `json:"approvalRules"`
	ProhibitedAutonomousActions []ProhibitedAction        `json:"prohibitedAutonomousActions"`
	SourceAuthorityRules        []SourceAuthorityRule     `json:"sourceAuthorityRules"`
	EvidenceRequirements        []EvidenceRequirement     `json:"evidenceRequirements"`
	DeterministicValidators     []DeterministicValidator  `json:"deterministicValidators"`
	SuccessCriteriaTemplates    []SuccessCriteriaTemplate `json:"successCriteriaTemplates"`
	StopEscalationConditions    []StopCondition           `json:"stopEscalationConditions"`
	Retention                   RetentionPolicy           `json:"retention"`
	SuitableAgentCapabilities   []string                  `json:"suitableAgentCapabilities"`
	AuditEvents                 []string                  `json:"auditEvents"`
}

type CatalogMetadata struct {
	Version   string `json:"version"`
	Digest    string `json:"digest"`
	PackCount int    `json:"packCount"`
}

type PackPreference struct {
	OwnerIdentity       string    `json:"ownerIdentity"`
	PackID              PackID    `json:"packId"`
	Enabled             *bool     `json:"enabled,omitempty"`
	ClassificationBoost int       `json:"classificationBoost"`
	ForceLocalOnly      bool      `json:"forceLocalOnly"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type PackView struct {
	Pack       DomainPack      `json:"pack"`
	Preference *PackPreference `json:"preference,omitempty"`
	Enabled    bool            `json:"enabled"`
	LocalOnly  bool            `json:"localOnly"`
}

type ClassificationRequest struct {
	Text            string              `json:"text"`
	ExplicitPackIDs []PackID            `json:"explicitPackIds,omitempty"`
	ExplicitSignals map[string][]string `json:"explicitSignals,omitempty"`
	OwnerIdentity   string              `json:"-"`
}

type SignalMatch struct {
	Signal   string         `json:"signal"`
	Strength SignalStrength `json:"strength"`
	Score    int            `json:"score"`
	Reason   string         `json:"reason"`
}

type ClassificationMatch struct {
	PackID    PackID        `json:"packId"`
	Score     int           `json:"score"`
	Explicit  bool          `json:"explicit"`
	Sensitive bool          `json:"sensitive"`
	Reasons   []string      `json:"reasons"`
	Signals   []SignalMatch `json:"signals"`
}

type SuppressedMatch struct {
	PackID  PackID   `json:"packId"`
	Reason  string   `json:"reason"`
	Signals []string `json:"signals"`
}

type ClassificationResult struct {
	Matches    []ClassificationMatch `json:"matches"`
	Suppressed []SuppressedMatch     `json:"suppressed"`
}

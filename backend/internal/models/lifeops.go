package models

import (
	"time"

	"github.com/google/uuid"
)

// LifeEntityDomainLink maps an owner-scoped operational entity to one
// canonical life domain. The unique key prevents duplicate classifications.
type LifeEntityDomainLink struct {
	ID                 uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	OwnerIdentity      string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_life_entity_domain_links_scope" json:"-"`
	EntityType         string    `gorm:"type:varchar(80);not null;uniqueIndex:uq_life_entity_domain_links_scope" json:"entityType"`
	EntityID           string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_life_entity_domain_links_scope" json:"entityId"`
	DomainID           string    `gorm:"type:varchar(80);not null;uniqueIndex:uq_life_entity_domain_links_scope;index" json:"domainId"`
	Primary            bool      `gorm:"not null;default:false" json:"primary"`
	Confidence         float64   `gorm:"type:numeric(5,4);not null" json:"confidence"`
	SourceLabel        string    `gorm:"type:varchar(255);not null" json:"sourceLabel"`
	SourceURI          string    `gorm:"type:text;not null;default:''" json:"sourceUri,omitempty"`
	EvidenceJSON       string    `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	VerificationStatus string    `gorm:"type:varchar(40);not null;index" json:"verificationStatus"`
	CreatedAt          time.Time `gorm:"not null" json:"createdAt"`
	UpdatedAt          time.Time `gorm:"not null" json:"updatedAt"`
}

func (LifeEntityDomainLink) TableName() string { return "life_entity_domain_links" }

// LifeNeedObservation is append-only evidence about an owner's current need
// state. Superseding observations are added rather than rewriting history.
type LifeNeedObservation struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key" json:"id"`
	OwnerIdentity string     `gorm:"type:varchar(255);not null;index:idx_life_need_observations_owner_observed,priority:1" json:"-"`
	DomainID      string     `gorm:"type:varchar(80);not null;index" json:"domainId"`
	NeedLevel     string     `gorm:"type:varchar(120);not null" json:"needLevel"`
	State         string     `gorm:"type:varchar(40);not null;index" json:"state"`
	CurrentLevel  int        `gorm:"type:smallint;not null" json:"currentLevel"`
	TargetLevel   int        `gorm:"type:smallint;not null" json:"targetLevel"`
	Gap           int        `gorm:"type:smallint;not null" json:"gap"`
	Priority      int        `gorm:"type:smallint;not null" json:"priority"`
	Confidence    float64    `gorm:"type:numeric(5,4);not null" json:"confidence"`
	EvidenceJSON  string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	SourceLabel   string     `gorm:"type:varchar(255);not null" json:"sourceLabel"`
	SourceURI     string     `gorm:"type:text;not null;default:''" json:"sourceUri,omitempty"`
	ObservedAt    time.Time  `gorm:"not null;index:idx_life_need_observations_owner_observed,priority:2,sort:desc" json:"observedAt"`
	ExpiresAt     *time.Time `gorm:"index" json:"expiresAt,omitempty"`
	NeedsReview   bool       `gorm:"not null;default:false;index" json:"needsReview"`
	CreatedAt     time.Time  `gorm:"not null" json:"createdAt"`
}

func (LifeNeedObservation) TableName() string { return "life_need_observations" }

// LifeCapacitySnapshot is append-only and captures the human operating
// constraints that planning must honor.
type LifeCapacitySnapshot struct {
	ID                   uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	OwnerIdentity        string    `gorm:"type:varchar(255);not null;index:idx_life_capacity_snapshots_owner_captured,priority:1" json:"-"`
	Status               string    `gorm:"type:varchar(40);not null;index" json:"status"`
	SignalsJSON          string    `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	TimeAvailableMinutes int       `gorm:"not null;default:0" json:"timeAvailableMinutes"`
	ConcurrentWorkLimit  int       `gorm:"not null;default:0" json:"concurrentWorkLimit"`
	CurrentLoad          int       `gorm:"type:smallint;not null;default:0" json:"currentLoad"`
	PlanningStepLimit    int       `gorm:"type:smallint;not null;default:1" json:"planningStepLimit"`
	ConstraintsJSON      string    `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	SourceLabel          string    `gorm:"type:varchar(255);not null" json:"sourceLabel"`
	SourceURI            string    `gorm:"type:text;not null;default:''" json:"sourceUri,omitempty"`
	CapturedAt           time.Time `gorm:"not null;index:idx_life_capacity_snapshots_owner_captured,priority:2,sort:desc" json:"capturedAt"`
	Confidence           float64   `gorm:"type:numeric(5,4);not null" json:"confidence"`
	Fresh                bool      `gorm:"not null;default:false" json:"fresh"`
	NeedsReview          bool      `gorm:"not null;default:false;index" json:"needsReview"`
	CreatedAt            time.Time `gorm:"not null" json:"createdAt"`
}

func (LifeCapacitySnapshot) TableName() string { return "life_capacity_snapshots" }

// LifeGoalNode is the durable hierarchy from values down to measured outcome.
// Success criteria and stop conditions remain explicit at executable levels.
type LifeGoalNode struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primary_key" json:"id"`
	OwnerIdentity       string     `gorm:"type:varchar(255);not null;index:idx_life_goal_nodes_owner_level,priority:1" json:"-"`
	ParentID            *uuid.UUID `gorm:"type:uuid;index" json:"parentId,omitempty"`
	Level               string     `gorm:"type:varchar(80);not null;index:idx_life_goal_nodes_owner_level,priority:2" json:"level"`
	DomainIDsJSON       string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	Title               string     `gorm:"type:varchar(500);not null" json:"title"`
	Description         string     `gorm:"type:text;not null;default:''" json:"description,omitempty"`
	SuccessCriteriaJSON string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	StopConditionsJSON  string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	Status              string     `gorm:"type:varchar(40);not null;index" json:"status"`
	Confidence          float64    `gorm:"type:numeric(5,4);not null" json:"confidence"`
	SourceLabel         string     `gorm:"type:varchar(255);not null" json:"sourceLabel"`
	SourceURI           string     `gorm:"type:text;not null;default:''" json:"sourceUri,omitempty"`
	TargetAt            *time.Time `gorm:"index" json:"targetAt,omitempty"`
	CreatedAt           time.Time  `gorm:"not null" json:"createdAt"`
	UpdatedAt           time.Time  `gorm:"not null;index" json:"updatedAt"`
}

func (LifeGoalNode) TableName() string { return "life_goal_nodes" }

// LifePriorityAssessment preserves the complete explainable MCDA input and
// contribution trace used to rank an owner-scoped operational entity.
type LifePriorityAssessment struct {
	ID                uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	OwnerIdentity     string    `gorm:"type:varchar(255);not null;index:idx_life_priority_owner_assessed,priority:1" json:"-"`
	EntityType        string    `gorm:"type:varchar(80);not null;index:idx_life_priority_entity,priority:2" json:"entityType"`
	EntityID          string    `gorm:"type:varchar(255);not null;index:idx_life_priority_entity,priority:3" json:"entityId"`
	Title             string    `gorm:"type:varchar(500);not null" json:"title"`
	Score             int       `gorm:"type:smallint;not null;index" json:"score"`
	Band              string    `gorm:"type:varchar(40);not null;index" json:"band"`
	FactorsJSON       string    `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	ContributionsJSON string    `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	ReasonsJSON       string    `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	CapacityApplied   bool      `gorm:"not null;default:false" json:"capacityApplied"`
	AlgorithmVersion  string    `gorm:"type:varchar(80);not null" json:"algorithmVersion"`
	SourceLabel       string    `gorm:"type:varchar(255);not null" json:"sourceLabel"`
	SourceURI         string    `gorm:"type:text;not null;default:''" json:"sourceUri,omitempty"`
	AssessedAt        time.Time `gorm:"not null;index:idx_life_priority_owner_assessed,priority:2,sort:desc" json:"assessedAt"`
}

func (LifePriorityAssessment) TableName() string { return "life_priority_assessments" }

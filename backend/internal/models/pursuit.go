package models

import (
	"time"

	"github.com/google/uuid"
)

type Pursuit struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	OwnerIdentity         string     `gorm:"type:varchar(255);index" json:"ownerIdentity,omitempty"`
	Title                 string     `gorm:"type:varchar(512);index;not null" json:"title"`
	Description           string     `gorm:"type:text" json:"description,omitempty"`
	WhyItMatters          string     `gorm:"type:text" json:"whyItMatters,omitempty"`
	ProjectKey            string     `gorm:"type:varchar(255);index" json:"projectKey,omitempty"`
	Domain                string     `gorm:"type:varchar(120);index" json:"domain,omitempty"`
	DesiredOutcome        string     `gorm:"type:text" json:"desiredOutcome,omitempty"`
	CurrentStateSummary   string     `gorm:"type:text" json:"currentStateSummary,omitempty"`
	Status                string     `gorm:"type:varchar(80);index;not null" json:"status"`
	PriorityScore         int        `gorm:"index" json:"priorityScore"`
	RiskLevel             string     `gorm:"type:varchar(80);index" json:"riskLevel"`
	Confidence            float64    `json:"confidence"`
	AutonomyLevel         string     `gorm:"type:varchar(80);index" json:"autonomyLevel"`
	NeedCategory          string     `gorm:"type:varchar(120);index" json:"needCategory,omitempty"`
	SourceOfCreation      string     `gorm:"type:varchar(120);index" json:"sourceOfCreation,omitempty"`
	NextRecommendedAction string     `gorm:"type:text" json:"nextRecommendedAction,omitempty"`
	CompletionDefinition  string     `gorm:"type:text" json:"completionDefinition,omitempty"`
	CompletionState       string     `gorm:"type:varchar(80);index" json:"completionState"`
	LastActivityAt        *time.Time `gorm:"index" json:"lastActivityAt,omitempty"`
	NextReviewAt          *time.Time `gorm:"index" json:"nextReviewAt,omitempty"`
	Archived              bool       `gorm:"default:false;index" json:"archived"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

type PursuitLink struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	PursuitID    uuid.UUID `gorm:"type:uuid;index;not null;uniqueIndex:pursuit_link_unique" json:"pursuitId"`
	LinkType     string    `gorm:"type:varchar(80);index;not null;uniqueIndex:pursuit_link_unique" json:"linkType"`
	LinkID       string    `gorm:"type:varchar(120);index;not null;uniqueIndex:pursuit_link_unique" json:"linkId"`
	Relationship string    `gorm:"type:varchar(80);index;not null;uniqueIndex:pursuit_link_unique" json:"relationship"`
	SourceURI    string    `gorm:"type:varchar(1024);index" json:"sourceUri,omitempty"`
	SourceLabel  string    `gorm:"type:varchar(512)" json:"sourceLabel,omitempty"`
	Confidence   float64   `json:"confidence"`
	CreatedAt    time.Time `json:"createdAt"`
}

type PursuitActivity struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	PursuitID  uuid.UUID `gorm:"type:uuid;index;not null" json:"pursuitId"`
	EventType  string    `gorm:"type:varchar(80);index;not null" json:"eventType"`
	Message    string    `gorm:"type:text" json:"message,omitempty"`
	Actor      string    `gorm:"type:varchar(120)" json:"actor,omitempty"`
	SourceType string    `gorm:"type:varchar(80);index" json:"sourceType,omitempty"`
	SourceID   string    `gorm:"type:varchar(120);index" json:"sourceId,omitempty"`
	SourceURI  string    `gorm:"type:varchar(1024)" json:"sourceUri,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// PursuitTaskAttempt is a compact, durable projection of a direct task-engine
// plan or run. It intentionally excludes retrieved source context and model
// output; those remain in their existing protected stores. Workflow-owned task
// runs stay on WorkflowItem and are aggregated separately by the pursuit view.
type PursuitTaskAttempt struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	PursuitID          uuid.UUID  `gorm:"type:uuid;index;not null" json:"pursuitId"`
	TaskPlanID         string     `gorm:"type:varchar(120);uniqueIndex;not null" json:"taskPlanId"`
	OwnerIdentity      string     `gorm:"type:varchar(255);index" json:"ownerIdentity,omitempty"`
	RequestSummary     string     `gorm:"type:text" json:"requestSummary,omitempty"`
	ProjectKey         string     `gorm:"type:varchar(255);index" json:"projectKey,omitempty"`
	Mode               string     `gorm:"type:varchar(40);index;not null" json:"mode"`
	Status             string     `gorm:"type:varchar(80);index;not null" json:"status"`
	RiskLevel          string     `gorm:"type:varchar(80);index" json:"riskLevel,omitempty"`
	VerificationStatus string     `gorm:"type:varchar(80);index" json:"verificationStatus,omitempty"`
	AutomationID       string     `gorm:"type:varchar(120);index" json:"automationId,omitempty"`
	LaunchEventID      string     `gorm:"type:varchar(120);index" json:"launchEventId,omitempty"`
	BlockedReason      string     `gorm:"type:text" json:"blockedReason,omitempty"`
	StartedAt          *time.Time `gorm:"index" json:"startedAt,omitempty"`
	CompletedAt        *time.Time `gorm:"index" json:"completedAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

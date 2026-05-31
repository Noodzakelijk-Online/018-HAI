package models

import (
	"time"

	"github.com/google/uuid"
)

type WorkflowItem struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	Title            string     `gorm:"type:varchar(512);index;not null" json:"title"`
	Description      string     `gorm:"type:text" json:"description,omitempty"`
	ProjectKey       string     `gorm:"type:varchar(255);index" json:"projectKey,omitempty"`
	CurrentState     string     `gorm:"type:varchar(80);index;not null" json:"currentState"`
	TaskType         string     `gorm:"type:varchar(80);index" json:"taskType"`
	RiskLevel        string     `gorm:"type:varchar(80);index" json:"riskLevel"`
	PriorityScore    int        `gorm:"index" json:"priorityScore"`
	Confidence       float64    `json:"confidence"`
	AutonomyLevel    string     `gorm:"type:varchar(80);index" json:"autonomyLevel"`
	RequiresApproval bool       `gorm:"index" json:"requiresApproval"`
	ApprovalReason   string     `gorm:"type:text" json:"approvalReason,omitempty"`
	BlockedReason    string     `gorm:"type:text" json:"blockedReason,omitempty"`
	NextAction       string     `gorm:"type:text" json:"nextAction,omitempty"`
	SourceType       string     `gorm:"type:varchar(80);index" json:"sourceType,omitempty"`
	SourceURI        string     `gorm:"type:varchar(1024)" json:"sourceUri,omitempty"`
	SourceLabel      string     `gorm:"type:varchar(512)" json:"sourceLabel,omitempty"`
	DueAt            *time.Time `gorm:"index" json:"dueAt,omitempty"`
	Archived         bool       `gorm:"default:false;index" json:"archived"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type WorkflowChecklistItem struct {
	ID               uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	WorkflowID       uuid.UUID `gorm:"type:uuid;index;not null" json:"workflowId"`
	Label            string    `gorm:"type:text;not null" json:"label"`
	Status           string    `gorm:"type:varchar(50);index;not null" json:"status"`
	Position         int       `gorm:"index" json:"position"`
	RequiresApproval bool      `gorm:"index" json:"requiresApproval"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type WorkflowEvent struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	WorkflowID  uuid.UUID `gorm:"type:uuid;index;not null" json:"workflowId"`
	EventType   string    `gorm:"type:varchar(80);index;not null" json:"eventType"`
	FromState   string    `gorm:"type:varchar(80);index" json:"fromState,omitempty"`
	ToState     string    `gorm:"type:varchar(80);index" json:"toState,omitempty"`
	Message     string    `gorm:"type:text" json:"message,omitempty"`
	Trigger     string    `gorm:"type:varchar(255)" json:"trigger,omitempty"`
	RuleApplied string    `gorm:"type:text" json:"ruleApplied,omitempty"`
	SourceURI   string    `gorm:"type:varchar(1024)" json:"sourceUri,omitempty"`
	Actor       string    `gorm:"type:varchar(120)" json:"actor,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

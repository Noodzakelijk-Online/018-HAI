package models

import (
	"time"

	"github.com/google/uuid"
)

// TemporalWorkflowRun is HAI's owner-scoped audit mirror for a deliberately
// narrow Temporal workflow. It stores identifiers and bounded result metadata,
// never workflow source text, credentials, or external-action payloads.
type TemporalWorkflowRun struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	OwnerIdentity      string     `gorm:"type:text;not null;index:idx_temporal_owner_created" json:"ownerIdentity"`
	TemporalWorkflowID string     `gorm:"type:text;not null;uniqueIndex" json:"temporalWorkflowId"`
	WorkflowType       string     `gorm:"type:text;not null" json:"workflowType"`
	Status             string     `gorm:"type:text;not null;index" json:"status"`
	ScheduledFor       time.Time  `gorm:"not null" json:"scheduledFor"`
	StartedAt          *time.Time `json:"startedAt,omitempty"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`
	Summary            string     `gorm:"type:text;not null" json:"summary"`
	ResultJSON         string     `gorm:"type:jsonb;not null;default:'{}'" json:"result"`
	CreatedAt          time.Time  `gorm:"index:idx_temporal_owner_created" json:"createdAt"`
	UpdatedAt          time.Time  `gorm:"index:idx_temporal_owner_created" json:"updatedAt"`
}

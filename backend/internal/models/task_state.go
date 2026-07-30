package models

import (
	"time"

	"github.com/google/uuid"
)

// TaskCompletionPlanLog is an append-only, redacted snapshot of one task plan.
// PostgreSQL rejects updates, deletes, and truncation so completion claims keep
// the exact plan and verification state that produced them.
type TaskCompletionPlanLog struct {
	ID                 uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	OwnerIdentity      string    `gorm:"type:varchar(255);not null;index:idx_task_completion_plan_logs_owner_created,priority:1" json:"-"`
	TaskPlanID         string    `gorm:"type:varchar(160);not null;index" json:"taskPlanId"`
	CompletionStatus   string    `gorm:"type:varchar(80);not null;index" json:"completionStatus"`
	VerificationStatus string    `gorm:"type:varchar(80);not null;default:'not_run';index" json:"verificationStatus"`
	PayloadJSON        string    `gorm:"type:jsonb;not null" json:"-"`
	PayloadDigest      string    `gorm:"type:char(64);not null;index" json:"payloadDigest"`
	ProvenanceSource   string    `gorm:"type:varchar(80);not null" json:"provenanceSource"`
	CreatedAt          time.Time `gorm:"not null;index:idx_task_completion_plan_logs_owner_created,priority:2,sort:desc" json:"createdAt"`
}

func (TaskCompletionPlanLog) TableName() string { return "task_completion_plan_logs" }

// TaskReviewItemRecord is the durable queue state for an action requiring
// human review. RequestJSON and RequestDigest are immutable provenance. Queue
// state may advance, but it cannot be rebound to a different action or owner.
type TaskReviewItemRecord struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primary_key" json:"id"`
	OwnerIdentity      string     `gorm:"type:varchar(255);not null;index:idx_task_review_items_owner_created,priority:1" json:"-"`
	OriginalTaskPlanID string     `gorm:"type:varchar(160);not null;index" json:"originalTaskPlanId"`
	CurrentTaskPlanID  string     `gorm:"type:varchar(160);not null;index" json:"currentTaskPlanId"`
	RequestDigest      string     `gorm:"type:char(64);not null;index" json:"requestDigest"`
	RequestJSON        string     `gorm:"type:jsonb;not null" json:"-"`
	Reason             string     `gorm:"type:text;not null" json:"reason"`
	Priority           string     `gorm:"type:varchar(32);not null;default:'normal';index" json:"priority"`
	Status             string     `gorm:"type:varchar(32);not null;index" json:"status"`
	ReviewRevision     int        `gorm:"not null;default:1" json:"reviewRevision"`
	CreatedAt          time.Time  `gorm:"not null;index:idx_task_review_items_owner_created,priority:2,sort:desc" json:"createdAt"`
	UpdatedAt          time.Time  `gorm:"not null" json:"updatedAt"`
	ResolvedAt         *time.Time `gorm:"index" json:"resolvedAt,omitempty"`
}

func (TaskReviewItemRecord) TableName() string { return "task_review_items" }

// TaskReviewDecisionRecord is an immutable approval/rejection event. It binds
// the actor, decision, reviewed request digest, task plan, and trusted source so
// later execution can prove exactly which action received owner approval.
type TaskReviewDecisionRecord struct {
	ID               uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	ReviewItemID     uuid.UUID `gorm:"type:uuid;not null;index:idx_task_review_decisions_item_resolved,priority:1" json:"reviewItemId"`
	ReviewRevision   int       `gorm:"not null" json:"reviewRevision"`
	OwnerIdentity    string    `gorm:"type:varchar(255);not null;index:idx_task_review_decisions_owner_resolved,priority:1" json:"-"`
	TaskPlanID       string    `gorm:"type:varchar(160);not null;index" json:"taskPlanId"`
	Decision         string    `gorm:"type:varchar(32);not null;index" json:"decision"`
	ResolutionNote   string    `gorm:"type:varchar(512);not null;default:''" json:"resolutionNote,omitempty"`
	ResolvedBy       string    `gorm:"type:varchar(255);not null" json:"resolvedBy"`
	ApprovalSource   string    `gorm:"type:varchar(80);not null" json:"approvalSource"`
	ApprovalSourceID string    `gorm:"type:varchar(160);not null;index" json:"approvalSourceId"`
	RequestDigest    string    `gorm:"type:char(64);not null;index" json:"requestDigest"`
	ResolvedAt       time.Time `gorm:"not null;index:idx_task_review_decisions_item_resolved,priority:2,sort:desc;index:idx_task_review_decisions_owner_resolved,priority:2,sort:desc" json:"resolvedAt"`
}

func (TaskReviewDecisionRecord) TableName() string { return "task_review_decisions" }

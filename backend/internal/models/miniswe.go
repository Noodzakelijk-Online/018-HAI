package models

import (
	"time"

	"github.com/google/uuid"
)

// MiniSWEPatchProposal is an audit record for a disposable coding-agent run.
// The generated diff is intentionally not stored: a patch can contain source
// content and is returned only to the requesting owner for manual review.
type MiniSWEPatchProposal struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	OwnerIdentity string     `gorm:"type:varchar(255);index;not null" json:"-"`
	WorkflowID    uuid.UUID  `gorm:"type:uuid;index;not null" json:"workflowId"`
	WorkspaceID   string     `gorm:"type:varchar(64);index;not null" json:"workspaceId"`
	Status        string     `gorm:"type:varchar(40);index;not null" json:"status"`
	Summary       string     `gorm:"type:varchar(512);not null" json:"summary"`
	DiffDigest    string     `gorm:"type:varchar(64)" json:"diffDigest,omitempty"`
	ChangedFiles  int        `json:"changedFiles"`
	DiffTruncated bool       `json:"diffTruncated"`
	CreatedAt     time.Time  `json:"createdAt"`
	CompletedAt   *time.Time `gorm:"index" json:"completedAt,omitempty"`
}

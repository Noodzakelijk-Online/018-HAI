package models

import (
	"time"

	"github.com/google/uuid"
)

type AmbientNeed struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	Key            string    `gorm:"type:varchar(80);uniqueIndex;not null" json:"key"`
	Name           string    `gorm:"type:varchar(120);not null" json:"name"`
	Description    string    `gorm:"type:text" json:"description"`
	CurrentLevel   int       `gorm:"default:0" json:"currentLevel"`
	TargetLevel    int       `gorm:"default:100" json:"targetLevel"`
	PriorityWeight int       `gorm:"default:50" json:"priorityWeight"`
	Enabled        bool      `gorm:"default:true;index" json:"enabled"`
	Notes          string    `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// AmbientNeedOverride stores a private copy of a planning preference. The
// shared AmbientNeed rows remain the system defaults for ownerless workers.
type AmbientNeedOverride struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	OwnerIdentity  string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_ambient_need_override_owner_key" json:"-"`
	NeedKey        string    `gorm:"type:varchar(80);not null;uniqueIndex:idx_ambient_need_override_owner_key" json:"needKey"`
	CurrentLevel   int       `gorm:"default:0" json:"currentLevel"`
	TargetLevel    int       `gorm:"default:100" json:"targetLevel"`
	PriorityWeight int       `gorm:"default:50" json:"priorityWeight"`
	Enabled        bool      `gorm:"default:true;index" json:"enabled"`
	Notes          string    `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type AmbientOpportunity struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	OwnerIdentity    string     `gorm:"type:varchar(255);index" json:"ownerIdentity,omitempty"`
	Fingerprint      string     `gorm:"type:varchar(64);uniqueIndex;not null" json:"fingerprint"`
	WorkflowID       *uuid.UUID `gorm:"type:uuid;index" json:"workflowId,omitempty"`
	NeedKey          string     `gorm:"type:varchar(80);index;not null" json:"needKey"`
	Title            string     `gorm:"type:varchar(512);not null" json:"title"`
	Rationale        string     `gorm:"type:text;not null" json:"rationale"`
	NextAction       string     `gorm:"type:text;not null" json:"nextAction"`
	SourceType       string     `gorm:"type:varchar(80);index" json:"sourceType,omitempty"`
	SourceID         string     `gorm:"type:varchar(160);index" json:"sourceId,omitempty"`
	SourceURI        string     `gorm:"type:varchar(1024)" json:"sourceUri,omitempty"`
	EvidenceManifest string     `gorm:"type:text" json:"evidenceManifest,omitempty"`
	ResolutionNote   string     `gorm:"type:text" json:"resolutionNote,omitempty"`
	PriorityScore    int        `gorm:"index" json:"priorityScore"`
	Urgency          int        `json:"urgency"`
	Impact           int        `json:"impact"`
	Effort           int        `json:"effort"`
	Confidence       int        `json:"confidence"`
	Risk             int        `json:"risk"`
	RequiresApproval bool       `gorm:"index" json:"requiresApproval"`
	Status           string     `gorm:"type:varchar(50);index;not null" json:"status"`
	LastSeenAt       time.Time  `gorm:"index" json:"lastSeenAt"`
	CooldownUntil    *time.Time `gorm:"index" json:"cooldownUntil,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type AmbientScan struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	OwnerIdentity      string     `gorm:"type:varchar(255);index" json:"ownerIdentity,omitempty"`
	Trigger            string     `gorm:"type:varchar(80);index;not null" json:"trigger"`
	Status             string     `gorm:"type:varchar(50);index;not null" json:"status"`
	StartedAt          time.Time  `gorm:"index" json:"startedAt"`
	CompletedAt        *time.Time `gorm:"index" json:"completedAt,omitempty"`
	ItemsExamined      int        `json:"itemsExamined"`
	OpportunitiesFound int        `json:"opportunitiesFound"`
	Created            int        `json:"created"`
	Updated            int        `json:"updated"`
	Deduplicated       int        `json:"deduplicated"`
	Advanced           int        `json:"advanced"`
	Filtered           int        `json:"filtered"`
	Skipped            int        `json:"skipped"`
	Blocked            int        `json:"blocked"`
	ManifestBytes      int64      `json:"manifestBytes"`
	DeduplicatedBytes  int64      `json:"deduplicatedBytes"`
	ErrorMessage       string     `gorm:"type:text" json:"errorMessage,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

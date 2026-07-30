package models

import (
	"time"

	"github.com/google/uuid"
)

// WASIRun records a bounded, manifest-approved local WASI invocation. It never
// stores module input or output because those can contain connected-source data.
type WASIRun struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	OwnerIdentity string     `gorm:"type:text;not null;index:idx_wasi_run_owner_created" json:"ownerIdentity"`
	ModuleID      string     `gorm:"type:text;not null" json:"moduleId"`
	ModuleSHA256  string     `gorm:"type:text;not null" json:"moduleSha256"`
	Status        string     `gorm:"type:text;not null;index" json:"status"`
	Summary       string     `gorm:"type:text;not null" json:"summary"`
	ExitCode      int        `json:"exitCode"`
	CreatedAt     time.Time  `gorm:"index:idx_wasi_run_owner_created" json:"createdAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
}

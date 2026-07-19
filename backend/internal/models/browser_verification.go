package models

import (
	"time"

	"github.com/google/uuid"
)

// BrowserVerificationRun is a minimal owner-scoped audit record for a
// read-only local browser check. It intentionally stores no cookies, DOM,
// screenshot, response body, or query string.
type BrowserVerificationRun struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	OwnerIdentity string     `gorm:"type:text;not null;index:idx_browser_verification_owner_created" json:"ownerIdentity"`
	ProfileID     string     `gorm:"type:text;not null" json:"profileId"`
	Status        string     `gorm:"type:text;not null;index" json:"status"`
	FinalPath     string     `gorm:"type:text" json:"finalPath,omitempty"`
	PageTitle     string     `gorm:"type:text" json:"pageTitle,omitempty"`
	Summary       string     `gorm:"type:text;not null" json:"summary"`
	StartedAt     time.Time  `json:"startedAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	CreatedAt     time.Time  `gorm:"index:idx_browser_verification_owner_created" json:"createdAt"`
	UpdatedAt     time.Time  `gorm:"index:idx_browser_verification_owner_created" json:"updatedAt"`
}

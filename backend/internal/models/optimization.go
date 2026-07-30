package models

import (
	"time"

	"github.com/google/uuid"
)

// OptimizationProposalRun is the durable audit record for a local planning
// proposal. It deliberately stores an opaque request digest and bounded result
// only, never source content, credentials, or task descriptions.
type OptimizationProposalRun struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	OwnerIdentity string    `gorm:"type:text;not null;index:idx_optimizer_owner_created" json:"ownerIdentity"`
	RequestDigest string    `gorm:"type:char(64);not null;index" json:"requestDigest"`
	Status        string    `gorm:"type:text;not null;index" json:"status"`
	Solver        string    `gorm:"type:text" json:"solver,omitempty"`
	Summary       string    `gorm:"type:text;not null" json:"summary"`
	ResultJSON    string    `gorm:"type:jsonb;not null;default:'{}'" json:"result"`
	CreatedAt     time.Time `gorm:"index:idx_optimizer_owner_created" json:"createdAt"`
}

package models

import (
	"time"

	"github.com/google/uuid"
)

// DomainPackPreference stores only owner-scoped overlays. The canonical pack
// catalog remains immutable code-owned policy.
type DomainPackPreference struct {
	ID                  uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	OwnerIdentity       string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_domain_pack_preferences_owner_pack" json:"-"`
	PackID              string    `gorm:"type:varchar(120);not null;uniqueIndex:uq_domain_pack_preferences_owner_pack" json:"packId"`
	CatalogVersion      string    `gorm:"type:varchar(32);not null" json:"catalogVersion"`
	Revision            int64     `gorm:"not null;default:1" json:"revision"`
	Status              string    `gorm:"type:varchar(32);not null;default:'active';index" json:"status"`
	Enabled             *bool     `json:"enabled,omitempty"`
	ClassificationBoost int       `gorm:"type:smallint;not null;default:0" json:"classificationBoost"`
	ForceLocalOnly      bool      `gorm:"not null;default:false" json:"forceLocalOnly"`
	AdaptationsJSON     string    `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	CreatedAt           time.Time `gorm:"not null" json:"createdAt"`
	UpdatedAt           time.Time `gorm:"not null" json:"updatedAt"`
}

func (DomainPackPreference) TableName() string { return "domain_pack_preferences" }

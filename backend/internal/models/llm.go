package models

import (
	"time"

	"github.com/google/uuid"
)

// LLMProviderProbe stores redacted provider-readiness evidence. It contains no
// provider credential or raw response payload, only the operator-safe result
// needed to distinguish configured endpoints from recently verified ones.
type LLMProviderProbe struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	ProviderID       string     `gorm:"type:varchar(120);index;not null" json:"providerId"`
	ProviderName     string     `gorm:"type:varchar(255);not null" json:"providerName"`
	Status           string     `gorm:"type:varchar(80);index;not null" json:"status"`
	Reason           string     `gorm:"type:text" json:"reason,omitempty"`
	EndpointURL      string     `gorm:"type:varchar(1024)" json:"endpointUrl,omitempty"`
	HTTPStatus       int        `json:"httpStatus,omitempty"`
	ModelsSeen       int        `json:"modelsSeen"`
	DurationMs       int64      `json:"durationMs"`
	Live             bool       `gorm:"index" json:"live"`
	RequiresReview   bool       `gorm:"index" json:"requiresReview"`
	CheckedAt        time.Time  `gorm:"index;not null" json:"checkedAt"`
	LastSuccessfulAt *time.Time `gorm:"index" json:"lastSuccessfulAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
}

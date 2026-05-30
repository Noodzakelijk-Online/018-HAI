package models

import (
	"time"

	"github.com/google/uuid"
)

type VerificationRun struct {
	ID                uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	Mode              string    `gorm:"type:varchar(40);index;not null" json:"mode"`
	Question          string    `gorm:"type:text;not null" json:"question"`
	ProjectKey        string    `gorm:"type:varchar(255);index" json:"projectKey,omitempty"`
	Answer            string    `gorm:"type:text" json:"answer"`
	Status            string    `gorm:"type:varchar(50);index;not null" json:"status"`
	ResearchQuestions string    `gorm:"type:text" json:"researchQuestions,omitempty"`
	SourcesSearched   string    `gorm:"type:text" json:"sourcesSearched,omitempty"`
	SourcesUsed       string    `gorm:"type:text" json:"sourcesUsed,omitempty"`
	SourcesRejected   string    `gorm:"type:text" json:"sourcesRejected,omitempty"`
	MissingSources    string    `gorm:"type:text" json:"missingSources,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type VerificationEvidence struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	RunID        uuid.UUID `gorm:"type:uuid;index;not null" json:"runId"`
	SourceType   string    `gorm:"type:varchar(80);index;not null" json:"sourceType"`
	SourceID     string    `gorm:"type:varchar(255);index" json:"sourceId,omitempty"`
	SourceURI    string    `gorm:"type:varchar(1024)" json:"sourceUri,omitempty"`
	SourceLabel  string    `gorm:"type:varchar(512)" json:"sourceLabel,omitempty"`
	Snippet      string    `gorm:"type:text;not null" json:"snippet"`
	Authority    string    `gorm:"type:varchar(80)" json:"authority,omitempty"`
	Freshness    string    `gorm:"type:varchar(80)" json:"freshness,omitempty"`
	QualityScore float64   `gorm:"default:0" json:"qualityScore"`
	Used         bool      `gorm:"default:false;index" json:"used"`
	Rejected     bool      `gorm:"default:false;index" json:"rejected"`
	RejectReason string    `gorm:"type:text" json:"rejectReason,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type VerificationClaim struct {
	ID                 uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	RunID              uuid.UUID `gorm:"type:uuid;index;not null" json:"runId"`
	ClaimText          string    `gorm:"type:text;not null" json:"claimText"`
	Status             string    `gorm:"type:varchar(50);index;not null" json:"status"`
	SourceRefs         string    `gorm:"type:text" json:"sourceRefs,omitempty"`
	SupportExplanation string    `gorm:"type:text" json:"supportExplanation,omitempty"`
	Confidence         float64   `gorm:"default:0" json:"confidence"`
	NeedsReview        bool      `gorm:"default:false;index" json:"needsReview"`
	HighRisk           bool      `gorm:"default:false;index" json:"highRisk"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type VerificationAuditLog struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	RunID     uuid.UUID `gorm:"type:uuid;index" json:"runId,omitempty"`
	Action    string    `gorm:"type:varchar(80);index;not null" json:"action"`
	Message   string    `gorm:"type:text" json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

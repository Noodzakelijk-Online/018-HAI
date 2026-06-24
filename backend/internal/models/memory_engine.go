package models

import (
	"time"

	"github.com/google/uuid"
)

type AIConversationArchive struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	Platform         string     `gorm:"type:varchar(50);index;uniqueIndex:idx_ai_conversation_identity;not null" json:"platform"`
	ExternalID       string     `gorm:"type:varchar(255);index;uniqueIndex:idx_ai_conversation_identity;not null" json:"externalId"`
	Title            string     `gorm:"type:varchar(512);index" json:"title"`
	SourceURI        string     `gorm:"type:varchar(1024);index;not null" json:"sourceUri"`
	ContentHash      string     `gorm:"type:varchar(64);index;not null" json:"contentHash"`
	Revision         int        `gorm:"default:1" json:"revision"`
	MessageCount     int        `json:"messageCount"`
	EncryptedPayload []byte     `gorm:"type:bytea" json:"-"`
	EncryptionNonce  []byte     `gorm:"type:bytea" json:"-"`
	Preview          string     `gorm:"type:text" json:"preview,omitempty"`
	CapturedAt       time.Time  `gorm:"index" json:"capturedAt"`
	LastMessageAt    *time.Time `gorm:"index" json:"lastMessageAt,omitempty"`
	Archived         bool       `gorm:"default:false;index" json:"archived"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type AIMemoryInsight struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id,omitempty"`
	ConversationID uuid.UUID `gorm:"type:uuid;index;not null" json:"conversationId"`
	Revision      int       `gorm:"index;not null" json:"revision"`
	Kind          string    `gorm:"type:varchar(50);index;not null" json:"kind"`
	Text          string    `gorm:"type:text;not null" json:"text"`
	ProjectKey    string    `gorm:"type:varchar(255);index" json:"projectKey,omitempty"`
	Owner         string    `gorm:"type:varchar(120);index" json:"owner,omitempty"`
	RobertNeeded  bool      `gorm:"index" json:"robertNeeded"`
	RiskLevel     string    `gorm:"type:varchar(50);index" json:"riskLevel"`
	Confidence    float64   `json:"confidence"`
	SourceURI     string    `gorm:"type:varchar(1024);index" json:"sourceUri"`
	SourceLabel   string    `gorm:"type:varchar(512)" json:"sourceLabel"`
	NeedsReview   bool      `gorm:"index" json:"needsReview"`
	Status        string    `gorm:"type:varchar(50);index" json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

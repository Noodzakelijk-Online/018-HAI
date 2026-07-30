package models

import (
	"time"

	"github.com/google/uuid"
)

// StandingMandate stores one owner-scoped, versioned grant of bounded
// authority. The JSON columns preserve the package's typed policy contract;
// database constraints enforce their top-level shapes.
type StandingMandate struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4();uniqueIndex:uq_standing_mandates_owner_id,priority:2" json:"id"`
	OwnerIdentity        string     `gorm:"type:varchar(255);not null;uniqueIndex:uq_standing_mandates_owner_id,priority:1;index:idx_standing_mandates_owner_status,priority:1" json:"-"`
	Name                 string     `gorm:"type:varchar(255);not null" json:"name"`
	Purpose              string     `gorm:"type:text;not null" json:"purpose"`
	Status               string     `gorm:"type:varchar(32);not null;index:idx_standing_mandates_owner_status,priority:2" json:"status"`
	Version              string     `gorm:"type:varchar(64);not null" json:"version"`
	Revision             uint64     `gorm:"type:bigint;not null" json:"revision"`
	ScopesJSON           string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	AutonomyCeiling      int        `gorm:"type:smallint;not null" json:"autonomyCeiling"`
	ApprovalPolicyJSON   string     `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	StopConditionsJSON   string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	SourceReferencesJSON string     `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	CreatedBy            string     `gorm:"type:varchar(255);not null" json:"createdBy"`
	CreatedAt            time.Time  `gorm:"not null" json:"createdAt"`
	UpdatedAt            time.Time  `gorm:"not null" json:"updatedAt"`
	ActivatedAt          *time.Time `json:"activatedAt,omitempty"`
	ExpiresAt            *time.Time `gorm:"index:idx_standing_mandates_expiry" json:"expiresAt,omitempty"`
	RevokedAt            *time.Time `json:"revokedAt,omitempty"`
	RevokedBy            string     `gorm:"type:varchar(255);not null;default:''" json:"revokedBy,omitempty"`
	RevocationReason     string     `gorm:"type:text;not null;default:''" json:"revocationReason,omitempty"`
}

func (StandingMandate) TableName() string { return "standing_mandates" }

// StandingMandateDecision is an immutable authorization receipt. It records
// the complete, normalized evidence envelope and digests emitted by the
// evaluator without asserting that approval signatures were verified.
type StandingMandateDecision struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	MandateID         uuid.UUID `gorm:"type:uuid;not null;index:idx_standing_mandate_decisions_mandate_evaluated,priority:1" json:"mandateId"`
	OwnerIdentity     string    `gorm:"type:varchar(255);not null;index:idx_standing_mandate_decisions_owner_evaluated,priority:1" json:"-"`
	ActorIdentity     string    `gorm:"type:varchar(255);not null" json:"actorIdentity"`
	Action            string    `gorm:"type:varchar(256);not null;index" json:"action"`
	Outcome           string    `gorm:"type:varchar(32);not null;index" json:"outcome"`
	Reason            string    `gorm:"type:text;not null" json:"reason"`
	EffectiveAutonomy int       `gorm:"type:smallint;not null" json:"effectiveAutonomy"`
	ApprovalRequired  bool      `gorm:"not null" json:"approvalRequired"`
	ApprovalSatisfied bool      `gorm:"not null" json:"approvalSatisfied"`
	MandateRevision   uint64    `gorm:"type:bigint;not null" json:"mandateRevision"`
	RequestDigest     string    `gorm:"type:char(64);not null;index" json:"requestDigest"`
	MandateDigest     string    `gorm:"type:char(64);not null;index" json:"mandateDigest"`
	DecisionDigest    string    `gorm:"type:char(64);not null;uniqueIndex" json:"decisionDigest"`
	EvidenceJSON      string    `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	EvaluatedAt       time.Time `gorm:"not null;index:idx_standing_mandate_decisions_owner_evaluated,priority:2,sort:desc;index:idx_standing_mandate_decisions_mandate_evaluated,priority:2,sort:desc" json:"evaluatedAt"`
}

func (StandingMandateDecision) TableName() string {
	return "standing_mandate_authorization_decisions"
}

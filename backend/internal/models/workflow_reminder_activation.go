package models

import (
	"time"

	"github.com/google/uuid"
)

// WorkflowReminderActivationRequest is an immutable, owner-scoped request to
// prepare an internal HAI reminder. It is deliberately not an execution job
// and cannot represent an email, message, or external calendar effect.
type WorkflowReminderActivationRequest struct {
	ID              uuid.UUID  `gorm:"type:uuid;primary_key" json:"id"`
	OwnerIdentity   string     `gorm:"type:varchar(255);index;not null" json:"-"`
	WorkflowID      uuid.UUID  `gorm:"type:uuid;index;not null" json:"workflowId"`
	ChecklistItemID uuid.UUID  `gorm:"type:uuid;index;not null" json:"checklistItemId"`
	ActivationKind  string     `gorm:"type:varchar(50);index;not null" json:"activationKind"`
	WorkflowState   string     `gorm:"type:varchar(80);not null" json:"workflowState"`
	ChecklistStatus string     `gorm:"type:varchar(50);not null" json:"checklistStatus"`
	ReminderAt      time.Time  `gorm:"index;not null" json:"reminderAt"`
	DueAt           *time.Time `gorm:"index" json:"dueAt,omitempty"`
	ReminderDigest  string     `gorm:"type:char(64);index;not null" json:"reminderDigest"`
	IdempotencyKey  string     `gorm:"type:varchar(160);not null" json:"idempotencyKey"`
	Authority       string     `gorm:"type:varchar(80);not null" json:"authority"`
	Actor           string     `gorm:"type:varchar(255);not null" json:"-"`
	Confirmation    string     `gorm:"type:varchar(120);not null" json:"confirmation"`
	RequestDigest   string     `gorm:"type:char(64);not null" json:"requestDigest"`
	RecordDigest    string     `gorm:"type:char(64);not null" json:"recordDigest"`
	RequestedAt     time.Time  `gorm:"index;not null" json:"requestedAt"`
	ExpiresAt       time.Time  `gorm:"index;not null" json:"expiresAt"`
}

func (WorkflowReminderActivationRequest) TableName() string {
	return "workflow_reminder_activation_requests"
}

// WorkflowReminderActivationDecision is an immutable link in the decision
// history for a reminder activation request. Approval remains non-executing;
// any future effect must have a separate authorization and effect ledger.
type WorkflowReminderActivationDecision struct {
	ID                      uuid.UUID  `gorm:"type:uuid;primary_key" json:"id"`
	ActivationRequestID     uuid.UUID  `gorm:"type:uuid;index;not null" json:"activationRequestId"`
	OwnerIdentity           string     `gorm:"type:varchar(255);index;not null" json:"-"`
	Decision                string     `gorm:"type:varchar(40);index;not null" json:"decision"`
	Reason                  string     `gorm:"type:text;not null" json:"reason"`
	Actor                   string     `gorm:"type:varchar(255);not null" json:"-"`
	Confirmation            string     `gorm:"type:varchar(120);not null" json:"confirmation"`
	ActivationRequestDigest string     `gorm:"type:char(64);not null" json:"activationRequestDigest"`
	PreviousDecisionID      *uuid.UUID `gorm:"type:uuid;index" json:"previousDecisionId,omitempty"`
	Authority               string     `gorm:"type:varchar(80);not null" json:"authority"`
	RequestDigest           string     `gorm:"type:char(64);not null" json:"requestDigest"`
	RecordDigest            string     `gorm:"type:char(64);not null" json:"recordDigest"`
	DecidedAt               time.Time  `gorm:"index;not null" json:"decidedAt"`
	ExpiresAt               *time.Time `gorm:"index" json:"expiresAt,omitempty"`
}

func (WorkflowReminderActivationDecision) TableName() string {
	return "workflow_reminder_activation_decisions"
}

// WorkflowReminderDeliveryAuthorization is a one-time, owner-approved grant
// to place an internal reminder signal into HAI. It cannot authorize email,
// messaging, calendar writes, desktop pushes, or any other external effect.
type WorkflowReminderDeliveryAuthorization struct {
	ID                       uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	ActivationRequestID      uuid.UUID `gorm:"type:uuid;index;not null" json:"activationRequestId"`
	ActivationDecisionID     uuid.UUID `gorm:"type:uuid;index;not null" json:"activationDecisionId"`
	OwnerIdentity            string    `gorm:"type:varchar(255);index;not null" json:"-"`
	WorkflowID               uuid.UUID `gorm:"type:uuid;index;not null" json:"workflowId"`
	ChecklistItemID          uuid.UUID `gorm:"type:uuid;index;not null" json:"checklistItemId"`
	ReminderAt               time.Time `gorm:"index;not null" json:"reminderAt"`
	ReminderDigest           string    `gorm:"type:char(64);index;not null" json:"reminderDigest"`
	ActivationRequestDigest  string    `gorm:"type:char(64);not null" json:"activationRequestDigest"`
	ActivationDecisionDigest string    `gorm:"type:char(64);not null" json:"activationDecisionDigest"`
	Channel                  string    `gorm:"type:varchar(40);not null" json:"channel"`
	IdempotencyKey           string    `gorm:"type:varchar(160);not null" json:"idempotencyKey"`
	Authority                string    `gorm:"type:varchar(80);not null" json:"authority"`
	Actor                    string    `gorm:"type:varchar(255);not null" json:"-"`
	Confirmation             string    `gorm:"type:varchar(120);not null" json:"confirmation"`
	RequestDigest            string    `gorm:"type:char(64);not null" json:"requestDigest"`
	RecordDigest             string    `gorm:"type:char(64);not null" json:"recordDigest"`
	AuthorizedAt             time.Time `gorm:"index;not null" json:"authorizedAt"`
	ExpiresAt                time.Time `gorm:"index;not null" json:"expiresAt"`
}

func (WorkflowReminderDeliveryAuthorization) TableName() string {
	return "workflow_reminder_delivery_authorizations"
}

// WorkflowReminderDeliveryAttempt is append-only. A delivered record is both
// the internal reminder receipt and the idempotent audit proof for the worker.
type WorkflowReminderDeliveryAttempt struct {
	ID                  uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	AuthorizationID     uuid.UUID `gorm:"type:uuid;index;not null" json:"authorizationId"`
	OwnerIdentity       string    `gorm:"type:varchar(255);index;not null" json:"-"`
	AttemptNumber       int       `gorm:"not null" json:"attemptNumber"`
	Status              string    `gorm:"type:varchar(40);index;not null" json:"status"`
	Reason              string    `gorm:"type:text;not null" json:"reason"`
	ReminderDigest      string    `gorm:"type:char(64);not null" json:"reminderDigest"`
	AuthorizationDigest string    `gorm:"type:char(64);not null" json:"authorizationDigest"`
	Authority           string    `gorm:"type:varchar(80);not null" json:"authority"`
	RecordDigest        string    `gorm:"type:char(64);not null" json:"recordDigest"`
	AttemptedAt         time.Time `gorm:"index;not null" json:"attemptedAt"`
}

func (WorkflowReminderDeliveryAttempt) TableName() string {
	return "workflow_reminder_delivery_attempts"
}

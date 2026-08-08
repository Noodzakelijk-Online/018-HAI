// Package lifeledger stores owner-scoped commitments and financial events as
// immutable, source-backed records. It is an evidence ledger, not an approval,
// payment, messaging, or execution interface.
package lifeledger

import (
	"context"
	"time"

	"automation-hub-backend/internal/lifeontology"

	"github.com/google/uuid"
)

const ContractVersion = "life-ledger.v1"

type CommitmentStatus string

const (
	CommitmentProposed  CommitmentStatus = "proposed"
	CommitmentActive    CommitmentStatus = "active"
	CommitmentWaiting   CommitmentStatus = "waiting"
	CommitmentFulfilled CommitmentStatus = "fulfilled"
	CommitmentCancelled CommitmentStatus = "cancelled"
	CommitmentBreached  CommitmentStatus = "breached"
	CommitmentDisputed  CommitmentStatus = "disputed"
)

type CostKind string

const (
	CostEstimate CostKind = "estimate"
	CostIncurred CostKind = "incurred"
	CostPaid     CostKind = "paid"
	CostRefund   CostKind = "refund"
)

type VerificationStatus string

const (
	VerificationSourceSupported VerificationStatus = "source_supported"
	VerificationHumanConfirmed  VerificationStatus = "human_confirmed"
	VerificationVerified        VerificationStatus = "verified"
	VerificationNeedsReview     VerificationStatus = "needs_review"
	VerificationDisputed        VerificationStatus = "disputed"
)

type EvidenceReference struct {
	ID            string             `json:"id"`
	URI           string             `json:"uri"`
	ContentDigest string             `json:"contentDigest"`
	Authority     string             `json:"authority,omitempty"`
	ObservedAt    time.Time          `json:"observedAt"`
	Verification  VerificationStatus `json:"verification"`
	LocalOnly     bool               `json:"localOnly"`
}

// CommitmentRevision is one immutable snapshot in a commitment's history.
// Later states append a new revision and never overwrite this record.
type CommitmentRevision struct {
	ContractVersion  string                                    `json:"contractVersion"`
	ID               uuid.UUID                                 `json:"id"`
	OwnerIdentity    string                                    `json:"ownerIdentity"`
	CommitmentKey    string                                    `json:"commitmentKey"`
	Revision         uint64                                    `json:"revision"`
	Domain           lifeontology.Domain                       `json:"domain"`
	Title            string                                    `json:"title"`
	Summary          string                                    `json:"summary,omitempty"`
	Status           CommitmentStatus                          `json:"status"`
	Counterparty     string                                    `json:"counterparty,omitempty"`
	ProjectKey       string                                    `json:"projectKey,omitempty"`
	DueAt            *time.Time                                `json:"dueAt,omitempty"`
	Verification     VerificationStatus                        `json:"verification"`
	Evidence         []EvidenceReference                       `json:"evidence"`
	LocalOnly        bool                                      `json:"localOnly"`
	IdempotencyKey   string                                    `json:"idempotencyKey"`
	RequestDigest    string                                    `json:"requestDigest"`
	RecordDigest     string                                    `json:"recordDigest"`
	ObservedAt       time.Time                                 `json:"observedAt"`
	RecordedAt       time.Time                                 `json:"recordedAt"`
	LifeGraphWarning string                                    `json:"lifeGraphWarning,omitempty"`
	LifeGraph        *lifeontology.OperationalProjectionResult `json:"lifeGraph,omitempty"`
}

// CostEntry is an immutable money event. CostKind distinguishes an estimate
// from an incurred, paid, or refunded amount; the ledger never performs money
// movement and never interprets an estimate as incurred.
type CostEntry struct {
	ContractVersion  string                                    `json:"contractVersion"`
	ID               uuid.UUID                                 `json:"id"`
	OwnerIdentity    string                                    `json:"ownerIdentity"`
	Domain           lifeontology.Domain                       `json:"domain"`
	Title            string                                    `json:"title"`
	Summary          string                                    `json:"summary,omitempty"`
	Kind             CostKind                                  `json:"kind"`
	AmountMinor      int64                                     `json:"amountMinor"`
	Currency         string                                    `json:"currency"`
	CommitmentKey    string                                    `json:"commitmentKey,omitempty"`
	ProjectKey       string                                    `json:"projectKey,omitempty"`
	Verification     VerificationStatus                        `json:"verification"`
	Evidence         []EvidenceReference                       `json:"evidence"`
	LocalOnly        bool                                      `json:"localOnly"`
	IdempotencyKey   string                                    `json:"idempotencyKey"`
	RequestDigest    string                                    `json:"requestDigest"`
	RecordDigest     string                                    `json:"recordDigest"`
	ObservedAt       time.Time                                 `json:"observedAt"`
	RecordedAt       time.Time                                 `json:"recordedAt"`
	LifeGraphWarning string                                    `json:"lifeGraphWarning,omitempty"`
	LifeGraph        *lifeontology.OperationalProjectionResult `json:"lifeGraph,omitempty"`
}

type RecordCommitmentRequest struct {
	OwnerIdentity    string
	CommitmentKey    string
	ExpectedRevision uint64
	Domain           lifeontology.Domain
	Title            string
	Summary          string
	Status           CommitmentStatus
	Counterparty     string
	ProjectKey       string
	DueAt            *time.Time
	Verification     VerificationStatus
	Evidence         []EvidenceReference
	IdempotencyKey   string
	ObservedAt       time.Time
}

type RecordCostRequest struct {
	OwnerIdentity  string
	Domain         lifeontology.Domain
	Title          string
	Summary        string
	Kind           CostKind
	AmountMinor    int64
	Currency       string
	CommitmentKey  string
	ProjectKey     string
	Verification   VerificationStatus
	Evidence       []EvidenceReference
	IdempotencyKey string
	ObservedAt     time.Time
}

type CommitmentWriteResult struct {
	Record  CommitmentRevision `json:"record"`
	Created bool               `json:"created"`
}

type CostWriteResult struct {
	Record  CostEntry `json:"record"`
	Created bool      `json:"created"`
}

type Repository interface {
	SaveCommitment(context.Context, CommitmentRevision, uint64) (CommitmentRevision, bool, error)
	GetCommitment(context.Context, string, string) (CommitmentRevision, error)
	ListCommitments(context.Context, string, int) ([]CommitmentRevision, error)
	ListCommitmentHistory(context.Context, string, string, int) ([]CommitmentRevision, error)
	AppendCost(context.Context, CostEntry) (CostEntry, bool, error)
	ListCosts(context.Context, string, int) ([]CostEntry, error)
}

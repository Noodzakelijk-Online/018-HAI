package dataexport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"automation-hub-backend/internal/executionauth"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/safety"

	"github.com/google/uuid"
)

const (
	memoryExportAction       = "dataexport.memory.export"
	memoryExportResourceType = "memory-export"
	memoryExportConsumer     = "dataexport"
	authorizationTTL         = 5 * time.Minute
)

var (
	ErrInvalidRequest           = errors.New("memory export request is invalid")
	ErrAuthorizationUnavailable = errors.New("memory export authorization is unavailable")
	ErrAuthorizationDenied      = errors.New("memory export authorization was denied")
	ErrAuthorizationMismatch    = errors.New("memory export authorization does not match the requested data")
	ErrEmergencyStopActive      = errors.New("memory export blocked by emergency stop")
)

// AuthorizationRequest contains authenticated actor and approval references.
// Owner, action, risk, resource, and effect identity are derived by Service.
type AuthorizationRequest struct {
	OwnerIdentity         string
	ActorIdentity         string
	IdempotencyKey        string
	TaskID                string
	ApprovalSourceID      string
	ApprovalBindingDigest string
	ProjectKey            string
}

type ExecutionAuthorizer interface {
	AuthorizeAndConsume(
		context.Context,
		executionauth.Request,
		string,
		string,
	) (executionauth.Receipt, error)
}

// Evidence is a non-sensitive reference to the durable authorization record.
type Evidence struct {
	ReceiptID       string    `json:"receiptId"`
	DecisionDigest  string    `json:"decisionDigest"`
	EffectDigest    string    `json:"effectDigest"`
	ApprovalSource  string    `json:"approvalSourceId"`
	ApprovedBy      string    `json:"approvedBy"`
	AuthorizedAt    time.Time `json:"authorizedAt"`
	ExportedAt      time.Time `json:"exportedAt"`
	RecordCount     int       `json:"recordCount"`
	OwnerBound      bool      `json:"ownerBound"`
	EmergencyStopOK bool      `json:"emergencyStopClear"`
}

type Result struct {
	Data     Export   `json:"data"`
	Evidence Evidence `json:"evidence"`
}

type Service struct {
	authorizer ExecutionAuthorizer
	now        func() time.Time
}

func NewService(authorizer ExecutionAuthorizer, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{authorizer: authorizer, now: now}
}

type memoryRecordDigest struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
}

type memoryExportEffect struct {
	Version       int                  `json:"version"`
	OwnerIdentity string               `json:"ownerIdentity"`
	Action        string               `json:"action"`
	Format        string               `json:"format"`
	ExportVersion int                  `json:"exportVersion"`
	Records       []memoryRecordDigest `json:"records"`
}

// MemoryExportEffectDigest derives the approval binding for one exact,
// owner-bound snapshot. Sorting affects identity only, not exported order.
func MemoryExportEffectDigest(
	ownerIdentity string,
	memories []models.ContextMemory,
) (string, error) {
	owner := strings.TrimSpace(ownerIdentity)
	if owner == "" || len(owner) > 256 {
		return "", fmt.Errorf("%w: verified owner identity is required", ErrInvalidRequest)
	}
	recordDigests := make([]memoryRecordDigest, 0, len(memories))
	for _, memory := range memories {
		if memory.ID == uuid.Nil {
			return "", fmt.Errorf("%w: every memory needs an id", ErrInvalidRequest)
		}
		if strings.TrimSpace(memory.OwnerIdentity) != owner {
			return "", fmt.Errorf("%w: memory ownership does not match", ErrInvalidRequest)
		}
		encoded, err := json.Marshal(memory)
		if err != nil {
			return "", fmt.Errorf("%w: memory cannot be encoded", ErrInvalidRequest)
		}
		sum := sha256.Sum256(encoded)
		recordDigests = append(recordDigests, memoryRecordDigest{
			ID:     memory.ID.String(),
			Digest: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(recordDigests, func(i, j int) bool {
		if recordDigests[i].ID == recordDigests[j].ID {
			return recordDigests[i].Digest < recordDigests[j].Digest
		}
		return recordDigests[i].ID < recordDigests[j].ID
	})
	encoded, err := json.Marshal(memoryExportEffect{
		Version:       1,
		OwnerIdentity: owner,
		Action:        memoryExportAction,
		Format:        exportFormat,
		ExportVersion: exportVersion,
		Records:       recordDigests,
	})
	if err != nil {
		return "", fmt.Errorf("%w: export identity cannot be encoded", ErrInvalidRequest)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// BuildMemoryExport consumes exact authority after validation and immediately
// before generating the sensitive snapshot.
func (s *Service) BuildMemoryExport(
	ctx context.Context,
	auth AuthorizationRequest,
	memories []models.ContextMemory,
) (Result, error) {
	if s == nil || s.authorizer == nil {
		return Result{}, ErrAuthorizationUnavailable
	}
	auth = normalizeAuthorization(auth)
	if auth.OwnerIdentity == "" || auth.ActorIdentity == "" ||
		auth.IdempotencyKey == "" || auth.TaskID == "" ||
		auth.ApprovalSourceID == "" || auth.ApprovalBindingDigest == "" {
		return Result{}, fmt.Errorf(
			"%w: actor, task, idempotency, and fresh approval references are required",
			ErrInvalidRequest,
		)
	}
	effectDigest, err := MemoryExportEffectDigest(auth.OwnerIdentity, memories)
	if err != nil {
		return Result{}, err
	}
	if auth.ApprovalBindingDigest != effectDigest {
		return Result{}, ErrAuthorizationMismatch
	}

	resourceID := "memory-export:" + effectDigest[:16]
	request := executionauth.Request{
		OwnerIdentity:         auth.OwnerIdentity,
		IdempotencyKey:        auth.IdempotencyKey,
		ActorIdentity:         auth.ActorIdentity,
		ActorKind:             executionauth.ActorHuman,
		TaskID:                auth.TaskID,
		Action:                memoryExportAction,
		Stage:                 executionauth.StageDataAccess,
		ResourceType:          memoryExportResourceType,
		ResourceID:            resourceID,
		ProjectKey:            auth.ProjectKey,
		Domain:                "private-data-export",
		DataScopes:            []string{"context-memory"},
		RequiredAuthority:     8,
		RequestedAutonomy:     6,
		Risk:                  executionauth.RiskHigh,
		Reversible:            false,
		ApprovalSourceID:      auth.ApprovalSourceID,
		ApprovalBindingDigest: auth.ApprovalBindingDigest,
		EffectDigest:          effectDigest,
		Facts: map[string]string{
			"format":       exportFormat,
			"record_count": fmt.Sprintf("%d", len(memories)),
		},
		SourceReferences: []string{"dataexport://context-memory"},
		RequestedAt:      s.now().UTC(),
	}
	receipt, err := s.authorizer.AuthorizeAndConsume(
		ctx,
		request,
		memoryExportConsumer,
		memoryExportAction+":"+effectDigest,
	)
	if err != nil {
		// Upstream policy errors may contain sensitive configuration details.
		return Result{}, ErrAuthorizationDenied
	}
	now := s.now().UTC()
	if err := validateReceipt(receipt, request, now); err != nil {
		return Result{}, err
	}
	if safety.EvaluateEmergencyStop().Active {
		return Result{}, ErrEmergencyStopActive
	}

	data := buildMemoryExport(memories)
	return Result{
		Data: data,
		Evidence: Evidence{
			ReceiptID:       receipt.ID.String(),
			DecisionDigest:  receipt.DecisionDigest,
			EffectDigest:    effectDigest,
			ApprovalSource:  receipt.Evidence.Approval.SourceID,
			ApprovedBy:      receipt.Evidence.Approval.ApprovedBy,
			AuthorizedAt:    receipt.EvaluatedAt,
			ExportedAt:      now,
			RecordCount:     data.Count,
			OwnerBound:      true,
			EmergencyStopOK: true,
		},
	}, nil
}

func normalizeAuthorization(value AuthorizationRequest) AuthorizationRequest {
	value.OwnerIdentity = strings.TrimSpace(value.OwnerIdentity)
	value.ActorIdentity = strings.TrimSpace(value.ActorIdentity)
	value.IdempotencyKey = strings.TrimSpace(value.IdempotencyKey)
	value.TaskID = strings.TrimSpace(value.TaskID)
	value.ApprovalSourceID = strings.TrimSpace(value.ApprovalSourceID)
	value.ApprovalBindingDigest = strings.ToLower(strings.TrimSpace(value.ApprovalBindingDigest))
	value.ProjectKey = strings.TrimSpace(value.ProjectKey)
	return value
}

func validateReceipt(
	receipt executionauth.Receipt,
	request executionauth.Request,
	now time.Time,
) error {
	approval := receipt.Evidence.Approval
	if receipt.Outcome != executionauth.OutcomeAuthorized ||
		receipt.OwnerIdentity != request.OwnerIdentity ||
		receipt.ActorIdentity != request.ActorIdentity ||
		receipt.ActorKind != executionauth.ActorHuman ||
		receipt.TaskID != request.TaskID ||
		receipt.Action != request.Action ||
		receipt.Stage != request.Stage ||
		receipt.ResourceType != request.ResourceType ||
		receipt.ResourceID != request.ResourceID ||
		receipt.ApprovalSourceID != request.ApprovalSourceID ||
		receipt.EffectDigest != request.EffectDigest ||
		approval.SourceID != request.ApprovalSourceID ||
		approval.DecisionID == "" ||
		approval.DecisionDigest == "" ||
		approval.ApprovedBy == "" {
		return ErrAuthorizationMismatch
	}
	if approval.ApprovedAt.IsZero() || approval.ExpiresAt.IsZero() ||
		approval.ApprovedAt.After(now.Add(time.Minute)) ||
		now.After(approval.ExpiresAt) ||
		now.Sub(approval.ApprovedAt) > authorizationTTL ||
		receipt.EvaluatedAt.IsZero() ||
		receipt.EvaluatedAt.After(now.Add(time.Minute)) ||
		now.Sub(receipt.EvaluatedAt) > authorizationTTL {
		return ErrAuthorizationDenied
	}
	return nil
}

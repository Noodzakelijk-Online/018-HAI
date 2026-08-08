package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	openClawSetPathAction  = "agent-runtime.openclaw-ecosystem.set-path"
	openClawRefreshAction  = "agent-runtime.openclaw-ecosystem.refresh"
	openClawUploadAction   = "agent-runtime.openclaw-ecosystem.upload"
	openClawResourceType   = "agent-runtime-ecosystem"
	openClawResourceID     = "openclaw"
	openClawMutationTarget = "agentruntime:openclaw-ecosystem:"
	openClawMutationTTL    = 5 * time.Minute
)

var (
	ErrEcosystemAuthorizationUnavailable = errors.New("OpenClaw ecosystem authorization is unavailable")
	ErrEcosystemAuthorizationDenied      = errors.New("OpenClaw ecosystem authorization was denied")
	ErrEcosystemAuthorizationMismatch    = errors.New("OpenClaw ecosystem authorization did not match the requested mutation")
	ErrEcosystemMutationConflict         = errors.New("OpenClaw ecosystem changed before the authorized mutation was applied")
)

// EcosystemMutationAuthorization contains references to server-side approval
// evidence. It never contains a caller assertion that approval occurred.
type EcosystemMutationAuthorization struct {
	IdempotencyKey        string `json:"idempotencyKey" form:"idempotencyKey"`
	TaskID                string `json:"taskId" form:"taskId"`
	ApprovalSourceID      string `json:"approvalSourceId" form:"approvalSourceId"`
	ApprovalBindingDigest string `json:"approvalBindingDigest" form:"approvalBindingDigest"`
}

// These cycle-free types are adapted to executionauth.Request and Receipt by
// production composition. executionauth already imports agentruntime for task
// final-effect verification, so agentruntime cannot import it back.
type EcosystemMutationAuthorizationRequest struct {
	OwnerIdentity         string
	IdempotencyKey        string
	ActorIdentity         string
	ActorKind             string
	TaskID                string
	Action                string
	Stage                 string
	ResourceType          string
	ResourceID            string
	RuntimeID             string
	RequiredAuthority     int
	RequestedAutonomy     int
	Risk                  string
	Reversible            bool
	ApprovalSourceID      string
	ApprovalBindingDigest string
	EffectDigest          string
	SourceReferences      []string
	RequestedAt           time.Time
}

type EcosystemMutationAuthorizationReceipt struct {
	ReceiptID             string
	DecisionDigest        string
	Outcome               string
	OwnerIdentity         string
	ActorIdentity         string
	TaskID                string
	Action                string
	Stage                 string
	ResourceType          string
	ResourceID            string
	RuntimeID             string
	ApprovalSourceID      string
	ApprovalBindingDigest string
	ApprovalDecisionID    string
	ApprovedBy            string
	ApprovedAt            time.Time
	ApprovalExpiresAt     time.Time
	EffectDigest          string
	EvaluatedAt           time.Time
}

type EcosystemMutationAuthorizer interface {
	AuthorizeAndConsumeEcosystemMutation(
		context.Context,
		EcosystemMutationAuthorizationRequest,
		string,
		string,
	) (EcosystemMutationAuthorizationReceipt, error)
}

type EcosystemMutationAuthorizerFunc func(
	context.Context,
	EcosystemMutationAuthorizationRequest,
	string,
	string,
) (EcosystemMutationAuthorizationReceipt, error)

func (f EcosystemMutationAuthorizerFunc) AuthorizeAndConsumeEcosystemMutation(
	ctx context.Context,
	request EcosystemMutationAuthorizationRequest,
	consumer string,
	executionTarget string,
) (EcosystemMutationAuthorizationReceipt, error) {
	return f(ctx, request, consumer, executionTarget)
}

type openClawEcosystemEffect struct {
	ContractVersion       int    `json:"contractVersion"`
	OwnerIdentity         string `json:"ownerIdentity"`
	ActorIdentity         string `json:"actorIdentity"`
	Action                string `json:"action"`
	ResourceType          string `json:"resourceType"`
	ResourceID            string `json:"resourceId"`
	RuntimeID             string `json:"runtimeId"`
	CurrentPath           string `json:"currentPath,omitempty"`
	CurrentSignature      string `json:"currentSignature,omitempty"`
	TargetPath            string `json:"targetPath,omitempty"`
	TargetSignature       string `json:"targetSignature,omitempty"`
	UploadedContentDigest string `json:"uploadedContentDigest,omitempty"`
	UploadedSize          int64  `json:"uploadedSize,omitempty"`
	DeleteManagedPath     string `json:"deleteManagedPath,omitempty"`
	ApprovalSourceID      string `json:"approvalSourceId"`
	ApprovalBindingDigest string `json:"approvalBindingDigest"`
}

func normalizeEcosystemAuthorization(
	value EcosystemMutationAuthorization,
) (EcosystemMutationAuthorization, error) {
	value.IdempotencyKey = strings.TrimSpace(value.IdempotencyKey)
	value.TaskID = strings.TrimSpace(value.TaskID)
	value.ApprovalSourceID = strings.TrimSpace(value.ApprovalSourceID)
	value.ApprovalBindingDigest = strings.ToLower(strings.TrimSpace(value.ApprovalBindingDigest))
	if value.IdempotencyKey == "" || value.TaskID == "" ||
		value.ApprovalSourceID == "" || !isLowerSHA256(value.ApprovalBindingDigest) {
		return EcosystemMutationAuthorization{}, fmt.Errorf(
			"%w: idempotency key, task, approval source, and binding digest are required",
			ErrEcosystemAuthorizationDenied,
		)
	}
	if len(value.IdempotencyKey) > 256 || len(value.TaskID) > 256 ||
		len(value.ApprovalSourceID) > 256 {
		return EcosystemMutationAuthorization{}, fmt.Errorf(
			"%w: authorization provenance exceeds its bound",
			ErrEcosystemAuthorizationDenied,
		)
	}
	return value, nil
}

func buildEcosystemMutationAuthorizationRequest(
	owner string,
	actor string,
	authorization EcosystemMutationAuthorization,
	effect openClawEcosystemEffect,
) (EcosystemMutationAuthorizationRequest, string, error) {
	owner = strings.TrimSpace(owner)
	actor = strings.TrimSpace(actor)
	if owner == "" || actor == "" {
		return EcosystemMutationAuthorizationRequest{}, "", ErrEcosystemAuthorizationDenied
	}
	switch effect.Action {
	case openClawSetPathAction:
		if strings.TrimSpace(effect.TargetPath) == "" ||
			strings.TrimSpace(effect.TargetSignature) == "" {
			return EcosystemMutationAuthorizationRequest{}, "",
				ErrEcosystemAuthorizationDenied
		}
	case openClawRefreshAction:
		if effect.TargetPath != effect.CurrentPath ||
			effect.TargetSignature != effect.CurrentSignature {
			return EcosystemMutationAuthorizationRequest{}, "",
				ErrEcosystemAuthorizationDenied
		}
	case openClawUploadAction:
		if !isOpenClawUploadArtifactPath(effect.TargetPath) ||
			!isLowerSHA256(effect.UploadedContentDigest) ||
			effect.UploadedSize <= 0 {
			return EcosystemMutationAuthorizationRequest{}, "",
				ErrEcosystemAuthorizationDenied
		}
	default:
		return EcosystemMutationAuthorizationRequest{}, "",
			ErrEcosystemAuthorizationDenied
	}
	authorization, err := normalizeEcosystemAuthorization(authorization)
	if err != nil {
		return EcosystemMutationAuthorizationRequest{}, "", err
	}
	effect.ContractVersion = 1
	effect.OwnerIdentity = owner
	effect.ActorIdentity = actor
	effect.ResourceType = openClawResourceType
	effect.ResourceID = openClawResourceID
	effect.RuntimeID = openClawResourceID
	effect.ApprovalSourceID = authorization.ApprovalSourceID
	effect.ApprovalBindingDigest = authorization.ApprovalBindingDigest
	encoded, err := json.Marshal(effect)
	if err != nil {
		return EcosystemMutationAuthorizationRequest{}, "", err
	}
	sum := sha256.Sum256(encoded)
	digest := hex.EncodeToString(sum[:])
	request := EcosystemMutationAuthorizationRequest{
		OwnerIdentity:     owner,
		IdempotencyKey:    authorization.IdempotencyKey,
		ActorIdentity:     actor,
		ActorKind:         "human",
		TaskID:            authorization.TaskID,
		Action:            effect.Action,
		Stage:             "self_modification",
		ResourceType:      openClawResourceType,
		ResourceID:        openClawResourceID,
		RuntimeID:         openClawResourceID,
		RequiredAuthority: 8,
		// Ecosystem mutation is always an explicitly approved, one-shot
		// execution. Level 6 is the case-approval execution level in the
		// unified authorization contract; levels 0-5 are deliberately
		// non-executing and would make every valid mutation fail closed.
		RequestedAutonomy:     6,
		Risk:                  "high",
		Reversible:            effect.Action == openClawRefreshAction,
		ApprovalSourceID:      authorization.ApprovalSourceID,
		ApprovalBindingDigest: authorization.ApprovalBindingDigest,
		EffectDigest:          digest,
		SourceReferences:      []string{"agent-runtime://openclaw/ecosystem"},
		RequestedAt:           time.Now().UTC(),
	}
	return request, openClawMutationTarget + digest, nil
}

func validateEcosystemMutationReceipt(
	receipt EcosystemMutationAuthorizationReceipt,
	request EcosystemMutationAuthorizationRequest,
	now time.Time,
) error {
	receiptID, receiptIDErr := uuid.Parse(strings.TrimSpace(receipt.ReceiptID))
	if receiptIDErr != nil || receiptID == uuid.Nil ||
		!isLowerSHA256(receipt.DecisionDigest) ||
		receipt.Outcome != "authorized" ||
		receipt.OwnerIdentity != request.OwnerIdentity ||
		receipt.ActorIdentity != request.ActorIdentity ||
		receipt.TaskID != request.TaskID ||
		receipt.Action != request.Action ||
		receipt.Stage != request.Stage ||
		receipt.ResourceType != request.ResourceType ||
		receipt.ResourceID != request.ResourceID ||
		receipt.RuntimeID != request.RuntimeID ||
		receipt.ApprovalSourceID != request.ApprovalSourceID ||
		receipt.ApprovalBindingDigest != request.ApprovalBindingDigest ||
		strings.TrimSpace(receipt.ApprovalDecisionID) == "" ||
		strings.TrimSpace(receipt.ApprovedBy) == "" ||
		receipt.EffectDigest != request.EffectDigest {
		return ErrEcosystemAuthorizationMismatch
	}
	now = now.UTC()
	if receipt.ApprovedAt.IsZero() ||
		receipt.ApprovalExpiresAt.IsZero() ||
		receipt.ApprovedAt.After(now.Add(time.Minute)) ||
		now.After(receipt.ApprovalExpiresAt) ||
		now.Sub(receipt.ApprovedAt) > openClawMutationTTL ||
		receipt.EvaluatedAt.IsZero() ||
		receipt.EvaluatedAt.After(now.Add(time.Minute)) ||
		now.Sub(receipt.EvaluatedAt) > openClawMutationTTL {
		return fmt.Errorf("%w: authorization receipt is not fresh", ErrEcosystemAuthorizationDenied)
	}
	return nil
}

func isLowerSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

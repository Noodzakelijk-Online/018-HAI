package executionapproval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"automation-hub-backend/internal/executionauth"

	"github.com/google/uuid"
)

const (
	workflowDecisionPrefix = "workflow-decision:"
	workflowDecisionType   = "approval"
	workflowDecisionValue  = "approved"
	workflowBindingPrefix  = "automation-action:"

	maximumWorkflowReasonRunes = 2048
	maximumWorkflowActorBytes  = 255
)

var (
	ErrInvalidWorkflowReference    = errors.New("invalid workflow approval reference")
	ErrWorkflowApprovalUnavailable = errors.New("durable workflow approval is unavailable")
	ErrWorkflowBindingMismatch     = errors.New("workflow approval binding digest does not match")
	ErrInvalidWorkflowDecision     = errors.New("durable workflow approval decision is invalid")
	ErrStaleWorkflowApproval       = errors.New("durable workflow approval is stale")
	ErrFutureWorkflowApproval      = errors.New("durable workflow approval timestamp is in the future")
)

// WorkflowApprovalDecisionRecord is the immutable owner-scoped projection an
// adapter must load from durable workflow storage. Implementations must resolve
// DecisionID through a join to its WorkflowID and OwnerIdentity; caller-provided
// workflow or owner fields are never authority.
type WorkflowApprovalDecisionRecord struct {
	DecisionID    string
	WorkflowID    string
	OwnerIdentity string
	DecisionType  string
	Decision      string
	Reason        string
	ActionBinding string
	Approved      bool
	Actor         string
	CreatedAt     time.Time
}

// WorkflowApprovalDecisionRepository is deliberately narrower than the
// workflow repository. The current workflow repository cannot safely resolve a
// decision UUID and owner in one contract, so production wiring must provide an
// adapter that performs that owner-scoped durable lookup.
type WorkflowApprovalDecisionRepository interface {
	FindWorkflowApprovalDecision(
		context.Context,
		string,
		string,
	) (*WorkflowApprovalDecisionRecord, error)
}

// WorkflowApprovalResolver turns one durable, action-bound workflow decision
// into execution authorization evidence. It never accepts a caller-constructed
// approval fact.
type WorkflowApprovalResolver struct {
	repository WorkflowApprovalDecisionRepository
	now        func() time.Time
}

var _ executionauth.ApprovalResolver = (*WorkflowApprovalResolver)(nil)

func NewWorkflowApprovalResolver(
	repository WorkflowApprovalDecisionRepository,
) (*WorkflowApprovalResolver, error) {
	if isNilWorkflowRepository(repository) {
		return nil, fmt.Errorf("%w: workflow approval repository is required", ErrInvalidRequest)
	}
	return &WorkflowApprovalResolver{
		repository: repository,
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

func (r *WorkflowApprovalResolver) Resolve(
	ctx context.Context,
	ownerIdentity string,
	sourceID string,
	bindingDigest string,
) (executionauth.ResolvedApproval, error) {
	if r == nil || isNilWorkflowRepository(r.repository) || r.now == nil {
		return executionauth.ResolvedApproval{}, fmt.Errorf(
			"%w: workflow approval resolver is not configured",
			ErrInvalidRequest,
		)
	}
	if ctx == nil {
		return executionauth.ResolvedApproval{}, fmt.Errorf("%w: context is required", ErrInvalidRequest)
	}
	if err := ctx.Err(); err != nil {
		return executionauth.ResolvedApproval{}, err
	}
	if err := validateOwner(ownerIdentity); err != nil {
		return executionauth.ResolvedApproval{}, err
	}
	decisionID, err := parseWorkflowDecisionSource(sourceID)
	if err != nil {
		return executionauth.ResolvedApproval{}, err
	}
	if !validSHA256Digest(bindingDigest) {
		return executionauth.ResolvedApproval{}, fmt.Errorf(
			"%w: binding digest must be a lowercase SHA-256 digest",
			ErrInvalidRequest,
		)
	}

	decision, err := r.repository.FindWorkflowApprovalDecision(
		ctx,
		ownerIdentity,
		decisionID.String(),
	)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return executionauth.ResolvedApproval{}, ctxErr
		}
		return executionauth.ResolvedApproval{}, fmt.Errorf(
			"%w: read durable workflow decision: %w",
			ErrWorkflowApprovalUnavailable,
			err,
		)
	}
	if err := ctx.Err(); err != nil {
		return executionauth.ResolvedApproval{}, err
	}
	if decision == nil {
		return executionauth.ResolvedApproval{}, ErrWorkflowApprovalUnavailable
	}
	if err := validateWorkflowDecision(*decision, ownerIdentity, decisionID); err != nil {
		return executionauth.ResolvedApproval{}, err
	}
	actionDigest, err := workflowActionDigest(decision.ActionBinding)
	if err != nil {
		return executionauth.ResolvedApproval{}, fmt.Errorf(
			"%w: %v",
			ErrInvalidWorkflowDecision,
			err,
		)
	}
	if actionDigest != bindingDigest {
		return executionauth.ResolvedApproval{}, ErrWorkflowBindingMismatch
	}

	now := r.now().UTC()
	approvedAt := decision.CreatedAt.UTC()
	if approvedAt.After(now.Add(approvalFutureSkew)) {
		return executionauth.ResolvedApproval{}, ErrFutureWorkflowApproval
	}
	expiresAt := approvedAt.Add(approvalFreshnessLimit)
	if !now.Before(expiresAt) {
		return executionauth.ResolvedApproval{}, ErrStaleWorkflowApproval
	}

	decisionDigest, err := digestWorkflowDecision(*decision)
	if err != nil {
		return executionauth.ResolvedApproval{}, fmt.Errorf(
			"%w: %v",
			ErrInvalidWorkflowDecision,
			err,
		)
	}
	return executionauth.ResolvedApproval{
		SourceID:       sourceID,
		DecisionID:     decision.DecisionID,
		DecisionDigest: decisionDigest,
		BindingDigest:  bindingDigest,
		ApprovedBy:     decision.Actor,
		ApproverRoles:  []string{"owner"},
		ApprovedAt:     approvedAt,
		ExpiresAt:      expiresAt,
	}, nil
}

func parseWorkflowDecisionSource(sourceID string) (uuid.UUID, error) {
	if !strings.HasPrefix(sourceID, workflowDecisionPrefix) {
		return uuid.Nil, ErrInvalidWorkflowReference
	}
	rawID := strings.TrimPrefix(sourceID, workflowDecisionPrefix)
	id, err := uuid.Parse(rawID)
	if err != nil || id == uuid.Nil || sourceID != workflowDecisionPrefix+id.String() {
		return uuid.Nil, ErrInvalidWorkflowReference
	}
	return id, nil
}

func validateWorkflowDecision(
	decision WorkflowApprovalDecisionRecord,
	ownerIdentity string,
	decisionID uuid.UUID,
) error {
	storedDecisionID, err := uuid.Parse(decision.DecisionID)
	if err != nil ||
		storedDecisionID == uuid.Nil ||
		decision.DecisionID != storedDecisionID.String() ||
		storedDecisionID != decisionID {
		return fmt.Errorf("%w: decision id does not match", ErrInvalidWorkflowDecision)
	}
	workflowID, err := uuid.Parse(decision.WorkflowID)
	if err != nil || workflowID == uuid.Nil || decision.WorkflowID != workflowID.String() {
		return fmt.Errorf("%w: workflow id is invalid", ErrInvalidWorkflowDecision)
	}
	if err := validateOwner(decision.OwnerIdentity); err != nil ||
		decision.OwnerIdentity != ownerIdentity {
		return fmt.Errorf("%w: decision owner does not match", ErrInvalidWorkflowDecision)
	}
	if decision.DecisionType != workflowDecisionType ||
		decision.Decision != workflowDecisionValue ||
		!decision.Approved {
		return ErrWorkflowApprovalUnavailable
	}
	if decision.Actor == "" ||
		decision.Actor != strings.TrimSpace(decision.Actor) ||
		!utf8.ValidString(decision.Actor) ||
		len(decision.Actor) > maximumWorkflowActorBytes ||
		decision.Actor != ownerIdentity {
		return fmt.Errorf("%w: approver does not match the workflow owner", ErrInvalidWorkflowDecision)
	}
	if !utf8.ValidString(decision.Reason) ||
		utf8.RuneCountInString(decision.Reason) > maximumWorkflowReasonRunes {
		return fmt.Errorf("%w: approval reason is invalid", ErrInvalidWorkflowDecision)
	}
	if decision.CreatedAt.IsZero() {
		return fmt.Errorf("%w: approval timestamp is missing", ErrInvalidWorkflowDecision)
	}
	return nil
}

func workflowActionDigest(binding string) (string, error) {
	if binding == "" || binding != strings.TrimSpace(binding) ||
		!strings.HasPrefix(binding, workflowBindingPrefix) {
		return "", errors.New("action binding is missing or malformed")
	}
	parts := strings.Split(strings.TrimPrefix(binding, workflowBindingPrefix), ":")
	if len(parts) != 2 || !supportedWorkflowApprovalScope(parts[0]) {
		return "", errors.New("action binding scope is unsupported")
	}
	if !validSHA256Digest(parts[1]) {
		return "", errors.New("action binding digest is invalid")
	}
	return parts[1], nil
}

func supportedWorkflowApprovalScope(scope string) bool {
	switch scope {
	case "automation.api.read",
		"automation.api.mutate",
		"automation.script.execute",
		"automation.docker.start",
		"automation.agent-runtime.execute":
		return true
	default:
		return false
	}
}

func isNilWorkflowRepository(repository WorkflowApprovalDecisionRepository) bool {
	if repository == nil {
		return true
	}
	value := reflect.ValueOf(repository)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

type immutableWorkflowDecisionDigestV1 struct {
	ContractVersion int    `json:"contractVersion"`
	DecisionID      string `json:"decisionId"`
	WorkflowID      string `json:"workflowId"`
	OwnerIdentity   string `json:"ownerIdentity"`
	DecisionType    string `json:"decisionType"`
	Decision        string `json:"decision"`
	Reason          string `json:"reason"`
	ActionBinding   string `json:"actionBinding"`
	Approved        bool   `json:"approved"`
	Actor           string `json:"actor"`
	CreatedAt       string `json:"createdAt"`
}

func digestWorkflowDecision(decision WorkflowApprovalDecisionRecord) (string, error) {
	payload, err := json.Marshal(immutableWorkflowDecisionDigestV1{
		ContractVersion: 1,
		DecisionID:      decision.DecisionID,
		WorkflowID:      decision.WorkflowID,
		OwnerIdentity:   decision.OwnerIdentity,
		DecisionType:    decision.DecisionType,
		Decision:        decision.Decision,
		Reason:          decision.Reason,
		ActionBinding:   decision.ActionBinding,
		Approved:        decision.Approved,
		Actor:           decision.Actor,
		CreatedAt:       decision.CreatedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

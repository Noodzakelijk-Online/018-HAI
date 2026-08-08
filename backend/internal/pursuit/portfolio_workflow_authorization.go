package pursuit

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"automation-hub-backend/internal/executionauth"
	"automation-hub-backend/internal/models"
	"automation-hub-backend/internal/plangraph"
	"automation-hub-backend/internal/workflow"

	"github.com/google/uuid"
)

const (
	PortfolioWorkflowEffectAction                = "pursuit.portfolio.create-workflow"
	PortfolioWorkflowEffectResourceType          = "workflow-intake"
	PortfolioWorkflowEffectToolID                = "workflow.intake"
	PortfolioWorkflowEffectRuntimeID             = "hai-workflow-engine"
	PortfolioWorkflowEffectConfirmation          = "AUTHORIZE PORTFOLIO WORKFLOW EFFECT"
	PortfolioWorkflowEffectExecutionConfirmation = "CREATE APPROVED PORTFOLIO WORKFLOW"
	PortfolioWorkflowEffectApprovalSourcePrefix  = "portfolio-decision:"
	PortfolioWorkflowEffectAuthority             = "execution_authorization_only"
	PortfolioWorkflowEffectExecutionAuthority    = "workflow_effect_executed"
	PortfolioWorkflowEffectConsumer              = "pursuit-portfolio-workflow"
	PortfolioWorkflowEffectSourceType            = "portfolio_workflow_effect"
	PortfolioWorkflowEffectRelationship          = "portfolio_authorized_workflow"
	PortfolioWorkflowEffectEvent                 = "pursuit.portfolio_workflow_effect_executed"
	PortfolioWorkflowEffectActivityKeyPrefix     = "portfolio-workflow-effect:"
)

var (
	ErrPortfolioWorkflowApprovalUnavailable = errors.New("portfolio workflow approval is unavailable")
	ErrPortfolioWorkflowApprovalInvalid     = errors.New("portfolio workflow approval is invalid")
	ErrPortfolioWorkflowApprovalStale       = errors.New("portfolio workflow approval is stale")
	ErrPortfolioWorkflowBindingMismatch     = errors.New("portfolio workflow approval binding does not match the exact effect")
)

// PortfolioWorkflowEffect is the only concrete operation a portfolio proposal
// decision may authorize. All effect fields are derived by HAI from immutable
// server-side evidence; the HTTP caller cannot select a different tool,
// runtime, resource, autonomy level, or cost.
type PortfolioWorkflowEffect struct {
	Action                   string `json:"action"`
	Stage                    string `json:"stage"`
	ResourceType             string `json:"resourceType"`
	ResourceID               string `json:"resourceId"`
	ProjectKey               string `json:"projectKey,omitempty"`
	Domain                   string `json:"domain,omitempty"`
	ToolID                   string `json:"toolId"`
	RuntimeID                string `json:"runtimeId"`
	Risk                     string `json:"risk"`
	Reversible               bool   `json:"reversible"`
	EstimatedCost            int64  `json:"estimatedCostMicros"`
	ActionSummary            string `json:"actionSummary"`
	EffectDigest             string `json:"effectDigest"`
	ApprovalSource           string `json:"approvalSourceId"`
	CoordinationPlanID       string `json:"coordinationPlanId,omitempty"`
	CoordinationPlanRevision uint64 `json:"coordinationPlanRevision,omitempty"`
	CoordinationPlanDigest   string `json:"coordinationPlanDigest,omitempty"`
	CoordinationPlanNodeID   string `json:"coordinationPlanNodeId,omitempty"`
}

type PortfolioWorkflowEffectAuthorizationRequest struct {
	ExpectedItemDigest     string `json:"expectedItemDigest"`
	ExpectedDecisionDigest string `json:"expectedDecisionDigest"`
	Confirmation           string `json:"confirmation"`
}

type PortfolioWorkflowEffectAuthorizationResult struct {
	Effect     PortfolioWorkflowEffect `json:"effect"`
	Receipt    executionauth.Receipt   `json:"receipt"`
	Authority  string                  `json:"authority"`
	CanExecute bool                    `json:"canExecute"`
}

type PortfolioWorkflowEffectExecutionRequest struct {
	AuthorizationReceiptID string `json:"authorizationReceiptId"`
	ExpectedItemDigest     string `json:"expectedItemDigest"`
	ExpectedDecisionDigest string `json:"expectedDecisionDigest"`
	Confirmation           string `json:"confirmation"`
}

// PortfolioWorkflowEffectExecutionResult proves that one authorized receipt
// was consumed and applied to one review-gated local workflow. CanExecute is
// false because the created workflow still has its own approval and worker
// boundaries; this response cannot be reused as downstream authority.
type PortfolioWorkflowEffectExecutionResult struct {
	Effect        PortfolioWorkflowEffect   `json:"effect"`
	Receipt       executionauth.Receipt     `json:"receipt"`
	Consumption   executionauth.Consumption `json:"consumption"`
	PursuitID     uuid.UUID                 `json:"pursuitId"`
	WorkflowID    uuid.UUID                 `json:"workflowId"`
	WorkflowState string                    `json:"workflowState"`
	Replayed      bool                      `json:"replayed"`
	Authority     string                    `json:"authority"`
	CanExecute    bool                      `json:"canExecute"`
}

// PortfolioWorkflowEffectApprovalSnapshot is the owner-scoped durable evidence
// required both by the pursuit service and by executionauth's independent
// approval resolver.
type PortfolioWorkflowEffectApprovalSnapshot struct {
	Allocation     models.PursuitPortfolioAllocation
	Proposal       models.PursuitPortfolioExecutionProposal
	Item           models.PursuitPortfolioExecutionProposalItem
	AllocationItem models.PursuitPortfolioAllocationItem
	Pursuit        models.Pursuit
	Decision       models.PursuitPortfolioExecutionProposalDecision
	LatestDecision *models.PursuitPortfolioExecutionProposalDecision
	Settled        bool
}

type PortfolioWorkflowEffectApprovalRepository interface {
	LoadPortfolioWorkflowEffectApprovalSnapshot(
		context.Context,
		string,
		uuid.UUID,
	) (*PortfolioWorkflowEffectApprovalSnapshot, error)
}

type portfolioWorkflowEffectAuthorizer interface {
	Authorize(context.Context, executionauth.Request) (executionauth.Receipt, error)
}

type portfolioWorkflowEffectExecutor interface {
	AuthorizeAndConsume(context.Context, executionauth.Request, string, string) (executionauth.Receipt, error)
	Get(context.Context, string, uuid.UUID) (executionauth.Receipt, error)
	GetConsumption(context.Context, string, uuid.UUID) (executionauth.Consumption, error)
}

// PortfolioWorkflowEffectApprovalResolver lets the canonical execution
// authorization service independently resolve one current, unexpired owner
// approval. It never grants authority by itself.
type PortfolioWorkflowEffectApprovalResolver struct {
	repository           PortfolioWorkflowEffectApprovalRepository
	acceptedPlanResolver plangraph.AcceptedRevisionResolver
	now                  func() time.Time
}

var _ executionauth.ApprovalResolver = (*PortfolioWorkflowEffectApprovalResolver)(nil)

func NewPortfolioWorkflowEffectApprovalResolver(
	repository PortfolioWorkflowEffectApprovalRepository,
	acceptedPlanResolvers ...plangraph.AcceptedRevisionResolver,
) (*PortfolioWorkflowEffectApprovalResolver, error) {
	if repository == nil {
		return nil, fmt.Errorf("portfolio workflow approval repository is required")
	}
	if len(acceptedPlanResolvers) > 1 {
		return nil, fmt.Errorf("at most one accepted plan resolver may be configured")
	}
	var acceptedPlanResolver plangraph.AcceptedRevisionResolver
	if len(acceptedPlanResolvers) == 1 {
		acceptedPlanResolver = acceptedPlanResolvers[0]
	} else if gormRepository, ok := repository.(*GormRepository); ok && gormRepository.DB != nil {
		acceptedPlanResolver = plangraph.NewService(
			plangraph.NewGormRepository(gormRepository.DB), nil,
		)
	}
	return &PortfolioWorkflowEffectApprovalResolver{
		repository:           repository,
		acceptedPlanResolver: acceptedPlanResolver,
		now:                  func() time.Time { return time.Now().UTC() },
	}, nil
}

func (r *PortfolioWorkflowEffectApprovalResolver) Resolve(
	ctx context.Context,
	ownerIdentity string,
	sourceID string,
	bindingDigest string,
) (executionauth.ResolvedApproval, error) {
	if r == nil || r.repository == nil || r.now == nil {
		return executionauth.ResolvedApproval{}, ErrPortfolioWorkflowApprovalUnavailable
	}
	if ctx == nil {
		return executionauth.ResolvedApproval{}, fmt.Errorf("context is required")
	}
	if err := ctx.Err(); err != nil {
		return executionauth.ResolvedApproval{}, err
	}
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	if ownerIdentity == "" || len(ownerIdentity) > 255 {
		return executionauth.ResolvedApproval{}, ErrPortfolioWorkflowApprovalInvalid
	}
	decisionID, err := parsePortfolioWorkflowApprovalSource(sourceID)
	if err != nil {
		return executionauth.ResolvedApproval{}, err
	}
	if !validPortfolioRecordDigest(bindingDigest) {
		return executionauth.ResolvedApproval{}, ErrPortfolioWorkflowBindingMismatch
	}
	snapshot, err := r.repository.LoadPortfolioWorkflowEffectApprovalSnapshot(
		ctx, ownerIdentity, decisionID,
	)
	if err != nil {
		return executionauth.ResolvedApproval{}, fmt.Errorf("%w: %v", ErrPortfolioWorkflowApprovalUnavailable, err)
	}
	if snapshot == nil {
		return executionauth.ResolvedApproval{}, ErrPortfolioWorkflowApprovalUnavailable
	}
	if err := validatePortfolioWorkflowEffectApproval(ownerIdentity, snapshot, r.now().UTC()); err != nil {
		return executionauth.ResolvedApproval{}, err
	}
	if _, err := revalidatePortfolioWorkflowCoordinationPlan(
		ctx, ownerIdentity, snapshot, r.acceptedPlanResolver,
	); err != nil {
		return executionauth.ResolvedApproval{}, err
	}
	effect, err := buildPortfolioWorkflowEffect(snapshot)
	if err != nil {
		return executionauth.ResolvedApproval{}, err
	}
	if effect.EffectDigest != bindingDigest {
		return executionauth.ResolvedApproval{}, ErrPortfolioWorkflowBindingMismatch
	}
	return executionauth.ResolvedApproval{
		SourceID:       sourceID,
		DecisionID:     snapshot.Decision.ID.String(),
		DecisionDigest: snapshot.Decision.RecordDigest,
		BindingDigest:  effect.EffectDigest,
		ApprovedBy:     snapshot.Decision.Actor,
		ApproverRoles:  []string{"owner"},
		ApprovedAt:     snapshot.Decision.DecidedAt,
		ExpiresAt:      *snapshot.Decision.ExpiresAt,
	}, nil
}

// WithPortfolioWorkflowEffectAuthorization installs the canonical policy
// service after router composition. It does not install an executor.
func WithPortfolioWorkflowEffectAuthorization(
	value Service,
	authorizer portfolioWorkflowEffectAuthorizer,
) (Service, error) {
	concrete, ok := value.(*service)
	if !ok || concrete == nil {
		return nil, fmt.Errorf("portfolio workflow authorization requires the canonical pursuit service")
	}
	if authorizer == nil {
		return nil, fmt.Errorf("portfolio workflow execution authorizer is required")
	}
	concrete.portfolioWorkflowAuthorizer = authorizer
	return concrete, nil
}

// WithPortfolioWorkflowEffectExecution installs the canonical single-use
// receipt consumer. Keeping this separate from authorization prevents an
// authorization-only composition from accidentally acquiring execution power.
func WithPortfolioWorkflowEffectExecution(
	value Service,
	executor portfolioWorkflowEffectExecutor,
) (Service, error) {
	concrete, ok := value.(*service)
	if !ok || concrete == nil {
		return nil, fmt.Errorf("portfolio workflow execution requires the canonical pursuit service")
	}
	if executor == nil {
		return nil, fmt.Errorf("portfolio workflow execution consumer is required")
	}
	concrete.portfolioWorkflowExecutor = executor
	return concrete, nil
}

// AuthorizePortfolioWorkflowEffectForOwner asks the canonical execution policy
// service for an immutable receipt. Even an authorized receipt is deliberately
// left unconsumed, so this method cannot create a workflow or perform any
// external effect.
func (s *service) AuthorizePortfolioWorkflowEffectForOwner(
	ctx context.Context,
	ownerIdentity, actor string,
	itemID uuid.UUID,
	request PortfolioWorkflowEffectAuthorizationRequest,
) (*PortfolioWorkflowEffectAuthorizationResult, error) {
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	actor = strings.TrimSpace(actor)
	request.ExpectedItemDigest = strings.TrimSpace(request.ExpectedItemDigest)
	request.ExpectedDecisionDigest = strings.TrimSpace(request.ExpectedDecisionDigest)
	request.Confirmation = strings.TrimSpace(request.Confirmation)
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}
	if ownerIdentity == "" || actor == "" || ownerIdentity != actor {
		return nil, fmt.Errorf("the authenticated owner must authorize a portfolio workflow effect")
	}
	if itemID == uuid.Nil {
		return nil, fmt.Errorf("a valid portfolio execution proposal item id is required")
	}
	if !validPortfolioRecordDigest(request.ExpectedItemDigest) ||
		!validPortfolioRecordDigest(request.ExpectedDecisionDigest) {
		return nil, fmt.Errorf("exact proposal item and approval decision digests are required")
	}
	if request.Confirmation != PortfolioWorkflowEffectConfirmation {
		return nil, fmt.Errorf("exact portfolio workflow effect authorization confirmation is required")
	}
	if s.portfolioWorkflowAuthorizer == nil {
		return nil, fmt.Errorf("portfolio workflow execution authorization is unavailable")
	}
	repository, ok := s.repo.(PortfolioWorkflowEffectApprovalRepository)
	if !ok {
		return nil, fmt.Errorf("durable portfolio workflow approval storage is unavailable")
	}
	decisionSnapshot, err := loadCurrentPortfolioWorkflowApprovalForItem(
		ctx, repository, ownerIdentity, itemID,
	)
	if err != nil {
		return nil, err
	}
	if decisionSnapshot.Item.RecordDigest != request.ExpectedItemDigest ||
		decisionSnapshot.Decision.RecordDigest != request.ExpectedDecisionDigest {
		return nil, fmt.Errorf("portfolio workflow approval evidence changed; inspect the current item and decision")
	}
	if err := validatePortfolioWorkflowEffectApproval(
		ownerIdentity, decisionSnapshot, time.Now().UTC(),
	); err != nil {
		return nil, err
	}
	if _, err := s.revalidatePortfolioAllocationCoordinationPlan(
		ownerIdentity, &decisionSnapshot.Allocation,
		[]models.PursuitPortfolioAllocationItem{decisionSnapshot.AllocationItem},
	); err != nil {
		return nil, err
	}
	effect, err := buildPortfolioWorkflowEffect(decisionSnapshot)
	if err != nil {
		return nil, err
	}
	authorizationRequest := buildPortfolioWorkflowAuthorizationRequest(
		ownerIdentity, actor, itemID, request.ExpectedItemDigest,
		request.ExpectedDecisionDigest, decisionSnapshot, effect,
	)
	receipt, err := s.portfolioWorkflowAuthorizer.Authorize(ctx, authorizationRequest)
	if err != nil {
		return nil, fmt.Errorf("authorize portfolio workflow effect: %w", err)
	}
	if !portfolioWorkflowReceiptMatches(receipt, authorizationRequest, effect) {
		return nil, fmt.Errorf("portfolio workflow authorization receipt does not match the exact effect")
	}
	return &PortfolioWorkflowEffectAuthorizationResult{
		Effect: effect, Receipt: receipt,
		Authority: PortfolioWorkflowEffectAuthority, CanExecute: false,
	}, nil
}

func buildPortfolioWorkflowAuthorizationRequest(
	ownerIdentity, actor string,
	itemID uuid.UUID,
	itemDigest, decisionDigest string,
	snapshot *PortfolioWorkflowEffectApprovalSnapshot,
	effect PortfolioWorkflowEffect,
) executionauth.Request {
	requestAt := snapshot.Decision.DecidedAt.UTC()
	request := executionauth.Request{
		OwnerIdentity:         ownerIdentity,
		IdempotencyKey:        "portfolio-workflow:" + snapshot.Decision.ID.String(),
		ActorIdentity:         actor,
		ActorKind:             executionauth.ActorHuman,
		TaskID:                "portfolio-item:" + itemID.String(),
		Action:                effect.Action,
		Stage:                 executionauth.StageExecution,
		ResourceType:          effect.ResourceType,
		ResourceID:            effect.ResourceID,
		ProjectKey:            effect.ProjectKey,
		Domain:                effect.Domain,
		ToolID:                effect.ToolID,
		RuntimeID:             effect.RuntimeID,
		RequiredAuthority:     1,
		RequestedAutonomy:     6,
		Risk:                  executionauth.RiskLevel(effect.Risk),
		Reversible:            effect.Reversible,
		EstimatedCostEUR:      0,
		ApprovalSourceID:      effect.ApprovalSource,
		ApprovalBindingDigest: effect.EffectDigest,
		EffectDigest:          effect.EffectDigest,
		Facts: map[string]string{
			"proposalItemDigest": itemDigest,
			"stateDigest":        snapshot.Item.StateDigest,
			"decisionDigest":     decisionDigest,
		},
		SourceReferences: []string{
			"hai://pursuits/" + snapshot.Pursuit.ID.String() +
				"/portfolio-execution-proposal-items/" + itemID.String(),
		},
		RequestedAt: requestAt,
	}
	if planURI := portfolioWorkflowCoordinationPlanURI(effect); planURI != "" {
		request.Facts["coordinationPlanId"] = effect.CoordinationPlanID
		request.Facts["coordinationPlanRevision"] = fmt.Sprintf("%d", effect.CoordinationPlanRevision)
		request.Facts["coordinationPlanDigest"] = effect.CoordinationPlanDigest
		request.Facts["coordinationPlanNodeId"] = effect.CoordinationPlanNodeID
		request.SourceReferences = append(request.SourceReferences, planURI)
	}
	if snapshot.Pursuit.MandateID != nil {
		request.MandateID = snapshot.Pursuit.MandateID.String()
	}
	return request
}

func portfolioWorkflowReceiptMatches(
	receipt executionauth.Receipt,
	request executionauth.Request,
	effect PortfolioWorkflowEffect,
) bool {
	if receipt.OwnerIdentity != request.OwnerIdentity || receipt.ActorIdentity != request.ActorIdentity ||
		receipt.ActorKind != executionauth.ActorHuman || receipt.TaskID != request.TaskID ||
		receipt.EffectDigest != effect.EffectDigest || receipt.ApprovalSourceID != effect.ApprovalSource ||
		receipt.Action != effect.Action || receipt.Stage != executionauth.StageExecution ||
		receipt.ResourceType != effect.ResourceType || receipt.ResourceID != effect.ResourceID ||
		receipt.ProjectKey != effect.ProjectKey || receipt.Domain != effect.Domain ||
		receipt.RuntimeID != effect.RuntimeID || receipt.RequiredAuthority != request.RequiredAuthority ||
		receipt.RequestedAutonomy != request.RequestedAutonomy ||
		receipt.Risk != request.Risk || receipt.Reversible != effect.Reversible ||
		receipt.EstimatedCostEUR != request.EstimatedCostEUR {
		return false
	}
	return receipt.OwnerIdentity == request.OwnerIdentity &&
		receipt.ActorIdentity == request.ActorIdentity &&
		receipt.IdempotencyKey == request.IdempotencyKey
}

func portfolioWorkflowExecutionTarget(effectDigest string) (string, error) {
	effectDigest = strings.TrimSpace(effectDigest)
	if !validPortfolioRecordDigest(effectDigest) {
		return "", fmt.Errorf("portfolio workflow effect digest is invalid")
	}
	return "workflow-intake:" + effectDigest, nil
}

// ExecutePortfolioWorkflowEffectForOwner consumes one exact authorization
// receipt and applies it only to local workflow intake. A retry after a network
// or process interruption resumes the same receipt-bound intake; it can never
// select a different pursuit, tool, runtime, or action.
func (s *service) ExecutePortfolioWorkflowEffectForOwner(
	ctx context.Context,
	ownerIdentity, actor string,
	itemID uuid.UUID,
	request PortfolioWorkflowEffectExecutionRequest,
) (*PortfolioWorkflowEffectExecutionResult, error) {
	ownerIdentity = strings.TrimSpace(ownerIdentity)
	actor = strings.TrimSpace(actor)
	request.AuthorizationReceiptID = strings.TrimSpace(request.AuthorizationReceiptID)
	request.ExpectedItemDigest = strings.TrimSpace(request.ExpectedItemDigest)
	request.ExpectedDecisionDigest = strings.TrimSpace(request.ExpectedDecisionDigest)
	request.Confirmation = strings.TrimSpace(request.Confirmation)
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}
	if ownerIdentity == "" || actor == "" || ownerIdentity != actor {
		return nil, fmt.Errorf("the authenticated owner must create the approved portfolio workflow")
	}
	if itemID == uuid.Nil {
		return nil, fmt.Errorf("a valid portfolio execution proposal item id is required")
	}
	receiptID, err := uuid.Parse(request.AuthorizationReceiptID)
	if err != nil || receiptID == uuid.Nil {
		return nil, fmt.Errorf("a valid execution authorization receipt id is required")
	}
	if !validPortfolioRecordDigest(request.ExpectedItemDigest) ||
		!validPortfolioRecordDigest(request.ExpectedDecisionDigest) {
		return nil, fmt.Errorf("exact proposal item and approval decision digests are required")
	}
	if request.Confirmation != PortfolioWorkflowEffectExecutionConfirmation {
		return nil, fmt.Errorf("exact portfolio workflow creation confirmation is required")
	}
	if s.portfolioWorkflowExecutor == nil {
		return nil, fmt.Errorf("portfolio workflow execution is unavailable")
	}
	if s.workflowService == nil {
		return nil, fmt.Errorf("workflow intake is unavailable")
	}
	repository, ok := s.repo.(PortfolioWorkflowEffectApprovalRepository)
	if !ok {
		return nil, fmt.Errorf("durable portfolio workflow approval storage is unavailable")
	}
	receipt, err := s.portfolioWorkflowExecutor.Get(ctx, ownerIdentity, receiptID)
	if err != nil {
		return nil, fmt.Errorf("load portfolio workflow authorization receipt: %w", err)
	}
	decisionID, err := portfolioWorkflowDecisionIDFromApprovalSource(receipt.ApprovalSourceID)
	if err != nil {
		return nil, err
	}
	snapshot, err := repository.LoadPortfolioWorkflowEffectApprovalSnapshot(
		ctx, ownerIdentity, decisionID,
	)
	if err != nil {
		return nil, fmt.Errorf("load receipt-bound portfolio workflow approval: %w", err)
	}
	if snapshot == nil || snapshot.Item.ID != itemID {
		return nil, fmt.Errorf("portfolio workflow authorization receipt is bound to a different proposal item")
	}
	if snapshot.Item.RecordDigest != request.ExpectedItemDigest ||
		snapshot.Decision.RecordDigest != request.ExpectedDecisionDigest {
		return nil, fmt.Errorf("portfolio workflow approval evidence changed; inspect the current item and decision")
	}
	effect, err := buildPortfolioWorkflowEffect(snapshot)
	if err != nil {
		return nil, err
	}
	authorizationRequest := buildPortfolioWorkflowAuthorizationRequest(
		ownerIdentity, actor, itemID, request.ExpectedItemDigest,
		request.ExpectedDecisionDigest, snapshot, effect,
	)
	if !portfolioWorkflowReceiptMatches(receipt, authorizationRequest, effect) {
		return nil, fmt.Errorf("portfolio workflow authorization receipt does not match the exact approved effect")
	}
	if receipt.Outcome != executionauth.OutcomeAuthorized {
		return nil, fmt.Errorf("portfolio workflow authorization receipt is not authorized")
	}
	target, err := portfolioWorkflowExecutionTarget(effect.EffectDigest)
	if err != nil {
		return nil, err
	}
	replayed := false
	consumption, consumptionErr := s.portfolioWorkflowExecutor.GetConsumption(ctx, ownerIdentity, receiptID)
	switch {
	case consumptionErr == nil:
		if !portfolioWorkflowConsumptionMatches(consumption, receipt, target) {
			return nil, fmt.Errorf("portfolio workflow authorization receipt was consumed for a different effect")
		}
		replayed = true
	case errors.Is(consumptionErr, executionauth.ErrNotFound):
		// The first exercise must still be backed by the current approval. Once
		// the exact receipt is consumed, a retry may finish the same idempotent
		// local effect even if the approval expires during interruption recovery.
		if err := validatePortfolioWorkflowEffectApproval(ownerIdentity, snapshot, time.Now().UTC()); err != nil {
			return nil, err
		}
		if _, err := s.revalidatePortfolioAllocationCoordinationPlan(
			ownerIdentity, &snapshot.Allocation,
			[]models.PursuitPortfolioAllocationItem{snapshot.AllocationItem},
		); err != nil {
			return nil, err
		}
		consumedReceipt, consumeErr := s.portfolioWorkflowExecutor.AuthorizeAndConsume(
			ctx, authorizationRequest, PortfolioWorkflowEffectConsumer, target,
		)
		if consumeErr != nil {
			if !errors.Is(consumeErr, executionauth.ErrAlreadyConsumed) {
				return nil, fmt.Errorf("consume portfolio workflow authorization: %w", consumeErr)
			}
			replayed = true
		} else if consumedReceipt.ID != receiptID ||
			!portfolioWorkflowReceiptMatches(consumedReceipt, authorizationRequest, effect) {
			return nil, fmt.Errorf("consumed portfolio workflow receipt does not match the reviewed authorization")
		}
		consumption, err = s.portfolioWorkflowExecutor.GetConsumption(ctx, ownerIdentity, receiptID)
		if err != nil || !portfolioWorkflowConsumptionMatches(consumption, receipt, target) {
			return nil, fmt.Errorf("verify portfolio workflow authorization consumption: %w", firstPortfolioError(err, executionauth.ErrFinalEffectMismatch))
		}
	default:
		return nil, fmt.Errorf("inspect portfolio workflow authorization consumption: %w", consumptionErr)
	}

	receiptURI := "hai://execution-authorization-receipts/" + receipt.ID.String()
	record, linked, err := s.loadPortfolioWorkflowEffect(snapshot.Pursuit.ID, ownerIdentity, receipt, receiptURI)
	if err != nil {
		return nil, err
	}
	if !linked {
		intakeRequest := workflow.IntakeRequest{
			OwnerIdentity:    ownerIdentity,
			Input:            effect.ActionSummary,
			ProjectKey:       effect.ProjectKey,
			MandateID:        optionalUUIDString(snapshot.Pursuit.MandateID),
			SourceType:       PortfolioWorkflowEffectSourceType,
			SourceID:         receipt.ID.String(),
			SourceURI:        receiptURI,
			SourceLabel:      "Approved portfolio workflow effect",
			ContentType:      "portfolio_workflow_effect",
			Trigger:          PortfolioWorkflowEffectAction,
			Actor:            actor,
			RequiresReview:   true,
			ReviewReason:     "This receipt authorizes workflow creation only; downstream execution requires its own exact approval.",
			CoordinationPlan: coordinationReferenceForAllocation(&snapshot.Allocation),
		}
		if recovery, ok := s.workflowService.(workflow.AuthorizedEffectRecoveryIntake); replayed && ok {
			record, err = recovery.IntakeAuthorizedEffectRecovery(intakeRequest)
		} else {
			record, err = s.workflowService.Intake(intakeRequest)
		}
		if err != nil {
			return nil, fmt.Errorf("apply consumed portfolio workflow effect: %w", err)
		}
		if err := validatePortfolioWorkflowEffectRecord(record, ownerIdentity, receipt, receiptURI); err != nil {
			return nil, err
		}
		if _, err := s.Link(snapshot.Pursuit.ID, LinkRequest{
			OwnerIdentity: ownerIdentity,
			LinkType:      LinkWorkflow,
			LinkID:        record.Item.ID.String(),
			Relationship:  PortfolioWorkflowEffectRelationship,
			SourceURI:     receiptURI,
			SourceLabel:   "Approved portfolio workflow effect",
			Confidence:    1,
			Actor:         actor,
		}); err != nil {
			return nil, fmt.Errorf("link receipt-bound workflow to pursuit: %w", err)
		}
	}
	recorded, err := s.portfolioWorkflowEffectActivityExists(
		snapshot.Pursuit.ID, record.Item.ID, receiptURI,
	)
	if err != nil {
		return nil, fmt.Errorf("inspect portfolio workflow effect audit: %w", err)
	}
	if !recorded {
		if _, err := s.recordActivityIdempotent(
			snapshot.Pursuit.ID,
			PortfolioWorkflowEffectEvent,
			"Consumed one exact authorization receipt and created a review-gated local workflow.",
			actor,
			LinkWorkflow,
			record.Item.ID.String(),
			receiptURI,
			PortfolioWorkflowEffectActivityKeyPrefix+receipt.ID.String(),
		); err != nil {
			return nil, fmt.Errorf("record portfolio workflow effect audit: %w", err)
		}
	}
	return &PortfolioWorkflowEffectExecutionResult{
		Effect: effect, Receipt: receipt, Consumption: consumption,
		PursuitID: snapshot.Pursuit.ID, WorkflowID: record.Item.ID,
		WorkflowState: record.Item.CurrentState, Replayed: replayed,
		Authority: PortfolioWorkflowEffectExecutionAuthority, CanExecute: false,
	}, nil
}

func (s *service) loadPortfolioWorkflowEffect(
	pursuitID uuid.UUID,
	ownerIdentity string,
	receipt executionauth.Receipt,
	receiptURI string,
) (*workflow.WorkflowRecord, bool, error) {
	links, err := s.repo.FindLinks(pursuitID)
	if err != nil {
		return nil, false, fmt.Errorf("inspect receipt-bound workflow link: %w", err)
	}
	workflowID := uuid.Nil
	for _, link := range links {
		if link.LinkType != LinkWorkflow || link.Relationship != PortfolioWorkflowEffectRelationship ||
			strings.TrimSpace(link.SourceURI) != receiptURI {
			continue
		}
		linkedID, parseErr := uuid.Parse(strings.TrimSpace(link.LinkID))
		if parseErr != nil || linkedID == uuid.Nil {
			return nil, false, fmt.Errorf("receipt-bound workflow link has an invalid workflow identity")
		}
		if workflowID != uuid.Nil && workflowID != linkedID {
			return nil, false, fmt.Errorf("authorization receipt is linked to conflicting workflows")
		}
		workflowID = linkedID
	}
	if workflowID == uuid.Nil {
		return nil, false, nil
	}
	items, err := s.repo.FindLinkedWorkflows([]uuid.UUID{workflowID})
	if err != nil {
		return nil, false, fmt.Errorf("load receipt-bound workflow: %w", err)
	}
	if len(items) != 1 || items[0].ID != workflowID {
		return nil, false, fmt.Errorf("receipt-bound workflow is missing or ambiguous")
	}
	record := &workflow.WorkflowRecord{Item: items[0]}
	if err := validatePortfolioWorkflowEffectRecord(record, ownerIdentity, receipt, receiptURI); err != nil {
		return nil, false, err
	}
	return record, true, nil
}

func validatePortfolioWorkflowEffectRecord(
	record *workflow.WorkflowRecord,
	ownerIdentity string,
	receipt executionauth.Receipt,
	receiptURI string,
) error {
	if record == nil || record.Item.ID == uuid.Nil ||
		strings.TrimSpace(record.Item.OwnerIdentity) != ownerIdentity ||
		record.Item.SourceType != PortfolioWorkflowEffectSourceType ||
		record.Item.SourceID != receipt.ID.String() ||
		strings.TrimSpace(record.Item.SourceURI) != receiptURI {
		return fmt.Errorf("workflow intake did not return the exact receipt-bound workflow")
	}
	return nil
}

func (s *service) portfolioWorkflowEffectActivityExists(
	pursuitID uuid.UUID,
	workflowID uuid.UUID,
	receiptURI string,
) (bool, error) {
	_, found, err := s.repo.FindActivityByIdentity(
		pursuitID,
		PortfolioWorkflowEffectEvent,
		LinkWorkflow,
		workflowID.String(),
		receiptURI,
	)
	return found, err
}

func portfolioWorkflowConsumptionMatches(
	consumption executionauth.Consumption,
	receipt executionauth.Receipt,
	target string,
) bool {
	return consumption.ReceiptID == receipt.ID &&
		consumption.OwnerIdentity == receipt.OwnerIdentity &&
		consumption.Consumer == PortfolioWorkflowEffectConsumer &&
		consumption.ExecutionTarget == target &&
		consumption.ReceiptDigest == receipt.DecisionDigest &&
		!consumption.ConsumedAt.IsZero()
}

func portfolioWorkflowDecisionIDFromApprovalSource(sourceID string) (uuid.UUID, error) {
	sourceID = strings.TrimSpace(sourceID)
	if !strings.HasPrefix(sourceID, PortfolioWorkflowEffectApprovalSourcePrefix) {
		return uuid.Nil, fmt.Errorf("portfolio workflow authorization receipt has an invalid approval source")
	}
	decisionID, err := uuid.Parse(strings.TrimPrefix(sourceID, PortfolioWorkflowEffectApprovalSourcePrefix))
	if err != nil || decisionID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("portfolio workflow authorization receipt has an invalid approval decision")
	}
	return decisionID, nil
}

func optionalUUIDString(value *uuid.UUID) string {
	if value == nil || *value == uuid.Nil {
		return ""
	}
	return value.String()
}

func firstPortfolioError(primary, fallback error) error {
	if primary != nil {
		return primary
	}
	return fallback
}

func loadCurrentPortfolioWorkflowApprovalForItem(
	ctx context.Context,
	repository PortfolioWorkflowEffectApprovalRepository,
	ownerIdentity string,
	itemID uuid.UUID,
) (*PortfolioWorkflowEffectApprovalSnapshot, error) {
	// The existing decision snapshot lookup is deliberately used only to find
	// the current decision ID; the exported repository lookup independently
	// reloads the complete owner-scoped evidence used by executionauth.
	decisionRepository, ok := repository.(interface {
		LoadPortfolioExecutionProposalDecisionSnapshot(string, uuid.UUID) (*portfolioExecutionProposalDecisionSnapshot, error)
	})
	if !ok {
		return nil, fmt.Errorf("portfolio workflow approval lookup is unavailable")
	}
	current, err := decisionRepository.LoadPortfolioExecutionProposalDecisionSnapshot(ownerIdentity, itemID)
	if err != nil {
		return nil, fmt.Errorf("load current portfolio workflow approval: %w", err)
	}
	if current == nil || current.LatestDecision == nil {
		return nil, ErrPortfolioWorkflowApprovalUnavailable
	}
	return repository.LoadPortfolioWorkflowEffectApprovalSnapshot(
		ctx, ownerIdentity, current.LatestDecision.ID,
	)
}

func validatePortfolioWorkflowEffectApproval(
	ownerIdentity string,
	snapshot *PortfolioWorkflowEffectApprovalSnapshot,
	now time.Time,
) error {
	if snapshot == nil || snapshot.LatestDecision == nil ||
		snapshot.Decision.ID == uuid.Nil || snapshot.LatestDecision.ID != snapshot.Decision.ID {
		return ErrPortfolioWorkflowApprovalUnavailable
	}
	source := &portfolioExecutionProposalDecisionSnapshot{
		Allocation: snapshot.Allocation, Proposal: snapshot.Proposal, Item: snapshot.Item,
		AllocationItem: snapshot.AllocationItem, Pursuit: snapshot.Pursuit,
		Settled: snapshot.Settled, LatestDecision: snapshot.LatestDecision,
	}
	if err := validatePortfolioExecutionDecisionSource(ownerIdentity, source); err != nil {
		return fmt.Errorf("%w: %v", ErrPortfolioWorkflowApprovalInvalid, err)
	}
	if snapshot.Decision.Decision != PortfolioExecutionDecisionApproved ||
		snapshot.Decision.ExpiresAt == nil {
		return ErrPortfolioWorkflowApprovalUnavailable
	}
	if !now.Before(snapshot.Decision.ExpiresAt.UTC()) {
		return ErrPortfolioWorkflowApprovalStale
	}
	if snapshot.Settled {
		return fmt.Errorf("%w: accepted resource reservation is already settled", ErrPortfolioWorkflowApprovalInvalid)
	}
	settled := map[uuid.UUID]struct{}{}
	stateDigest, err := digestPortfolioExecutionState(
		snapshot.Pursuit, snapshot.AllocationItem, settled,
	)
	if err != nil || stateDigest != snapshot.Item.StateDigest {
		return fmt.Errorf("%w: pursuit state changed; prepare and approve a fresh proposal", ErrPortfolioWorkflowApprovalInvalid)
	}
	return nil
}

func buildPortfolioWorkflowEffect(
	snapshot *PortfolioWorkflowEffectApprovalSnapshot,
) (PortfolioWorkflowEffect, error) {
	if snapshot == nil {
		return PortfolioWorkflowEffect{}, ErrPortfolioWorkflowApprovalInvalid
	}
	if err := validatePortfolioCoordinationBindingShape(&snapshot.Allocation); err != nil {
		return PortfolioWorkflowEffect{}, fmt.Errorf("%w: %v", ErrPortfolioWorkflowApprovalInvalid, err)
	}
	risk := normalizePortfolioWorkflowRisk(snapshot.Item.RiskLevel)
	approvalSource := PortfolioWorkflowEffectApprovalSourcePrefix + snapshot.Decision.ID.String()
	effect := PortfolioWorkflowEffect{
		Action: PortfolioWorkflowEffectAction, Stage: string(executionauth.StageExecution),
		ResourceType: PortfolioWorkflowEffectResourceType, ResourceID: snapshot.Item.ID.String(),
		ProjectKey: strings.TrimSpace(snapshot.Pursuit.ProjectKey),
		Domain:     strings.ToLower(strings.TrimSpace(snapshot.Pursuit.Domain)),
		ToolID:     PortfolioWorkflowEffectToolID, RuntimeID: PortfolioWorkflowEffectRuntimeID,
		Risk: string(risk), Reversible: true, EstimatedCost: 0,
		ActionSummary: snapshot.Item.ActionSummary, ApprovalSource: approvalSource,
	}
	if reference := coordinationReferenceForAllocation(&snapshot.Allocation); !reference.IsZero() {
		effect.CoordinationPlanID = reference.PlanID.String()
		effect.CoordinationPlanRevision = reference.Revision
		effect.CoordinationPlanDigest = strings.ToLower(strings.TrimSpace(reference.Digest))
		effect.CoordinationPlanNodeID = strings.TrimSpace(reference.NodeID)
	}
	if effect.CoordinationPlanID == "" {
		digest, err := digestPortfolioPayload(struct {
			ContractVersion                                                            int
			ProposalItemID, ProposalItemDigest, StateDigest                            string
			DecisionID, DecisionDigest, Action, Stage, ResourceType, ResourceID        string
			ProjectKey, Domain, ToolID, RuntimeID, Risk, ActionSummary, ApprovalSource string
			Reversible                                                                 bool
			EstimatedCostMicros                                                        int64
		}{
			1, snapshot.Item.ID.String(), snapshot.Item.RecordDigest, snapshot.Item.StateDigest,
			snapshot.Decision.ID.String(), snapshot.Decision.RecordDigest,
			effect.Action, effect.Stage, effect.ResourceType, effect.ResourceID,
			effect.ProjectKey, effect.Domain, effect.ToolID, effect.RuntimeID, effect.Risk,
			effect.ActionSummary, effect.ApprovalSource, effect.Reversible, effect.EstimatedCost,
		})
		if err != nil {
			return PortfolioWorkflowEffect{}, err
		}
		effect.EffectDigest = digest
		return effect, nil
	}
	digest, err := digestPortfolioPayload(struct {
		ContractVersion                                                            int
		ProposalItemID, ProposalItemDigest, StateDigest                            string
		DecisionID, DecisionDigest, Action, Stage, ResourceType, ResourceID        string
		ProjectKey, Domain, ToolID, RuntimeID, Risk, ActionSummary, ApprovalSource string
		CoordinationPlanID, CoordinationPlanDigest, CoordinationPlanNodeID         string
		CoordinationPlanRevision                                                   uint64
		Reversible                                                                 bool
		EstimatedCostMicros                                                        int64
	}{
		2, snapshot.Item.ID.String(), snapshot.Item.RecordDigest, snapshot.Item.StateDigest,
		snapshot.Decision.ID.String(), snapshot.Decision.RecordDigest,
		effect.Action, effect.Stage, effect.ResourceType, effect.ResourceID,
		effect.ProjectKey, effect.Domain, effect.ToolID, effect.RuntimeID, effect.Risk,
		effect.ActionSummary, effect.ApprovalSource,
		effect.CoordinationPlanID, effect.CoordinationPlanDigest, effect.CoordinationPlanNodeID,
		effect.CoordinationPlanRevision, effect.Reversible, effect.EstimatedCost,
	})
	if err != nil {
		return PortfolioWorkflowEffect{}, err
	}
	effect.EffectDigest = digest
	return effect, nil
}

func revalidatePortfolioWorkflowCoordinationPlan(
	ctx context.Context,
	ownerIdentity string,
	snapshot *PortfolioWorkflowEffectApprovalSnapshot,
	resolver plangraph.AcceptedRevisionResolver,
) (*plangraph.AcceptedRevisionBinding, error) {
	if snapshot == nil {
		return nil, ErrPortfolioWorkflowApprovalInvalid
	}
	if err := validatePortfolioCoordinationBindingShape(&snapshot.Allocation); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPortfolioWorkflowApprovalInvalid, err)
	}
	reference := coordinationReferenceForAllocation(&snapshot.Allocation)
	if reference.IsZero() {
		return nil, nil
	}
	if resolver == nil {
		return nil, fmt.Errorf("accepted coordination plan validation is unavailable")
	}
	binding, err := resolver.ResolveAccepted(ctx, strings.TrimSpace(ownerIdentity), reference)
	if err != nil {
		return nil, fmt.Errorf("validate accepted coordination plan: %w", err)
	}
	if binding == nil || binding.CanExecute {
		return nil, fmt.Errorf("accepted coordination plan violated the advisory-only invariant")
	}
	wantedPursuit := snapshot.AllocationItem.PursuitID.String()
	for _, node := range binding.Nodes {
		if strings.TrimSpace(node.Bindings.PursuitID) == wantedPursuit {
			return binding, nil
		}
	}
	return nil, fmt.Errorf("accepted coordination plan no longer contains pursuit %s", wantedPursuit)
}

func portfolioWorkflowCoordinationPlanURI(effect PortfolioWorkflowEffect) string {
	if strings.TrimSpace(effect.CoordinationPlanID) == "" ||
		effect.CoordinationPlanRevision == 0 ||
		!validPortfolioRecordDigest(effect.CoordinationPlanDigest) ||
		strings.TrimSpace(effect.CoordinationPlanNodeID) == "" {
		return ""
	}
	return fmt.Sprintf(
		"hai://plans/%s/revisions/%d/nodes/%s#sha256:%s",
		effect.CoordinationPlanID,
		effect.CoordinationPlanRevision,
		url.PathEscape(effect.CoordinationPlanNodeID),
		effect.CoordinationPlanDigest,
	)
}

func normalizePortfolioWorkflowRisk(value string) executionauth.RiskLevel {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(executionauth.RiskLow):
		return executionauth.RiskLow
	case string(executionauth.RiskMedium):
		return executionauth.RiskMedium
	case string(executionauth.RiskCritical):
		return executionauth.RiskCritical
	default:
		return executionauth.RiskHigh
	}
}

func parsePortfolioWorkflowApprovalSource(sourceID string) (uuid.UUID, error) {
	if !strings.HasPrefix(sourceID, PortfolioWorkflowEffectApprovalSourcePrefix) {
		return uuid.Nil, ErrPortfolioWorkflowApprovalInvalid
	}
	raw := strings.TrimPrefix(sourceID, PortfolioWorkflowEffectApprovalSourcePrefix)
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil || sourceID != PortfolioWorkflowEffectApprovalSourcePrefix+id.String() {
		return uuid.Nil, ErrPortfolioWorkflowApprovalInvalid
	}
	return id, nil
}

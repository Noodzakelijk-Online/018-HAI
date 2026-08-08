package controlledlearning

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxControlledLearningRequestBytes = 256 * 1024
	maxControlledLearningListLimit    = 500
	defaultControlledLearningLimit    = 100
)

type Handler struct {
	service *Service
}

type recordOutcomeHTTPReq struct {
	IdempotencyKey string             `json:"idempotencyKey"`
	OperationID    string             `json:"operationId"`
	ProjectKey     string             `json:"projectKey,omitempty"`
	DomainPackIDs  []string           `json:"domainPackIds,omitempty"`
	Basis          EvidenceBasis      `json:"basis"`
	Status         OutcomeStatus      `json:"status"`
	Summary        string             `json:"summary"`
	HumanConfirmed bool               `json:"humanConfirmed"`
	Correction     string             `json:"correction,omitempty"`
	Verification   VerificationStatus `json:"verification"`
	Sources        []SourceReference  `json:"sources,omitempty"`
	Criteria       []CriterionResult  `json:"criteria,omitempty"`
	Metrics        []MetricResult     `json:"metrics,omitempty"`
	Tags           []string           `json:"tags,omitempty"`
	OccurredAt     time.Time          `json:"occurredAt"`
}

type proposeHTTPReq struct {
	IdempotencyKey  string         `json:"idempotencyKey"`
	Method          LearningMethod `json:"method"`
	Target          TargetKind     `json:"target"`
	Title           string         `json:"title"`
	Hypothesis      string         `json:"hypothesis"`
	ProposedChange  string         `json:"proposedChange"`
	CurrentVersion  string         `json:"currentVersion"`
	ProposedVersion string         `json:"proposedVersion"`
	RollbackPlan    string         `json:"rollbackPlan"`
	EvaluationPlan  string         `json:"evaluationPlan"`
	EvidenceIDs     []string       `json:"evidenceIds"`
}

type decideHTTPReq struct {
	IdempotencyKey      string       `json:"idempotencyKey,omitempty"`
	ExpectedRevision    int64        `json:"expectedRevision"`
	Kind                DecisionKind `json:"kind"`
	HumanConfirmed      bool         `json:"humanConfirmed"`
	Rationale           string       `json:"rationale"`
	GovernanceReference string       `json:"governanceReference,omitempty"`
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) RecordOutcome(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	var input recordOutcomeHTTPReq
	if !decodeControlledLearningJSON(c, &input) {
		return
	}
	actor := ""
	if input.HumanConfirmed || input.Basis == EvidenceHumanCorrection {
		actor = owner
	}
	result, err := handler.service.RecordOutcome(c.Request.Context(), RecordOutcomeRequest{
		OwnerIdentity:  owner,
		IdempotencyKey: input.IdempotencyKey,
		OperationID:    input.OperationID,
		ProjectKey:     input.ProjectKey,
		DomainPackIDs:  input.DomainPackIDs,
		Basis:          input.Basis,
		Status:         input.Status,
		Summary:        input.Summary,
		ActorIdentity:  actor,
		HumanConfirmed: input.HumanConfirmed,
		Correction:     input.Correction,
		Verification:   input.Verification,
		Sources:        input.Sources,
		Criteria:       input.Criteria,
		Metrics:        input.Metrics,
		Tags:           input.Tags,
		OccurredAt:     input.OccurredAt,
	})
	respondControlledLearning(c, result, err, http.StatusCreated)
}

func (handler *Handler) GetOutcome(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	id, ok := controlledLearningPathID(c, "id", "outcome id")
	if !ok {
		return
	}
	result, err := handler.service.repository.GetOutcome(c.Request.Context(), owner, id)
	respondControlledLearning(c, result, err, http.StatusOK)
}

func (handler *Handler) ListOutcomes(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	limit, ok := controlledLearningLimit(c)
	if !ok {
		return
	}
	operationID := strings.TrimSpace(c.Query("operationId"))
	if operationID != "" {
		if err := validateRequired("operation id", operationID, maxIdentifierLength); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid controlled learning list filter"})
			return
		}
	}
	result, err := handler.service.repository.ListOutcomes(c.Request.Context(), OutcomeQuery{
		OwnerIdentity: owner,
		OperationID:   operationID,
		Limit:         limit,
	})
	respondControlledLearning(c, gin.H{"outcomes": result}, err, http.StatusOK)
}

func (handler *Handler) Propose(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	var input proposeHTTPReq
	if !decodeControlledLearningJSON(c, &input) {
		return
	}
	result, err := handler.service.Propose(c.Request.Context(), ProposeRequest{
		OwnerIdentity:   owner,
		IdempotencyKey:  input.IdempotencyKey,
		Method:          input.Method,
		Target:          input.Target,
		Title:           input.Title,
		Hypothesis:      input.Hypothesis,
		ProposedChange:  input.ProposedChange,
		CurrentVersion:  input.CurrentVersion,
		ProposedVersion: input.ProposedVersion,
		RollbackPlan:    input.RollbackPlan,
		EvaluationPlan:  input.EvaluationPlan,
		EvidenceIDs:     input.EvidenceIDs,
	})
	respondControlledLearning(c, result, err, http.StatusCreated)
}

func (handler *Handler) GetProposal(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	id, ok := controlledLearningPathID(c, "id", "proposal id")
	if !ok {
		return
	}
	result, err := handler.service.repository.GetProposal(c.Request.Context(), owner, id)
	respondControlledLearning(c, result, err, http.StatusOK)
}

func (handler *Handler) ListProposals(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	limit, ok := controlledLearningLimit(c)
	if !ok {
		return
	}
	status := ProposalStatus(strings.TrimSpace(c.Query("status")))
	if status != "" && !validProposalFilterStatus(status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid controlled learning list filter"})
		return
	}
	result, err := handler.service.repository.ListProposals(c.Request.Context(), ProposalQuery{
		OwnerIdentity: owner,
		Status:        status,
		Limit:         limit,
	})
	respondControlledLearning(c, gin.H{"proposals": result}, err, http.StatusOK)
}

func (handler *Handler) Decide(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	proposalID, ok := controlledLearningProposalID(c)
	if !ok {
		return
	}
	var input decideHTTPReq
	if !decodeControlledLearningJSON(c, &input) {
		return
	}
	// The HTTP boundary returns both the revised proposal and any durable
	// application record. Service.Decide remains the narrow compatibility API
	// for internal callers that only need the proposal.
	result, err := handler.service.DecideAndApply(c.Request.Context(), DecideRequest{
		OwnerIdentity:       owner,
		ProposalID:          proposalID,
		IdempotencyKey:      input.IdempotencyKey,
		ExpectedRevision:    input.ExpectedRevision,
		Kind:                input.Kind,
		ActorIdentity:       owner,
		HumanConfirmed:      input.HumanConfirmed,
		Rationale:           input.Rationale,
		GovernanceReference: input.GovernanceReference,
	})
	respondControlledLearning(c, publicDecisionResult(result), err, http.StatusOK)
}

func (handler *Handler) GetDecision(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	proposalID, ok := controlledLearningProposalID(c)
	if !ok {
		return
	}
	decisionID, ok := controlledLearningPathID(c, "decisionId", "decision id")
	if !ok {
		return
	}
	decisions, err := handler.service.repository.ListDecisions(
		c.Request.Context(),
		owner,
		proposalID,
	)
	if err != nil {
		respondControlledLearning(c, nil, err, http.StatusOK)
		return
	}
	for _, decision := range decisions {
		if decision.ID == decisionID {
			c.JSON(http.StatusOK, decision)
			return
		}
	}
	respondControlledLearning(c, nil, ErrNotFound, http.StatusOK)
}

func (handler *Handler) ListDecisions(c *gin.Context) {
	owner, ok := handler.owner(c)
	if !ok {
		return
	}
	proposalID, ok := controlledLearningProposalID(c)
	if !ok {
		return
	}
	limit, ok := controlledLearningLimit(c)
	if !ok {
		return
	}
	kind := DecisionKind(strings.TrimSpace(c.Query("kind")))
	if kind != "" && !validDecisionFilterKind(kind) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid controlled learning list filter"})
		return
	}
	result, err := handler.service.repository.ListDecisions(
		c.Request.Context(),
		owner,
		proposalID,
	)
	if err != nil {
		respondControlledLearning(c, nil, err, http.StatusOK)
		return
	}
	filtered := make([]ReviewDecision, 0, len(result))
	for _, decision := range result {
		if kind == "" || decision.Kind == kind {
			filtered = append(filtered, decision)
		}
		if len(filtered) == limit {
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{"decisions": filtered})
}

func (handler *Handler) owner(c *gin.Context) (string, bool) {
	value, exists := c.Get(identity.ContextSubjectKey)
	owner, ok := value.(string)
	owner = strings.TrimSpace(owner)
	if !exists || !ok || owner == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "an authenticated owner session is required for controlled learning",
		})
		return "", false
	}
	if handler == nil || handler.service == nil || handler.service.repository == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "controlled learning is unavailable",
		})
		return "", false
	}
	return owner, true
}

func controlledLearningProposalID(c *gin.Context) (string, bool) {
	key := "proposalId"
	if strings.TrimSpace(c.Param(key)) == "" {
		key = "id"
	}
	return controlledLearningPathID(c, key, "proposal id")
}

func controlledLearningPathID(c *gin.Context, key, label string) (string, bool) {
	value := strings.TrimSpace(c.Param(key))
	if err := validateRequired(label, value, maxIdentifierLength); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid controlled learning resource id"})
		return "", false
	}
	return value, true
}

func controlledLearningLimit(c *gin.Context) (int, bool) {
	values, exists := c.Request.URL.Query()["limit"]
	if !exists {
		return defaultControlledLearningLimit, true
	}
	if len(values) != 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "controlled learning limit must be provided exactly once",
		})
		return 0, false
	}
	raw := strings.TrimSpace(values[0])
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maxControlledLearningListLimit {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf(
				"controlled learning limit must be between 1 and %d",
				maxControlledLearningListLimit,
			),
		})
		return 0, false
	}
	return limit, true
}

func validProposalFilterStatus(status ProposalStatus) bool {
	switch status {
	case ProposalReviewRequired, ProposalGovernanceRequired, ProposalGovernanceReview,
		ProposalApproved, ProposalRejected, ProposalChangesRequested:
		return true
	default:
		return false
	}
}

func validDecisionFilterKind(kind DecisionKind) bool {
	switch kind {
	case DecisionApprove, DecisionReject, DecisionRequestChanges, DecisionEscalateGovernance:
		return true
	default:
		return false
	}
}

func decodeControlledLearningJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxControlledLearningRequestBytes,
	)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": "controlled learning request is too large",
			})
			return false
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid controlled learning request",
		})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "controlled learning request must contain one JSON object",
		})
		return false
	}
	return true
}

func respondControlledLearning(c *gin.Context, value any, err error, successStatus int) {
	if err == nil {
		c.JSON(successStatus, value)
		return
	}
	status, message := controlledLearningErrorResponse(err)
	if status == http.StatusInternalServerError {
		errorID := uuid.NewString()
		// Repository and provider errors may carry DSNs, credentials, source
		// payloads, or identities. Record only an opaque correlation marker in
		// the request context and keep the response deterministic.
		_ = c.Error(fmt.Errorf("controlled learning operation failed (%s)", errorID))
		c.JSON(status, gin.H{"error": message, "errorId": errorID})
		return
	}
	c.JSON(status, gin.H{"error": message})
}

func controlledLearningErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrOwnerScopeViolation):
		return http.StatusNotFound, "controlled learning record not found"
	case errors.Is(err, ErrIdempotencyConflict),
		errors.Is(err, ErrRevisionConflict),
		errors.Is(err, ErrInvalidStateChange),
		errors.Is(err, ErrApplicationInProgress):
		return http.StatusConflict, "controlled learning state conflict"
	case errors.Is(err, ErrPromoterUnavailable),
		errors.Is(err, ErrRollbackUnavailable):
		return http.StatusServiceUnavailable, "controlled learning application is unavailable"
	case errors.Is(err, ErrApplicationFailed):
		return http.StatusBadGateway, "controlled learning application failed"
	case errors.Is(err, ErrIntegrityViolation):
		return http.StatusConflict, "controlled learning record failed integrity verification"
	case errors.Is(err, ErrProtectedTarget):
		return http.StatusUnprocessableEntity, "protected learning target requires governance review"
	case errors.Is(err, ErrUnsupportedEvidence):
		return http.StatusUnprocessableEntity, "learning evidence is not eligible"
	case isControlledLearningValidationError(err):
		return http.StatusBadRequest, "controlled learning request failed validation"
	default:
		return http.StatusInternalServerError, "controlled learning operation failed"
	}
}

func isControlledLearningValidationError(err error) bool {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, fragment := range []string{
		"is required",
		"must be",
		"must differ",
		"must contain",
		"require explicit",
		"requires a",
		"unsupported ",
		"invalid ",
		"at least ",
		"at most ",
		"exceeds ",
		"in the future",
		"duplicate ",
		"references unknown",
		"contains credential",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

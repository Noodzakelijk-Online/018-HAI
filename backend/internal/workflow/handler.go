package workflow

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service             Service
	pursuitIntakeRouter PursuitIntakeRouter
}

// PursuitIntakeRouter keeps the legacy workflow endpoint compatible while
// allowing the canonical application to route new work through pursuits.
// Implementations must return the workflow record created or reused by the
// governed pursuit intake path.
type PursuitIntakeRouter interface {
	RouteWorkflowIntake(request IntakeRequest) (*WorkflowRecord, error)
}

func DefaultHandler() *Handler {
	return &Handler{service: DefaultService()}
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func NewHandlerWithPursuitIntakeRouter(service Service, pursuitIntakeRouter PursuitIntakeRouter) *Handler {
	return &Handler{service: service, pursuitIntakeRouter: pursuitIntakeRouter}
}

func (h *Handler) Intake(c *gin.Context) {
	var request IntakeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedWorkflowActor(c, "operator")
	request.OwnerIdentity = verifiedWorkflowOwner(c)
	request = normalizeWorkflowAPIIntake(request)
	var record *WorkflowRecord
	var err error
	if h.pursuitIntakeRouter != nil {
		record, err = h.pursuitIntakeRouter.RouteWorkflowIntake(request)
	} else {
		record, err = h.service.Intake(request)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, record)
}

func normalizeWorkflowAPIIntake(request IntakeRequest) IntakeRequest {
	request.Trigger = firstNonEmpty(request.Trigger, "workflow_api_intake")
	if strings.TrimSpace(request.SourceType) == "" {
		request.SourceType = "workflow_api"
	}
	if strings.TrimSpace(request.SourceID) == "" && strings.TrimSpace(request.SourceURI) == "" {
		request.SourceID = workflowAPIIntakeSourceID(request)
		request.SourceURI = "workflow-api://intake/" + request.SourceID
		request.SourceLabel = firstNonEmpty(request.SourceLabel, "Direct workflow API intake")
	}
	return request
}

func workflowAPIIntakeSourceID(request IntakeRequest) string {
	value := strings.Join([]string{
		strings.ToLower(strings.Join(strings.Fields(request.Input), " ")),
		strings.ToLower(strings.TrimSpace(request.ProjectKey)),
		strings.ToLower(strings.TrimSpace(request.AutomationID)),
	}, "\n")
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("workflow-api-%x", sum[:12])
}

func (h *Handler) Items(c *gin.Context) {
	includeArchived, _ := strconv.ParseBool(c.Query("includeArchived"))
	items, err := h.service.ItemsForOwner(verifiedWorkflowOwner(c), includeArchived)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) ApprovalItems(c *gin.Context) {
	items, err := h.service.ApprovalItemsForOwner(verifiedWorkflowOwner(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) Dashboard(c *gin.Context) {
	dashboard, err := h.service.DashboardForOwner(verifiedWorkflowOwner(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dashboard)
}

func (h *Handler) Get(c *gin.Context) {
	id, ok := parseWorkflowID(c)
	if !ok {
		return
	}
	record, err := h.service.GetForOwner(verifiedWorkflowOwner(c), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) Transition(c *gin.Context) {
	id, ok := parseWorkflowID(c)
	if !ok {
		return
	}
	if !h.ensureWorkflowVisible(c, id) {
		return
	}
	var request TransitionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Generic state transitions cannot establish approval provenance.
	// Approval-required workflows must use ResolveApproval.
	request.Approved = false
	request.Actor = verifiedWorkflowActor(c, "operator")
	_, err := h.service.Transition(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.respondScopedWorkflow(c, id, http.StatusOK)
}

func (h *Handler) ResolveApproval(c *gin.Context) {
	id, ok := parseWorkflowID(c)
	if !ok {
		return
	}
	if !h.ensureWorkflowVisible(c, id) {
		return
	}
	var request ApprovalResolutionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedWorkflowActor(c, "operator")
	_, err := h.service.ResolveApproval(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.respondScopedWorkflow(c, id, http.StatusOK)
}

func (h *Handler) ResolveInterruptedExecution(c *gin.Context) {
	id, ok := parseWorkflowID(c)
	if !ok {
		return
	}
	if !h.ensureWorkflowVisible(c, id) {
		return
	}
	var request InterruptedExecutionResolutionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedWorkflowActor(c, "operator")
	_, err := h.service.ResolveInterruptedExecution(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.respondScopedWorkflow(c, id, http.StatusOK)
}

func (h *Handler) ResolveProposal(c *gin.Context) {
	id, ok := parseWorkflowID(c)
	if !ok {
		return
	}
	if !h.ensureWorkflowVisible(c, id) {
		return
	}
	proposalID, err := uuid.Parse(c.Param("proposalId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid proposal id"})
		return
	}
	var request ProposalResolutionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedWorkflowActor(c, "operator")
	_, err = h.service.ResolveProposal(id, proposalID, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.respondScopedWorkflow(c, id, http.StatusOK)
}

func (h *Handler) UpdateChecklistItem(c *gin.Context) {
	id, ok := parseWorkflowID(c)
	if !ok {
		return
	}
	if !h.ensureWorkflowVisible(c, id) {
		return
	}
	itemID, err := uuid.Parse(c.Param("itemId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid checklist item id"})
		return
	}
	var request ChecklistUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedWorkflowActor(c, "operator")
	_, err = h.service.UpdateChecklistItem(id, itemID, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.respondScopedWorkflow(c, id, http.StatusOK)
}

// verifiedWorkflowActor ignores any actor label supplied in request JSON.
// The router writes the verified IDP JWT subject into the Gin context; when
// that identity path is absent in local development, audit records retain the
// explicit generic operator fallback instead of a forged user name.
func verifiedWorkflowActor(c *gin.Context, fallback string) string {
	if value, ok := c.Get(identity.ContextSubjectKey); ok {
		if subject, ok := value.(string); ok && strings.TrimSpace(subject) != "" {
			return strings.TrimSpace(subject)
		}
	}
	return fallback
}

func verifiedWorkflowOwner(c *gin.Context) string {
	if value, ok := c.Get(identity.ContextSubjectKey); ok {
		if subject, ok := value.(string); ok {
			return strings.TrimSpace(subject)
		}
	}
	return ""
}

func (h *Handler) ensureWorkflowVisible(c *gin.Context, id uuid.UUID) bool {
	if _, err := h.service.GetForOwner(verifiedWorkflowOwner(c), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workflow not found"})
		return false
	}
	return true
}

func (h *Handler) respondScopedWorkflow(c *gin.Context, id uuid.UUID, status int) {
	record, err := h.service.GetForOwner(verifiedWorkflowOwner(c), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workflow not found"})
		return
	}
	c.JSON(status, record)
}

func (h *Handler) RunDue(c *gin.Context) {
	var request RunDueRequest
	_ = c.ShouldBindJSON(&request)
	result, err := h.service.RunDueForOwner(verifiedWorkflowOwner(c), request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) RecoverStaleClaims(c *gin.Context) {
	var request RunDueRequest
	_ = c.ShouldBindJSON(&request)
	result, err := h.service.RecoverStaleClaimsForOwner(verifiedWorkflowOwner(c), request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) RunDueOpenLoops(c *gin.Context) {
	var request RunDueRequest
	_ = c.ShouldBindJSON(&request)
	result, err := h.service.RunDueOpenLoopsForOwner(verifiedWorkflowOwner(c), request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Overview(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.Overview())
}

func parseWorkflowID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workflow id"})
		return uuid.UUID{}, false
	}
	return id, true
}

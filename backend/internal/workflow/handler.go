package workflow

import (
	"net/http"
	"strconv"
	"strings"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service Service
}

func DefaultHandler() *Handler {
	return &Handler{service: DefaultService()}
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Intake(c *gin.Context) {
	var request IntakeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedWorkflowActor(c, "operator")
	record, err := h.service.Intake(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, record)
}

func (h *Handler) Items(c *gin.Context) {
	includeArchived, _ := strconv.ParseBool(c.Query("includeArchived"))
	items, err := h.service.Items(includeArchived)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) ApprovalItems(c *gin.Context) {
	items, err := h.service.ApprovalItems()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) Dashboard(c *gin.Context) {
	dashboard, err := h.service.Dashboard()
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
	record, err := h.service.Get(id)
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
	var request TransitionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Generic state transitions cannot establish approval provenance.
	// Approval-required workflows must use ResolveApproval.
	request.Approved = false
	request.Actor = verifiedWorkflowActor(c, "operator")
	record, err := h.service.Transition(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) ResolveApproval(c *gin.Context) {
	id, ok := parseWorkflowID(c)
	if !ok {
		return
	}
	var request ApprovalResolutionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedWorkflowActor(c, "operator")
	record, err := h.service.ResolveApproval(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) ResolveInterruptedExecution(c *gin.Context) {
	id, ok := parseWorkflowID(c)
	if !ok {
		return
	}
	var request InterruptedExecutionResolutionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	request.Actor = verifiedWorkflowActor(c, "operator")
	record, err := h.service.ResolveInterruptedExecution(id, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) ResolveProposal(c *gin.Context) {
	id, ok := parseWorkflowID(c)
	if !ok {
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
	record, err := h.service.ResolveProposal(id, proposalID, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) UpdateChecklistItem(c *gin.Context) {
	id, ok := parseWorkflowID(c)
	if !ok {
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
	record, err := h.service.UpdateChecklistItem(id, itemID, request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, record)
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

func (h *Handler) RunDue(c *gin.Context) {
	var request RunDueRequest
	_ = c.ShouldBindJSON(&request)
	result, err := h.service.RunDue(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) RecoverStaleClaims(c *gin.Context) {
	var request RunDueRequest
	_ = c.ShouldBindJSON(&request)
	result, err := h.service.RecoverStaleClaims(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) RunDueOpenLoops(c *gin.Context) {
	var request RunDueRequest
	_ = c.ShouldBindJSON(&request)
	result, err := h.service.RunDueOpenLoops(request)
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

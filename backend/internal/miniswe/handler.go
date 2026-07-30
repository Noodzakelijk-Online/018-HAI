package miniswe

import (
	"errors"
	"net/http"
	"strconv"

	"automation-hub-backend/internal/identity"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) Status(c *gin.Context) { c.JSON(http.StatusOK, h.service.Status()) }

func (h *Handler) Probe(c *gin.Context) {
	result, err := h.service.Probe(c.Request.Context())
	if errors.Is(err, ErrNotConfigured) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "status": h.service.Status()})
		return
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local mini-SWE runner could not be verified"})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) ProposePatch(c *gin.Context) {
	workflowID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid workflow ID"})
		return
	}
	var request struct {
		WorkspaceID string `json:"workspaceId"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspaceId is required"})
		return
	}
	proposal, err := h.service.ProposePatch(c.Request.Context(), owner(c), workflowID, request.WorkspaceID)
	switch {
	case errors.Is(err, ErrNotConfigured):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "status": h.service.Status()})
	case errors.Is(err, ErrApprovalRequired):
		c.JSON(http.StatusPreconditionRequired, gin.H{"error": err.Error()})
	case errors.Is(err, ErrWorkflowNotReady):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ErrWorkspaceDenied), errors.Is(err, ErrInvalidRequest):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "local mini-SWE runner could not produce a patch proposal", "job": proposal})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not record mini-SWE patch proposal"})
	default:
		c.JSON(http.StatusCreated, proposal)
	}
}

func (h *Handler) Jobs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	jobs, err := h.service.Jobs(owner(c), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read mini-SWE patch proposal history"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": jobs})
}

func owner(c *gin.Context) string {
	value, _ := c.Get(identity.ContextSubjectKey)
	ownerIdentity, _ := value.(string)
	return ownerIdentity
}
